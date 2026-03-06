// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// CookbookFetchResult summarises the outcome of a cookbook fetching pass.
type CookbookFetchResult struct {
	// Total is the number of cookbook versions that were candidates for
	// download (pending or failed status, active only).
	Total int

	// Downloaded is the number of cookbook versions successfully downloaded.
	Downloaded int

	// Skipped is the number of cookbook versions skipped because they are
	// already downloaded (status = 'ok'). This should normally be zero
	// since we only query for pending/failed, but is tracked for
	// observability in case of races.
	Skipped int

	// Failed is the number of cookbook versions whose download failed.
	Failed int

	// FilesWritten is the total number of individual files written to disk
	// across all successfully downloaded cookbook versions.
	FilesWritten int

	// Duration is the wall-clock time spent on the fetching pass.
	Duration time.Duration

	// Errors collects per-cookbook error details for logging.
	Errors []CookbookFetchError
}

// CookbookFetchError records a single cookbook version download failure.
type CookbookFetchError struct {
	CookbookID string
	Name       string
	Version    string
	Err        error
}

func (e CookbookFetchError) Error() string {
	return fmt.Sprintf("%s/%s: %v", e.Name, e.Version, e.Err)
}

// fetchCookbooks downloads cookbook versions from the Chef server that have
// a pending or failed download status. Only active cookbooks (applied to at
// least one node) are fetched — unused cookbooks are identified and flagged
// but not downloaded, per the specification.
//
// Each cookbook version's manifest is fetched via GetCookbookVersionManifest,
// and then every file in the manifest is downloaded from the bookshelf URL
// and written to disk under <cookbookCacheDir>/<org_id>/<name>/<version>/.
// On success the download_status is set to 'ok'. On failure it is set to
// 'failed' with the error detail recorded. Failures are non-fatal — the
// run continues for all remaining cookbooks.
//
// Downloads are parallelised up to the configured concurrency.git_pull
// worker pool size (since both git pulls and cookbook downloads are I/O-bound
// fetching operations with similar resource profiles).
//
// If cookbookCacheDir is empty, file extraction to disk is skipped and only
// the manifest is fetched (legacy behaviour for environments where disk
// extraction is not yet configured). The download_status is still updated.
func fetchCookbooks(
	ctx context.Context,
	client *chefapi.Client,
	db *datastore.DB,
	log *logging.ScopedLogger,
	org datastore.Organisation,
	concurrency int,
	cookbookCacheDir string,
) CookbookFetchResult {
	start := time.Now()

	if concurrency <= 0 {
		concurrency = 1
	}

	// Find all active cookbook versions that need downloading.
	cookbooks, err := db.ListActiveCookbooksNeedingDownload(ctx, org.ID)
	if err != nil {
		log.Error(fmt.Sprintf("failed to list cookbooks needing download: %v", err))
		return CookbookFetchResult{
			Duration: time.Since(start),
			Errors: []CookbookFetchError{{
				Err: fmt.Errorf("listing cookbooks needing download: %w", err),
			}},
		}
	}

	result := CookbookFetchResult{
		Total: len(cookbooks),
	}

	if len(cookbooks) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	log.Info(fmt.Sprintf("downloading %d cookbook version(s) from Chef server", len(cookbooks)))

	// Use a buffered channel as a semaphore to bound concurrency.
	sem := make(chan struct{}, concurrency)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cb := range cookbooks {
		// Check for context cancellation before starting a new download.
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(cb datastore.Cookbook) {
			defer wg.Done()

			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			filesWritten, fetchErr := downloadCookbookVersion(ctx, client, db, cb, cookbookCacheDir)

			mu.Lock()
			defer mu.Unlock()

			if fetchErr != nil {
				result.Failed++
				cfe := CookbookFetchError{
					CookbookID: cb.ID,
					Name:       cb.Name,
					Version:    cb.Version,
					Err:        fetchErr,
				}
				result.Errors = append(result.Errors, cfe)
				log.Warn(fmt.Sprintf("cookbook download failed: %s", cfe.Error()))
			} else {
				result.Downloaded++
				result.FilesWritten += filesWritten
			}
		}(cb)
	}

	wg.Wait()
	result.Duration = time.Since(start)
	return result
}

