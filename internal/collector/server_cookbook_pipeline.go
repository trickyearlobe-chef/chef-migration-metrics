// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ServerCookbookPipelineResult summarises the concurrent download+scan
// pipeline for server cookbooks.
type ServerCookbookPipelineResult struct {
	Total      int           // Cookbook versions considered
	Downloaded int           // Successfully downloaded
	Scanned    int           // Successfully scanned by CookStyle
	Skipped    int           // Skipped (already scanned or not downloaded)
	Failed     int           // Download or scan failures
	Cleaned    int           // Legacy cached cookbook directories removed
	Duration   time.Duration // Wall-clock time

	Errors []CookbookFetchError // Per-cookbook errors
}

// scanWorkItem is sent from a download worker to a scan worker via a channel.
// It carries everything a scan worker needs to run CookStyle, generate
// autocorrect previews, and optionally delete the cookbook files from disk.
type scanWorkItem struct {
	cookbook        datastore.ServerCookbook
	destDir         string
	index           int // 1-based position in the total list (for logging)
	deleteAfterScan bool
}

// runServerCookbookPipeline processes server cookbooks using two concurrent
// worker pools connected by a channel:
//
//  1. Download workers fetch cookbook files from the Chef Server (or resolve
//     already-downloaded files from the cache). Each completed download is
//     sent to the scan channel immediately.
//  2. Scan workers receive downloaded cookbooks and run CookStyle analysis,
//     autocorrect preview generation, and optional file cleanup.
//
// This means scanning starts as soon as the first cookbook is downloaded,
// rather than waiting for all downloads to complete. Both pools are bounded:
// downloads by defaultDownloadWorkers, scans by the concurrency.cookstyle_scan
// configuration value.
//
// Cookbooks that already have CookStyle results are handled efficiently —
// scanOneServerCookbook detects existing results and returns Skipped=true,
// so no redundant work is performed.
//
// When cookstyleScanner is nil, the function falls back to download-only
// behaviour (scan workers just handle cleanup).
func runServerCookbookPipeline(
	ctx context.Context,
	client *chefapi.Client,
	db *datastore.DB,
	log *logging.ScopedLogger,
	org datastore.Organisation,
	cookbookCacheDir string,
	targetChefVersions []string,
	cookstyleScanner *analysis.CookstyleScanner,
	autocorrectGen *remediation.AutocorrectGenerator,
	deleteAfterScan bool,
	downloadWorkers int,
	scanWorkers int,
) ServerCookbookPipelineResult {
	start := time.Now()
	result := ServerCookbookPipelineResult{}

	// Get all active server cookbooks for this organisation — both those
	// needing download (pending/failed) AND those already downloaded (ok)
	// that may still need CookStyle scanning. The scan-skip logic inside
	// scanOneServerCookbook handles the immutability optimisation.
	cookbooks, err := db.ListActiveServerCookbooksForPipeline(ctx, org.Name)
	if err != nil {
		log.Error(fmt.Sprintf("failed to list cookbooks for pipeline: %v", err))
		result.Duration = time.Since(start)
		result.Errors = append(result.Errors, CookbookFetchError{
			Err: fmt.Errorf("listing cookbooks: %w", err),
		})
		return result
	}

	result.Total = len(cookbooks)

	if len(cookbooks) == 0 {
		log.Info("no server cookbook versions need processing")
		result.Duration = time.Since(start)
		return result
	}

	if downloadWorkers <= 0 {
		downloadWorkers = 4
	}

	log.Info(fmt.Sprintf("concurrent server cookbook pipeline: %d version(s) to process (%d download workers, %d scan workers)",
		len(cookbooks), downloadWorkers, scanWorkers))

	// -----------------------------------------------------------------------
	// Shared mutable state — protected by mu.
	// -----------------------------------------------------------------------
	var mu sync.Mutex
	var downloaded, scanned, skipped, failed int
	var errors []CookbookFetchError

	// Atomic counters for progress logging from any goroutine.
	var completedCount atomic.Int64

	// -----------------------------------------------------------------------
	// Scan channel — download workers produce, scan workers consume.
	// Buffered to avoid blocking download workers when scan workers are busy.
	// -----------------------------------------------------------------------
	scanCh := make(chan scanWorkItem, scanWorkers*2)

	// -----------------------------------------------------------------------
	// Scan worker pool — consumes from scanCh.
	// -----------------------------------------------------------------------
	var scanWG sync.WaitGroup
	for w := 0; w < scanWorkers; w++ {
		scanWG.Add(1)
		go func() {
			defer scanWG.Done()
			for item := range scanCh {
				if ctx.Err() != nil {
					// Context cancelled — still clean up if needed.
					if item.deleteAfterScan && item.destDir != "" {
						_ = os.RemoveAll(item.destDir)
					}
					continue // drain channel
				}

				scanCount, skipCount := scanAndPreview(
					ctx, db, log, item,
					targetChefVersions, cookstyleScanner, autocorrectGen,
					len(cookbooks),
				)

				// Optionally delete the downloaded files after scanning.
				if item.deleteAfterScan && item.destDir != "" {
					if removeErr := os.RemoveAll(item.destDir); removeErr != nil {
						log.Warn(fmt.Sprintf("[%d/%d] failed to clean up cookbook files %s/%s at %s: %v",
							item.index, len(cookbooks), item.cookbook.Name, item.cookbook.Version, item.destDir, removeErr))
					}
				}

				mu.Lock()
				scanned += scanCount
				skipped += skipCount
				mu.Unlock()

				done := completedCount.Add(1)
				// Periodic progress summary every 25 completed cookbooks.
				if done%25 == 0 {
					mu.Lock()
					totalElapsed := time.Since(start).Round(time.Second)
					log.Info(fmt.Sprintf("pipeline progress: %d/%d completed (%d downloaded, %d scanned, %d skipped, %d failed) in %s",
						done, len(cookbooks), downloaded, scanned, skipped, failed, totalElapsed))
					mu.Unlock()
				}
			}
		}()
	}

	// -----------------------------------------------------------------------
	// Download worker pool — produces into scanCh.
	// -----------------------------------------------------------------------
	var downloadWG sync.WaitGroup
	downloadSem := make(chan struct{}, downloadWorkers)

	for i, cb := range cookbooks {
		if ctx.Err() != nil {
			break
		}

		downloadSem <- struct{}{} // acquire slot
		downloadWG.Add(1)

		go func(idx int, cb datastore.ServerCookbook) {
			defer func() {
				<-downloadSem // release slot
				downloadWG.Done()
			}()

			if ctx.Err() != nil {
				return
			}

			destDir, ok := resolveCookbookDir(ctx, client, db, log, cb, cookbookCacheDir, deleteAfterScan, idx+1, len(cookbooks))
			if !ok {
				mu.Lock()
				failed++
				errors = append(errors, CookbookFetchError{
					OrganisationName: cb.OrganisationName,
					Name:             cb.Name,
					Version:          cb.Version,
					Err:              fmt.Errorf("download failed"),
				})
				mu.Unlock()
				completedCount.Add(1)
				return
			}

			if !cb.IsDownloaded() {
				mu.Lock()
				downloaded++
				mu.Unlock()
			}

			// Send to scan workers immediately.
			scanCh <- scanWorkItem{
				cookbook:        cb,
				destDir:         destDir,
				index:           idx + 1,
				deleteAfterScan: deleteAfterScan,
			}
		}(i, cb)
	}

	// Wait for all downloads to finish, then close the scan channel so
	// scan workers know there's no more work coming.
	downloadWG.Wait()
	close(scanCh)

	// Wait for all scan workers to finish.
	scanWG.Wait()

	// -----------------------------------------------------------------------
	// Legacy cache cleanup (runs after all workers are done).
	// -----------------------------------------------------------------------
	if deleteAfterScan && cookbookCacheDir != "" {
		cleaned := cleanLegacyCookbookCache(log, cookbookCacheDir, org.Name)
		result.Cleaned = cleaned
		if cleaned > 0 {
			log.Info(fmt.Sprintf("cleaned %d legacy cached cookbook directory/directories for org %s", cleaned, org.Name))
		}
	}

	// -----------------------------------------------------------------------
	// Assemble final result.
	// -----------------------------------------------------------------------
	mu.Lock()
	result.Downloaded = downloaded
	result.Scanned = scanned
	result.Skipped = skipped
	result.Failed = failed
	result.Errors = errors
	mu.Unlock()
	result.Duration = time.Since(start)

	log.Info(fmt.Sprintf("server cookbook pipeline complete: %d total, %d downloaded, %d scanned, %d skipped, %d failed in %s",
		result.Total, result.Downloaded, result.Scanned, result.Skipped, result.Failed,
		result.Duration.Round(time.Millisecond)))

	return result
}

