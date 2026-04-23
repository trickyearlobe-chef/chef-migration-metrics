// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// InstanceRunner runs a single kitchen instance (cookbook × platform × suite).
type InstanceRunner interface {
	RunInstance(ctx context.Context, req RunInstanceRequest) RunInstanceResult
}

// RunInstanceRequest defines a single kitchen test instance to execute.
type RunInstanceRequest struct {
	BatchID           string
	GitRepoName       string
	GitRepoURL        string
	CommitSHA         string
	TargetChefVersion string
	PlatformName      string
	SuiteName         string
}

// RunInstanceResult is the outcome of a single kitchen instance run.
type RunInstanceResult struct {
	ConvergePassed  *bool
	TestsPassed     *bool
	TimedOut        bool
	ConvergeOutput  string
	VerifyOutput    string
	DestroyOutput   string
	DurationSeconds *int
	ErrorMessage    string
	TemplateUsed    string
	DriverUsed      string
	StartedAt       *time.Time
	CompletedAt     *time.Time
}

// BatchResultStore persists per-instance results and manages batch status.
type BatchResultStore interface {
	UpsertGitKitchenResult(ctx context.Context, p UpsertResultParams) error
	UpdateBatchStatus(ctx context.Context, batchID string, status string, now time.Time) error
}

// UpsertResultParams maps to the datastore UpsertGitKitchenResultParams.
// Defined here to avoid coupling batch package to datastore.
type UpsertResultParams struct {
	BatchID           string
	GitRepoName       string
	GitRepoURL        string
	TargetChefVersion string
	CommitSHA         string
	PlatformName      string
	SuiteName         string
	TemplateUsed      string
	DriverUsed        string
	ConvergePassed    *bool
	TestsPassed       *bool
	TimedOut          bool
	ConvergeOutput    string
	VerifyOutput      string
	DestroyOutput     string
	DurationSeconds   *int
	ErrorMessage      string
	StartedAt         *time.Time
	CompletedAt       *time.Time
}

// InstanceEnumerator provides the list of (platform, suite) instances for a cookbook.
type InstanceEnumerator interface {
	ListInstances(ctx context.Context, repoName string) ([]InstanceInfo, error)
}

// InstanceInfo represents a single kitchen instance discovered by analysis.
type InstanceInfo struct {
	PlatformName string
	SuiteName    string
}

// ExecutorLogger abstracts logging for the executor.
type ExecutorLogger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}

// Executor orchestrates batch kitchen runs with bounded concurrency.
type Executor struct {
	resolver   *Resolver
	runner     InstanceRunner
	store      BatchResultStore
	enumerator InstanceEnumerator
	logger     ExecutorLogger
}

// NewExecutor creates an Executor.
func NewExecutor(resolver *Resolver, runner InstanceRunner, store BatchResultStore, enumerator InstanceEnumerator, logger ExecutorLogger) *Executor {
	return &Executor{
		resolver:   resolver,
		runner:     runner,
		store:      store,
		enumerator: enumerator,
		logger:     logger,
	}
}

// workItem is a single unit of work for the executor.
type workItem struct {
	batchID           string
	gitRepoName       string
	gitRepoURL        string
	commitSHA         string
	targetChefVersion string
	platformName      string
	suiteName         string
}