// downloadCookbookVersion fetches a single cookbook version from the Chef
// server, extracts all files to disk, and updates its download status in the
// database. Cookbook versions on the Chef server are immutable, so a
// successful download only needs to happen once.
//
// The function:
//  1. Calls GetCookbookVersionManifest to fetch the cookbook manifest
//     containing file references with bookshelf download URLs.
//  2. Downloads each file from its bookshelf URL, validates its SHA-256
//     checksum, and writes it to <cookbookCacheDir>/<org_id>/<name>/<version>/<path>.
//  3. On success: marks the cookbook as download_status = 'ok'.
//  4. On failure: marks the cookbook as download_status = 'failed' with the
//     error detail, so the dashboard can show the failure indicator and the
//     version will be retried on the next collection run.
//
// If cookbookCacheDir is empty, file extraction is skipped and only the
// manifest fetch + status update are performed.
//
// Returns the number of files written to disk and any error.
func downloadCookbookVersion(
	ctx context.Context,
	client *chefapi.Client,
	db *datastore.DB,
	cb datastore.Cookbook,
	cookbookCacheDir string,
) (int, error) {
	// Fetch the cookbook version manifest from the Chef server.
	manifest, err := client.GetCookbookVersionManifest(ctx, cb.Name, cb.Version)
	if err != nil {
		markDownloadFailed(ctx, db, cb.ID, err)
		return 0, err
	}

	// Extract all files to disk if a cache directory is configured.
	filesWritten := 0
	if cookbookCacheDir != "" {
		destDir := filepath.Join(cookbookCacheDir, cb.OrganisationID, cb.Name, cb.Version)

		written, extractErr := extractCookbookFiles(ctx, client, manifest, destDir)
		if extractErr != nil {
			// Clean up partial download — remove the destination directory
			// so the next retry starts fresh.
			_ = os.RemoveAll(destDir)
			markDownloadFailed(ctx, db, cb.ID, extractErr)
			return 0, extractErr
		}
		filesWritten = written
	}

	// Download succeeded — mark as 'ok'.
	if _, markErr := db.MarkCookbookDownloadOK(ctx, cb.ID); markErr != nil {
		return filesWritten, fmt.Errorf("downloaded successfully but failed to update status: %w", markErr)
	}

	return filesWritten, nil
}

// extractCookbookFiles downloads every file referenced in the cookbook
// manifest and writes them to the destination directory. Each file is
// written to <destDir>/<path> where path comes from the manifest entry
// (e.g. "recipes/default.rb", "metadata.rb").
//
// Files are downloaded sequentially within a single cookbook version. The
// parallelism is at the cookbook-version level (bounded by the semaphore in
// fetchCookbooks), not at the file level, to avoid overwhelming the
// bookshelf with too many concurrent requests.
//
// Returns the number of files successfully written and any error. On error,
// the caller is responsible for cleaning up any partially written files.
func extractCookbookFiles(
	ctx context.Context,
	client *chefapi.Client,
	manifest *chefapi.CookbookVersionManifest,
	destDir string,
) (int, error) {
	allFiles := manifest.AllFiles()
	if len(allFiles) == 0 {
		// Empty cookbook — create the directory anyway so the path resolver
		// finds it, then return success.
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return 0, fmt.Errorf("creating cookbook directory: %w", err)
		}
		return 0, nil
	}

	written := 0
	for _, ref := range allFiles {
		if ctx.Err() != nil {
			return written, fmt.Errorf("context cancelled after %d of %d files: %w", written, len(allFiles), ctx.Err())
		}

		if ref.Path == "" {
			// Defensive — skip files with no path (shouldn't happen in
			// practice but guard against malformed manifests).
			continue
		}

		if err := downloadAndWriteFile(ctx, client, ref, destDir); err != nil {
			return written, fmt.Errorf("downloading %s: %w", ref.Path, err)
		}
		written++
	}

	return written, nil
}