// resolveCookbookDir determines the filesystem directory for a cookbook,
// downloading it if necessary. Returns the directory path and true on
// success, or ("", false) on failure (already logged and marked in DB).
func resolveCookbookDir(
	ctx context.Context,
	client *chefapi.Client,
	db *datastore.DB,
	log *logging.ScopedLogger,
	cb datastore.ServerCookbook,
	cookbookCacheDir string,
	deleteAfterScan bool,
	index, total int,
) (string, bool) {
	if cb.IsDownloaded() {
		// Already downloaded — try to find it in the cache.
		if !deleteAfterScan && cookbookCacheDir != "" {
			candidateDir := filepath.Join(cookbookCacheDir, cb.OrganisationName, cb.Name, cb.Version)
			if info, statErr := os.Stat(candidateDir); statErr == nil && info.IsDir() {
				return candidateDir, true
			}
		}
		// Cache miss (deleteAfterScan cleaned it up, or cache moved) —
		// re-download to a temp directory for scanning.
		destDir, downloadErr := downloadCookbook(ctx, client, db, cb, cookbookCacheDir, deleteAfterScan)
		if downloadErr != nil {
			log.Warn(fmt.Sprintf("[%d/%d] cookbook re-download failed: %s/%s: %v",
				index, total, cb.Name, cb.Version, downloadErr))
			return "", false
		}
		return destDir, true
	}

	// Needs download (pending or failed status).
	destDir, downloadErr := downloadCookbook(ctx, client, db, cb, cookbookCacheDir, deleteAfterScan)
	if downloadErr != nil {
		log.Warn(fmt.Sprintf("[%d/%d] cookbook download failed: %s/%s: %v",
			index, total, cb.Name, cb.Version, downloadErr))
		return "", false
	}
	return destDir, true
}