// Execute runs a batch. It resolves the batch filters, enumerates instances
// per cookbook, fans out with bounded concurrency, persists results, and
// transitions the batch to completed (or cancelled if context is cancelled).
//
// maxConcurrent controls the worker pool size. If <= 0, defaults to 1.
// The context should be cancellable — the cancel handler cancels it.
//
// This method blocks until all instances complete or the context is cancelled.
func (e *Executor) Execute(ctx context.Context, batchID string, filters Filters, maxCount *int, maxConcurrent int, targetChefVersions []string) error {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	// Resolve which cookbooks to run.
	estimate, err := e.resolver.ResolveBatch(ctx, filters, maxCount)
	if err != nil {
		return fmt.Errorf("resolve batch: %w", err)
	}
	e.logger.Info(fmt.Sprintf("batch %s: resolved %d cookbooks", batchID, estimate.TotalCookbooks))

	// Normalise target versions.
	versions := targetChefVersions
	if len(versions) == 0 {
		versions = []string{""}
	}

	// Build work items.
	var items []workItem
	for _, cb := range estimate.Cookbooks {
		instances, enumErr := e.enumerator.ListInstances(ctx, cb.Name)
		if enumErr != nil {
			e.logger.Warn(fmt.Sprintf("batch %s: enumerator error for %s: %v", batchID, cb.Name, enumErr))
			continue
		}
		if len(instances) == 0 {
			instances = []InstanceInfo{{PlatformName: "default", SuiteName: "default"}}
		}
		for _, inst := range instances {
			for _, ver := range versions {
				items = append(items, workItem{
					batchID:           batchID,
					gitRepoName:       cb.Name,
					gitRepoURL:        cb.GitRepoURL,
					targetChefVersion: ver,
					platformName:      inst.PlatformName,
					suiteName:         inst.SuiteName,
				})
			}
		}
	}

	e.logger.Info(fmt.Sprintf("batch %s: %d work items, concurrency %d", batchID, len(items), maxConcurrent))

	// Fan out with bounded concurrency.
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)
	var completed int64

	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{}

		go func(wi workItem) {
			defer wg.Done()
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			result := e.runner.RunInstance(ctx, RunInstanceRequest{
				BatchID:           wi.batchID,
				GitRepoName:       wi.gitRepoName,
				GitRepoURL:        wi.gitRepoURL,
				CommitSHA:         wi.commitSHA,
				TargetChefVersion: wi.targetChefVersion,
				PlatformName:      wi.platformName,
				SuiteName:         wi.suiteName,
			})

			storeErr := e.store.UpsertGitKitchenResult(ctx, UpsertResultParams{
				BatchID:           wi.batchID,
				GitRepoName:       wi.gitRepoName,
				GitRepoURL:        wi.gitRepoURL,
				TargetChefVersion: wi.targetChefVersion,
				CommitSHA:         wi.commitSHA,
				PlatformName:      wi.platformName,
				SuiteName:         wi.suiteName,
				TemplateUsed:      result.TemplateUsed,
				DriverUsed:        result.DriverUsed,
				ConvergePassed:    result.ConvergePassed,
				TestsPassed:       result.TestsPassed,
				TimedOut:          result.TimedOut,
				ConvergeOutput:    result.ConvergeOutput,
				VerifyOutput:      result.VerifyOutput,
				DestroyOutput:     result.DestroyOutput,
				DurationSeconds:   result.DurationSeconds,
				ErrorMessage:      result.ErrorMessage,
				StartedAt:         result.StartedAt,
				CompletedAt:       result.CompletedAt,
			})
			if storeErr != nil {
				e.logger.Error(fmt.Sprintf("batch %s: persist result for %s/%s/%s: %v",
					wi.batchID, wi.gitRepoName, wi.platformName, wi.suiteName, storeErr))
			}

			n := atomic.AddInt64(&completed, 1)
			if n%10 == 0 || n == int64(len(items)) {
				e.logger.Info(fmt.Sprintf("batch %s: %d/%d items completed", wi.batchID, n, len(items)))
			}
		}(item)
	}

	wg.Wait()

	// Transition batch status using a background context so the update
	// persists even if the original context was cancelled.
	finalStatus := "completed"
	if ctx.Err() != nil {
		finalStatus = "cancelled"
	}

	bgCtx := context.Background()
	if statusErr := e.store.UpdateBatchStatus(bgCtx, batchID, finalStatus, time.Now()); statusErr != nil {
		e.logger.Error(fmt.Sprintf("batch %s: update status to %s: %v", batchID, finalStatus, statusErr))
	}
	e.logger.Info(fmt.Sprintf("batch %s: finished with status %s", batchID, finalStatus))

	return ctx.Err()
}
