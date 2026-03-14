// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ServerCookbookPipelineResult summarises the streaming download-scan-delete
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

// runServerCookbookPipeline processes server cookbooks one at a time:
// download → CookStyle scan → autocorrect preview → delete from disk.
// This keeps disk usage to a single cookbook at a time, instead of
// downloading all cookbooks and leaving them on disk permanently.
//
// Cookbooks that already have CookStyle results are not re-downloaded
// (the scan skip check inside scanOne handles immutability caching).
// Cookbooks that are already downloaded (status = 'ok') but lack scan
// results ARE re-downloaded to a temp location, scanned, and deleted.
//
// After the pipeline finishes processing pending cookbooks, it runs a
// cleanup pass that removes any server cookbook files left on disk in the
// cache directory from previous runs (before the streaming pipeline was
// deployed). This ensures the disk is not permanently consumed by
// thousands of cookbook versions that will never be read again.
//
// If cookstyleScanner is nil, the function falls back to download-only
// behaviour (equivalent to the old fetchCookbooks, without deletion).
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
) ServerCookbookPipelineResult {
	start := time.Now()
	result := ServerCookbookPipelineResult{}

	// Get all active server cookbooks for this organisation — not just
	// those needing download. We want to scan any that haven't been
	// scanned yet, even if they were previously downloaded.
	cookbooks, err := db.ListActiveServerCookbooksNeedingDownload(ctx, org.ID)
	if err != nil {
		log.Error(fmt.Sprintf("failed to list cookbooks for pipeline: %v", err))
		result.Duration = time.Since(start)
		result.Errors = append(result.Errors, CookbookFetchError{
			Err: fmt.Errorf("listing cookbooks: %w", err),
		})
		return result
	}

	result.Total = len(cookbooks)

	if len(cookbooks) > 0 {
		log.Info(fmt.Sprintf("streaming server cookbook pipeline: %d version(s) to process", len(cookbooks)))

		// Log progress every 25 cookbooks so operators can monitor long runs.
		const progressInterval = 25

		for i, cb := range cookbooks {
			if ctx.Err() != nil {
				break
			}

			cbStart := time.Now()

			// Step 1: Download to a temporary directory. Always uses
			// os.MkdirTemp so the files live in a true temp location that
			// is fully removed (including the directory itself) after
			// scanning. This avoids accumulating files in the persistent
			// cache directory.
			tmpDir, downloadErr := downloadToTempDir(ctx, client, db, cb)
			if downloadErr != nil {
				result.Failed++
				result.Errors = append(result.Errors, CookbookFetchError{
					CookbookID: cb.ID,
					Name:       cb.Name,
					Version:    cb.Version,
					Err:        downloadErr,
				})
				log.Warn(fmt.Sprintf("[%d/%d] cookbook download failed: %s/%s: %v",
					i+1, len(cookbooks), cb.Name, cb.Version, downloadErr))
				continue
			}
			result.Downloaded++

			// Step 2: CookStyle scan for each target version.
			scanCount := 0
			skipCount := 0
			if cookstyleScanner != nil && len(targetChefVersions) > 0 {
				for _, tv := range targetChefVersions {
					if ctx.Err() != nil {
						break
					}

					sr := cookstyleScanner.ScanSingleServerCookbook(ctx, cb, tv, tmpDir)
					if sr.Skipped {
						result.Skipped++
						skipCount++
					} else if sr.Error != nil {
						log.Warn(fmt.Sprintf("[%d/%d] CookStyle scan failed: %s/%s target %s: %v",
							i+1, len(cookbooks), cb.Name, cb.Version, tv, sr.Error))
					} else {
						result.Scanned++
						scanCount++

						// Step 3: Autocorrect preview (only if scan produced offenses).
						if autocorrectGen != nil && sr.OffenseCount > 0 {
							// Look up the persisted cookstyle result ID (scanOne persists it).
							dbResult, dbErr := db.GetServerCookbookCookstyleResult(ctx, cb.ID, tv)
							if dbErr == nil && dbResult != nil {
								csInfo := remediation.CookstyleResultInfo{
									ResultID:          dbResult.ID,
									CookbookID:        cb.ID,
									CookbookName:      cb.Name,
									CookbookVersion:   cb.Version,
									TargetChefVersion: tv,
									OffenseCount:      sr.OffenseCount,
									Passed:            sr.Passed,
									Source:            remediation.SourceServerCookbook,
								}
								autocorrectGen.GenerateSinglePreview(ctx, csInfo, tmpDir)
							}
						}
					}
				}
			}

			// Step 4: Delete the downloaded files. Because we used
			// os.MkdirTemp, this removes the entire temp directory —
			// no empty parent directories are left behind.
			if tmpDir != "" {
				if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
					log.Warn(fmt.Sprintf("[%d/%d] failed to clean up cookbook files %s/%s at %s: %v",
						i+1, len(cookbooks), cb.Name, cb.Version, tmpDir, removeErr))
				}
			}

			// Per-cookbook completion log.
			elapsed := time.Since(cbStart).Round(time.Millisecond)
			if skipCount > 0 && scanCount == 0 {
				log.Debug(fmt.Sprintf("[%d/%d] %s/%s — skipped (already scanned) in %s",
					i+1, len(cookbooks), cb.Name, cb.Version, elapsed))
			} else {
				log.Info(fmt.Sprintf("[%d/%d] %s/%s — downloaded, %d scanned, %d skipped in %s",
					i+1, len(cookbooks), cb.Name, cb.Version, scanCount, skipCount, elapsed))
			}

			// Periodic progress summary.
			if (i+1)%progressInterval == 0 {
				totalElapsed := time.Since(start).Round(time.Second)
				log.Info(fmt.Sprintf("pipeline progress: %d/%d processed (%d downloaded, %d scanned, %d skipped, %d failed) in %s",
					i+1, len(cookbooks), result.Downloaded, result.Scanned, result.Skipped, result.Failed, totalElapsed))
			}
		}
	} else {
		log.Info("no server cookbook versions need processing")
	}

	// Step 5: Clean up legacy cached cookbook files. Before the streaming
	// pipeline was introduced, fetchCookbooks downloaded every cookbook to
	// the persistent cache directory and never removed them. Cookbooks
	// that were already processed (download_status = 'ok') are never
	// returned by ListActiveCookbooksNeedingDownload, so the loop above
	// never sees them and their files remain on disk indefinitely.
	//
	// Walk the cache directory for this organisation and remove any server
	// cookbook version directories that still exist. The streaming pipeline
	// no longer writes to the cache directory (it uses os.MkdirTemp), so
	// anything present is a leftover from the old code path.
	if cookbookCacheDir != "" {
		cleaned := cleanLegacyCookbookCache(log, cookbookCacheDir, org.ID)
		result.Cleaned = cleaned
		if cleaned > 0 {
			log.Info(fmt.Sprintf("cleaned %d legacy cached cookbook directory/directories for org %s", cleaned, org.Name))
		}
	}

	result.Duration = time.Since(start)
	return result
}

