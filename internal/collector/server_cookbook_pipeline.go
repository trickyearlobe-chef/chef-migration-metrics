// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
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
	cookbooks, err := db.ListActiveCookbooksNeedingDownload(ctx, org.ID)
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
		result.Duration = time.Since(start)
		return result
	}

	log.Info(fmt.Sprintf("streaming server cookbook pipeline: %d version(s) to process", len(cookbooks)))

	// Log progress every 25 cookbooks so operators can monitor long runs.
	const progressInterval = 25

	for i, cb := range cookbooks {
		if ctx.Err() != nil {
			break
		}

		cbStart := time.Now()

		// Step 1: Download to a temporary directory.
		tmpDir, downloadErr := downloadToTempDir(ctx, client, db, cb, cookbookCacheDir)
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

				sr := cookstyleScanner.ScanSingleCookbook(ctx, cb, tv, tmpDir)
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
						dbResult, dbErr := db.GetCookstyleResult(ctx, cb.ID, tv)
						if dbErr == nil && dbResult != nil {
							csInfo := remediation.CookstyleResultInfo{
								ResultID:          dbResult.ID,
								CookbookID:        cb.ID,
								CookbookName:      cb.Name,
								CookbookVersion:   cb.Version,
								TargetChefVersion: tv,
								OffenseCount:      sr.OffenseCount,
								Passed:            sr.Passed,
							}
							autocorrectGen.GenerateSinglePreview(ctx, csInfo, tmpDir)
						}
					}
				}
			}
		}

		// Step 4: Delete the downloaded files.
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

	result.Duration = time.Since(start)
	return result
}

// downloadToTempDir downloads a single cookbook version to disk and returns
// the directory path. If cookbookCacheDir is set, uses the standard layout
// (<cacheDir>/<org_id>/<name>/<version>/); otherwise uses a temp directory.
// On success, marks the cookbook as download_status = 'ok'.
func downloadToTempDir(
	ctx context.Context,
	client *chefapi.Client,
	db *datastore.DB,
	cb datastore.Cookbook,
	cookbookCacheDir string,
) (string, error) {
	// Determine destination directory.
	var destDir string
	if cookbookCacheDir != "" {
		destDir = filepath.Join(cookbookCacheDir, cb.OrganisationID, cb.Name, cb.Version)
	} else {
		tmpBase, err := os.MkdirTemp("", "cmm-cookbook-*")
		if err != nil {
			markDownloadFailed(ctx, db, cb.ID, err)
			return "", fmt.Errorf("creating temp directory: %w", err)
		}
		destDir = tmpBase
	}

	// Fetch manifest.
	manifest, err := client.GetCookbookVersionManifest(ctx, cb.Name, cb.Version)
	if err != nil {
		markDownloadFailed(ctx, db, cb.ID, err)
		return "", err
	}

	// Extract files.
	_, extractErr := extractCookbookFiles(ctx, client, manifest, destDir)
	if extractErr != nil {
		_ = os.RemoveAll(destDir)
		markDownloadFailed(ctx, db, cb.ID, extractErr)
		return "", extractErr
	}

	// Mark as downloaded.
	if _, markErr := db.MarkCookbookDownloadOK(ctx, cb.ID); markErr != nil {
		// Non-fatal — files are on disk and scannable even if the DB
		// status update fails.
		_ = markErr
	}

	return destDir, nil
}