// scanAndPreview runs CookStyle scanning and autocorrect preview generation
// for a single cookbook against all target Chef versions. Returns the number
// of successful scans and skipped scans.
func scanAndPreview(
	ctx context.Context,
	db *datastore.DB,
	log *logging.ScopedLogger,
	item scanWorkItem,
	targetChefVersions []string,
	cookstyleScanner *analysis.CookstyleScanner,
	autocorrectGen *remediation.AutocorrectGenerator,
	total int,
) (scanCount, skipCount int) {
	if cookstyleScanner == nil || len(targetChefVersions) == 0 {
		return 0, 0
	}

	cb := item.cookbook

	for _, tv := range targetChefVersions {
		if ctx.Err() != nil {
			break
		}

		sr := cookstyleScanner.ScanSingleServerCookbook(ctx, cb, tv, item.destDir)
		if sr.Skipped {
			skipCount++
		} else if sr.Error != nil {
			log.Warn(fmt.Sprintf("[%d/%d] CookStyle scan failed: %s/%s target %s: %v",
				item.index, total, cb.Name, cb.Version, tv, sr.Error))
		} else {
			scanCount++

			// Autocorrect preview (only if scan produced offenses).
			if autocorrectGen != nil && sr.OffenseCount > 0 {
				dbResult, dbErr := db.GetServerCookbookCookstyleResult(ctx, cb.OrganisationName+"/"+cb.Name+"/"+cb.Version, tv)
				if dbErr == nil && dbResult != nil {
					csInfo := remediation.CookstyleResultInfo{
						ResultID:          dbResult.ID,
						CookbookID:        cb.OrganisationName + "/" + cb.Name + "/" + cb.Version,
						CookbookName:      cb.Name,
						CookbookVersion:   cb.Version,
						TargetChefVersion: tv,
						OffenseCount:      sr.OffenseCount,
						Passed:            sr.Passed,
						Source:            remediation.SourceServerCookbook,
					}
					autocorrectGen.GenerateSinglePreview(ctx, csInfo, item.destDir)
				}
			}
		}
	}

	// Per-cookbook completion log.
	if skipCount > 0 && scanCount == 0 {
		log.Debug(fmt.Sprintf("[%d/%d] %s/%s — skipped (already scanned)",
			item.index, total, cb.Name, cb.Version))
	} else if scanCount > 0 {
		log.Info(fmt.Sprintf("[%d/%d] %s/%s — %d scanned, %d skipped",
			item.index, total, cb.Name, cb.Version, scanCount, skipCount))
	}

	return scanCount, skipCount
}

