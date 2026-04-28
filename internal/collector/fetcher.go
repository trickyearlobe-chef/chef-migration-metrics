// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
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
	OrganisationName string
	Name             string
	Version          string
	Err              error
}

func (e CookbookFetchError) Error() string {
	return fmt.Sprintf("%s/%s: %v", e.Name, e.Version, e.Err)
}

// markDownloadFailed records a download failure in the database. It uses a
// background context with a short timeout if the original context has been
// cancelled, to ensure the failure is recorded even during shutdown.
func markDownloadFailed(ctx context.Context, db *datastore.DB, organisationName, name, version string, dlErr error) {
	errStr := formatDownloadError(dlErr)
	dbCtx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		dbCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	// Best-effort — if this also fails, the cookbook remains in its current
	// status (pending or failed) and will be retried on the next run.
	_, _ = db.MarkServerCookbookDownloadFailed(dbCtx, organisationName, name, version, errStr)
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

// extractCookbookFiles downloads all files from a cookbook version manifest
// into destDir. Returns the number of files written and any error.
func extractCookbookFiles(
	ctx context.Context,
	client *chefapi.Client,
	manifest *chefapi.CookbookVersionManifest,
	destDir string,
) (int, error) {
	allFiles := manifest.AllFiles()
	if len(allFiles) == 0 {
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
			continue
		}

		if err := downloadAndWriteFile(ctx, client, ref, destDir); err != nil {
			return written, fmt.Errorf("downloading %s: %w", ref.Path, err)
		}
		written++
	}

	return written, nil
}

func downloadAndWriteFile(
	ctx context.Context,
	client *chefapi.Client,
	ref chefapi.CookbookFileRef,
	destDir string,
) error {
	cleanPath := filepath.Clean(ref.Path)
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || hasParentTraversal(cleanPath) {
		return fmt.Errorf("unsafe file path in manifest: %q", ref.Path)
	}

	fullPath := filepath.Join(destDir, cleanPath)

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

	data, err := client.DownloadFileContent(ctx, ref.URL, ref.Checksum)
	if err != nil {
		return err
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return fmt.Errorf("writing file %s: %w", fullPath, err)
	}

	return nil
}

func hasParentTraversal(cleanPath string) bool {
	for _, part := range splitPathComponents(cleanPath) {
		if part == ".." {
			return true
		}
	}
	return false
}

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
		p = filepath.Clean(dir)
	}
	return parts
}

func isSubPath(parent, child string) bool {
	parentPrefix := parent
	if parentPrefix != "/" && parentPrefix[len(parentPrefix)-1] != filepath.Separator {
		parentPrefix += string(filepath.Separator)
	}
	return child == parent || len(child) > len(parentPrefix) && child[:len(parentPrefix)] == parentPrefix
}