// downloadAndWriteFile downloads a single cookbook file from its bookshelf
// URL, validates the checksum, and writes it to <destDir>/<path>. Parent
// directories are created as needed.
func downloadAndWriteFile(
	ctx context.Context,
	client *chefapi.Client,
	ref chefapi.CookbookFileRef,
	destDir string,
) error {
	// Sanitise the path to prevent directory traversal attacks. The Chef
	// server should only return well-formed relative paths, but we validate
	// defensively.
	cleanPath := filepath.Clean(ref.Path)
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || hasParentTraversal(cleanPath) {
		return fmt.Errorf("unsafe file path in manifest: %q", ref.Path)
	}

	fullPath := filepath.Join(destDir, cleanPath)

	// Ensure the resolved path is actually under destDir (belt and braces).
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolving destination directory: %w", err)
	}
	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}
	if !isSubPath(absDestDir, absFullPath) {
		return fmt.Errorf("file path %q escapes destination directory", ref.Path)
	}

	// Download the file content with checksum validation.
	data, err := client.DownloadFileContent(ctx, ref.URL, ref.Checksum)
	if err != nil {
		return err
	}

	// Create parent directories.
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	// Write the file. Use 0644 permissions — these are read-only analysis
	// inputs, not executable scripts (CookStyle reads them as text).
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return fmt.Errorf("writing file %s: %w", fullPath, err)
	}

	return nil
}

// hasParentTraversal returns true if the cleaned path contains a ".."
// component that could escape the base directory.
func hasParentTraversal(cleanPath string) bool {
	// filepath.Clean normalises "a/../b" to "b", but a leading ".." in the
	// cleaned result means the path escapes the base.
	for _, part := range splitPathComponents(cleanPath) {
		if part == ".." {
			return true
		}
	}
	return false
}

// splitPathComponents splits a filepath into its individual directory/file
// components using the OS-specific separator.
func splitPathComponents(p string) []string {
	var parts []string
	for {
		dir, file := filepath.Split(p)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		if dir == "" || dir == p {
			break
		}
		// Remove trailing separator from dir for next iteration.
		p = filepath.Clean(dir)
	}
	return parts
}

// isSubPath returns true if child is a path under (or equal to) parent.
// Both paths must be absolute.
func isSubPath(parent, child string) bool {
	// Ensure parent has a trailing separator for prefix comparison, so
	// "/var/lib/foo" doesn't match "/var/lib/foobar".
	parentPrefix := parent
	if parentPrefix != "/" && parentPrefix[len(parentPrefix)-1] != filepath.Separator {
		parentPrefix += string(filepath.Separator)
	}
	return child == parent || len(child) > len(parentPrefix) && child[:len(parentPrefix)] == parentPrefix
}

// markDownloadFailed records a download failure in the database. It uses a
// background context with a short timeout if the original context has been
// cancelled, to ensure the failure is recorded even during shutdown.
func markDownloadFailed(ctx context.Context, db *datastore.DB, cookbookID string, dlErr error) {
	errStr := formatDownloadError(dlErr)
	dbCtx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		dbCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	// Best-effort — if this also fails, the cookbook remains in its current
	// status (pending or failed) and will be retried on the next run.
	_, _ = db.MarkCookbookDownloadFailed(dbCtx, cookbookID, errStr)
}

// formatDownloadError produces a human-readable error string suitable for
// storage in the download_error column. If the error is an APIError, it
// includes the HTTP status code.
func formatDownloadError(err error) string {
	if apiErr, ok := err.(*chefapi.APIError); ok {
		return fmt.Sprintf("%d %s: %s", apiErr.StatusCode, apiErr.Method, apiErr.Body)
	}
	return err.Error()
}