// downloadCookbook downloads a single cookbook version to disk and returns
// the directory path. The caller is responsible for removing the directory
// when deleteAfterScan is true.
//
// When deleteAfterScan is false and cookbookCacheDir is non-empty, files
// are written to the persistent cache layout:
//
//	<cookbookCacheDir>/<org_id>/<name>/<version>/
//
// This keeps cookbooks on disk so they can be re-scanned when CookStyle is
// upgraded without re-downloading from the Chef Server.
//
// When deleteAfterScan is true, os.MkdirTemp is used so files live in a
// true temp location that is fully removed after scanning.
//
// On success, marks the cookbook as download_status = 'ok' in the database.
func downloadCookbook(
	ctx context.Context,
	client *chefapi.Client,
	db *datastore.DB,
	cb datastore.ServerCookbook,
	cookbookCacheDir string,
	deleteAfterScan bool,
) (string, error) {
	var destDir string
	var err error

	if !deleteAfterScan && cookbookCacheDir != "" {
		// Persistent cache: <cookbookCacheDir>/<org_name>/<name>/<version>/
		destDir = filepath.Join(cookbookCacheDir, cb.OrganisationName, cb.Name, cb.Version)
		if err = os.MkdirAll(destDir, 0o750); err != nil {
			markDownloadFailed(ctx, db, cb.OrganisationName, cb.Name, cb.Version, err)
			return "", fmt.Errorf("creating cache directory %s: %w", destDir, err)
		}
	} else {
		// Ephemeral temp directory — removed by caller after scanning.
		destDir, err = os.MkdirTemp("", fmt.Sprintf("cmm-cb-%s-%s-*", cb.Name, cb.Version))
		if err != nil {
			markDownloadFailed(ctx, db, cb.OrganisationName, cb.Name, cb.Version, err)
			return "", fmt.Errorf("creating temp directory: %w", err)
		}
	}

	// Fetch manifest.
	manifest, mErr := client.GetCookbookVersionManifest(ctx, cb.Name, cb.Version)
	if mErr != nil {
		_ = os.RemoveAll(destDir)
		markDownloadFailed(ctx, db, cb.OrganisationName, cb.Name, cb.Version, mErr)
		return "", mErr
	}

	// Extract files.
	_, extractErr := extractCookbookFiles(ctx, client, manifest, destDir)
	if extractErr != nil {
		_ = os.RemoveAll(destDir)
		markDownloadFailed(ctx, db, cb.OrganisationName, cb.Name, cb.Version, extractErr)
		return "", extractErr
	}

	// Parse and persist cookbook metadata from the manifest. This populates
	// maintainer, description, license, platforms, dependencies, and the
	// frozen flag on the server_cookbooks row. Non-fatal — if parsing or
	// persistence fails, the cookbook is still usable for scanning.
	meta, metaErr := manifest.ParseMetadata()
	if metaErr == nil {
		platformsJSON, _ := json.Marshal(meta.Platforms)
		dependenciesJSON, _ := json.Marshal(meta.Dependencies)
		_, _ = db.UpdateServerCookbookMetadata(ctx, datastore.UpdateServerCookbookMetadataParams{
			OrganisationName: cb.OrganisationName,
			Name:             cb.Name,
			Version:          cb.Version,
			IsFrozen:         manifest.Frozen,
			Maintainer:       meta.Maintainer,
			Description:      meta.Description,
			LongDescription:  meta.LongDescription,
			License:          meta.License,
			Platforms:        platformsJSON,
			Dependencies:     dependenciesJSON,
		})
	}

	// Mark as downloaded.
	if _, markErr := db.MarkServerCookbookDownloadOK(ctx, cb.OrganisationName, cb.Name, cb.Version); markErr != nil {
		// Non-fatal — files are on disk and scannable even if the DB
		// status update fails.
		_ = markErr
	}

	return destDir, nil
}

