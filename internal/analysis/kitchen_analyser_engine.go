// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// KitchenAnalyserStore is the minimal DB interface needed by the analyser.
type KitchenAnalyserStore interface {
	UpsertKitchenAnalysisResult(ctx context.Context, p datastore.UpsertKitchenAnalysisResultParams) (*datastore.KitchenAnalysisResult, error)
	RebuildDiscoveredPlatforms(ctx context.Context) error
}

// KitchenAnalyser performs static analysis of Test Kitchen configuration
// files discovered in git repositories and persists the results.
type KitchenAnalyser struct {
	db          KitchenAnalyserStore
	logger      *logging.Logger
	concurrency int
}

// KitchenAnalysisBatchResult summarises a batch analysis run.
type KitchenAnalysisBatchResult struct {
	Total    int
	Analysed int
	Skipped  int
	Errors   int
	Duration time.Duration
}

// NewKitchenAnalyser creates a KitchenAnalyser. Concurrency defaults to 4
// if the supplied value is <= 0.
func NewKitchenAnalyser(db KitchenAnalyserStore, logger *logging.Logger, concurrency int) *KitchenAnalyser {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &KitchenAnalyser{
		db:          db,
		logger:      logger,
		concurrency: concurrency,
	}
}

// AnalyseRepo analyses a single git repo's kitchen configuration and upserts
// the result. It returns (nil, nil) when the repo directory is empty (not
// cloned). Parse errors are recorded in the ErrorMessage field rather than
// causing a failure.
func (a *KitchenAnalyser) AnalyseRepo(ctx context.Context, repo datastore.GitRepo, dirFn func(datastore.GitRepo) string) (*datastore.KitchenAnalysisResult, error) {
	dir := dirFn(repo)
	if dir == "" {
		return nil, nil
	}

	entry := AnalyseKitchenDir(dir)

	params, err := buildUpsertParams(repo, entry)
	if err != nil {
		return nil, fmt.Errorf("kitchen analyser: building params for %s: %w", repo.Name, err)
	}

	result, err := a.db.UpsertKitchenAnalysisResult(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("kitchen analyser: upserting result for %s: %w", repo.Name, err)
	}
	return result, nil
}

// AnalyseAll analyses all repos concurrently and rebuilds the discovered
// platforms view afterwards.
func (a *KitchenAnalyser) AnalyseAll(ctx context.Context, repos []datastore.GitRepo, dirFn func(datastore.GitRepo) string) KitchenAnalysisBatchResult {
	start := time.Now()
	log := a.logger.WithScope(logging.ScopeCollectionRun)

	result := KitchenAnalysisBatchResult{Total: len(repos)}

	work := make(chan datastore.GitRepo, len(repos))
	for _, r := range repos {
		work <- r
	}
	close(work)

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < a.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repo := range work {
				dir := dirFn(repo)
				if dir == "" {
					mu.Lock()
					result.Skipped++
					mu.Unlock()
					continue
				}

				_, err := a.AnalyseRepo(ctx, repo, dirFn)
				mu.Lock()
				if err != nil {
					result.Errors++
					log.Warn(fmt.Sprintf("kitchen analysis failed for %s: %v", repo.Name, err))
				} else {
					result.Analysed++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if err := a.db.RebuildDiscoveredPlatforms(ctx); err != nil {
		log.Warn(fmt.Sprintf("failed to rebuild discovered platforms: %v", err))
	}

	result.Duration = time.Since(start)
	log.Info(fmt.Sprintf(
		"kitchen analysis batch: %d total, %d analysed, %d skipped, %d errors in %s",
		result.Total, result.Analysed, result.Skipped, result.Errors,
		result.Duration.Round(time.Millisecond)))

	return result
}

// buildUpsertParams converts a KitchenAnalysisEntry into datastore upsert
// parameters for a given repo.
func buildUpsertParams(repo datastore.GitRepo, entry KitchenAnalysisEntry) (datastore.UpsertKitchenAnalysisResultParams, error) {
	kitchenFiles, err := json.Marshal(entry.KitchenFiles)
	if err != nil {
		return datastore.UpsertKitchenAnalysisResultParams{}, fmt.Errorf("marshal kitchen_files: %w", err)
	}

	platforms, err := json.Marshal(entry.Config.Platforms)
	if err != nil {
		return datastore.UpsertKitchenAnalysisResultParams{}, fmt.Errorf("marshal platforms: %w", err)
	}

	suites, err := json.Marshal(entry.Config.Suites)
	if err != nil {
		return datastore.UpsertKitchenAnalysisResultParams{}, fmt.Errorf("marshal suites: %w", err)
	}

	var localOverrideKeys json.RawMessage
	if len(entry.LocalOverrideKeys) > 0 {
		localOverrideKeys, err = json.Marshal(entry.LocalOverrideKeys)
		if err != nil {
			return datastore.UpsertKitchenAnalysisResultParams{}, fmt.Errorf("marshal local_override_keys: %w", err)
		}
	}

	var extensions json.RawMessage
	allExt := collectPlatformExtensions(entry.Config.Platforms)
	if len(allExt) > 0 {
		extensions, err = json.Marshal(allExt)
		if err != nil {
			return datastore.UpsertKitchenAnalysisResultParams{}, fmt.Errorf("marshal extensions: %w", err)
		}
	}

	var variantFiles json.RawMessage
	if len(entry.VariantFiles) > 0 {
		variantFiles, err = json.Marshal(entry.VariantFiles)
		if err != nil {
			return datastore.UpsertKitchenAnalysisResultParams{}, fmt.Errorf("marshal variant_files: %w", err)
		}
	}

	var requireChefOmnibus *bool
	if v, ok := entry.Config.ProvisionerSettings["require_chef_omnibus"]; ok {
		if b, isBool := v.(bool); isBool {
			requireChefOmnibus = &b
		}
	}

	return datastore.UpsertKitchenAnalysisResultParams{
		GitRepoName:        repo.Name,
		GitRepoURL:         repo.GitRepoURL,
		HeadCommitSHA:      repo.HeadCommitSHA,
		AnalysedAt:         time.Now().UTC(),
		KitchenFiles:       kitchenFiles,
		HasLocalOverride:   entry.HasLocalOverride,
		LocalOverrideKeys:  localOverrideKeys,
		DriverName:         entry.Config.DriverName,
		ProvisionerName:    entry.Config.ProvisionerName,
		RequireChefOmnibus: requireChefOmnibus,
		Platforms:          platforms,
		Suites:             suites,
		TransportType:      entry.Config.TransportType,
		Extensions:         extensions,
		VariantFiles:       variantFiles,
		ErrorMessage:       entry.ErrorMessage,
	}, nil
}

// collectPlatformExtensions gathers all non-empty extensions from platforms.
func collectPlatformExtensions(platforms []KitchenPlatform) map[string]map[string]any {
	out := make(map[string]map[string]any)
	for _, p := range platforms {
		if len(p.Extensions) > 0 {
			out[p.Name] = p.Extensions
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