// downloadToTempDir downloads a single cookbook version to a temporary
// directory and returns the path. The caller is responsible for removing
// the directory after use (os.RemoveAll).
//
// The function always uses os.MkdirTemp to create a fresh temporary
// directory. This ensures the files are completely removed after scanning —
// no empty parent directories or stale cache entries are left behind.
//
// On success, marks the cookbook as download_status = 'ok' in the database.
func downloadToTempDir(
	ctx context.Context,
	client *chefapi.Client,
	db *datastore.DB,
	cb datastore.ServerCookbook,
) (string, error) {
	// Always use a true temp directory. The streaming pipeline deletes
	// these files immediately after scanning, so there is no benefit to
	// using the persistent cache layout — and doing so would leave empty
	// parent directories on disk.
	destDir, err := os.MkdirTemp("", fmt.Sprintf("cmm-cb-%s-%s-*", cb.Name, cb.Version))
	if err != nil {
		markDownloadFailed(ctx, db, cb.ID, err)
		return "", fmt.Errorf("creating temp directory: %w", err)
	}

	// Fetch manifest.
	manifest, mErr := client.GetCookbookVersionManifest(ctx, cb.Name, cb.Version)
	if mErr != nil {
		_ = os.RemoveAll(destDir)
		markDownloadFailed(ctx, db, cb.ID, mErr)
		return "", mErr
	}

	// Extract files.
	_, extractErr := extractCookbookFiles(ctx, client, manifest, destDir)
	if extractErr != nil {
		_ = os.RemoveAll(destDir)
		markDownloadFailed(ctx, db, cb.ID, extractErr)
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
			ID:              cb.ID,
			IsFrozen:        manifest.Frozen,
			Maintainer:      meta.Maintainer,
			Description:     meta.Description,
			LongDescription: meta.LongDescription,
			License:         meta.License,
			Platforms:       platformsJSON,
			Dependencies:    dependenciesJSON,
		})
	}

	// Mark as downloaded.
	if _, markErr := db.MarkServerCookbookDownloadOK(ctx, cb.ID); markErr != nil {
		// Non-fatal — files are on disk and scannable even if the DB
		// status update fails.
		_ = markErr
	}

	return destDir, nil
}

// cleanLegacyCookbookCache removes server cookbook files that were
// downloaded to the persistent cache directory by the old fetchCookbooks
// code path (before the streaming pipeline was introduced). The streaming
// pipeline now uses os.MkdirTemp exclusively, so any files under
// <cookbookCacheDir>/<orgID>/ are leftovers that will never be read again.
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