// cleanLegacyCookbookCache removes server cookbook files from the persistent
// cache directory. This is only called when deleteAfterScan is true —
// the operator has explicitly opted to discard cookbook files after scanning.
//
// The function walks the org-specific subdirectory and removes cookbook
// version directories (the leaf level: <name>/<version>/), then prunes
// empty parent directories (<name>/ and <orgID>/) bottom-up.
//
// Returns the number of version directories removed.
func cleanLegacyCookbookCache(log *logging.ScopedLogger, cookbookCacheDir string, orgID string) int {
	orgDir := filepath.Join(cookbookCacheDir, orgID)

	info, err := os.Stat(orgDir)
	if err != nil || !info.IsDir() {
		// No cached files for this org — nothing to clean.
		return 0
	}

	cleaned := 0

	// Walk the directory tree: <orgDir>/<cookbookName>/<version>/
	// We collect version directories first, then remove them, then
	// prune empty parents.
	cookbookNames, readErr := os.ReadDir(orgDir)
	if readErr != nil {
		log.Warn(fmt.Sprintf("failed to read legacy cookbook cache directory %s: %v", orgDir, readErr))
		return 0
	}

	for _, nameEntry := range cookbookNames {
		if !nameEntry.IsDir() {
			continue
		}

		nameDir := filepath.Join(orgDir, nameEntry.Name())
		versions, vReadErr := os.ReadDir(nameDir)
		if vReadErr != nil {
			log.Warn(fmt.Sprintf("failed to read cookbook directory %s: %v", nameDir, vReadErr))
			continue
		}

		for _, versionEntry := range versions {
			if !versionEntry.IsDir() {
				continue
			}

			versionDir := filepath.Join(nameDir, versionEntry.Name())
			if removeErr := os.RemoveAll(versionDir); removeErr != nil {
				log.Warn(fmt.Sprintf("failed to remove legacy cookbook cache %s: %v", versionDir, removeErr))
			} else {
				log.Debug(fmt.Sprintf("removed legacy cached cookbook: %s/%s", nameEntry.Name(), versionEntry.Name()))
				cleaned++
			}
		}

		// Prune the cookbook name directory if it is now empty.
		removeEmptyDir(nameDir)
	}

	// Prune the org directory if it is now empty.
	removeEmptyDir(orgDir)

	return cleaned
}

// removeEmptyDir removes a directory only if it is empty. This is used
// for bottom-up pruning of parent directories after removing cookbook
// version directories. If the directory is not empty or cannot be read,
// it is left in place (non-fatal).
func removeEmptyDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		_ = os.Remove(dir) // os.Remove only removes empty directories
	}
}
