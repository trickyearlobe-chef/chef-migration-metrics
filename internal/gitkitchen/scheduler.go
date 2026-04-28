// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package gitkitchen

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Sentinel errors for RunOne.
var (
	ErrInstanceNotFound   = errors.New("gitkitchen: instance not found in plan")
	ErrInstanceNotRunnable = errors.New("gitkitchen: instance is not runnable")
)

// ResultStore abstracts the datastore methods needed by the scheduler.
type ResultStore interface {
	UpsertGitKitchenResult(ctx context.Context, p datastore.UpsertGitKitchenResultParams) (datastore.GitKitchenResult, error)
}

// SchedulerConfig holds scheduler settings.
type SchedulerConfig struct {
	MaxConcurrency    int
	TargetChefVersion string
}

// ProgressCallback is called after each instance completes.
type ProgressCallback func(completed, total int, instance PlannedInstance, result RunInstanceResult)

// BatchProgressCallback is called after each instance in a batch completes.
// It extends ProgressCallback with the repo name for multi-repo batches.
type BatchProgressCallback func(completed, total int, repoName string, instance PlannedInstance, result RunInstanceResult)

// batchWorkItem pairs a planned instance with its parent plan.
type batchWorkItem struct {
	plan     *PlanResult
	instance PlannedInstance
}

// RunAllResult holds aggregate results from RunAll.
type RunAllResult struct {
	Total     int
	Executed  int
	Passed    int
	Failed    int
	Errors    int
	Cancelled int
	Duration  time.Duration
}

// RunOneResult holds the result of running a single instance.
type RunOneResult struct {
	Instance PlannedInstance
	Result   RunInstanceResult
	DBResult datastore.GitKitchenResult
}

// InstanceRunner executes a single kitchen instance. The default implementation
// calls RunInstance; tests can substitute a mock.
type InstanceRunner func(ctx context.Context, params RunInstanceParams, tkConfig config.TestKitchenConfig,
	executor KitchenExecutor, credResolver CredentialResolver) RunInstanceResult

// Scheduler runs planned instances with bounded concurrency.
type Scheduler struct {
	executor     KitchenExecutor
	credResolver CredentialResolver
	store        ResultStore
	repoDirFn    func(name, url string) string
	runFn        InstanceRunner // injectable for testing
}

// SchedulerOption configures a Scheduler.
type SchedulerOption func(*Scheduler)

// WithRunFn overrides the instance runner. Use in tests to avoid real OS commands.
func WithRunFn(fn InstanceRunner) SchedulerOption {
	return func(s *Scheduler) { s.runFn = fn }
}

// NewScheduler creates a new Scheduler.
func NewScheduler(executor KitchenExecutor, credResolver CredentialResolver,
	store ResultStore,
	repoDirFn func(name, url string) string, opts ...SchedulerOption) *Scheduler {
	s := &Scheduler{
		executor:     executor,
		credResolver: credResolver,
		store:        store,
		repoDirFn:    repoDirFn,
		runFn:        RunInstance,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RunAll runs all mapped instances from a PlanResult with bounded concurrency.
func (s *Scheduler) RunAll(ctx context.Context, plan *PlanResult, cfg SchedulerConfig,
	tkConfig config.TestKitchenConfig, onProgress ProgressCallback) (*RunAllResult, error) {

	start := time.Now()

	// Filter to mapped instances only.
	var mapped []PlannedInstance
	for _, inst := range plan.Instances {
		if inst.Status == InstanceStatusMapped {
			mapped = append(mapped, inst)
		}
	}

	concurrency := cfg.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	result := &RunAllResult{Total: len(mapped)}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	var mu sync.Mutex
	var completed int32
	var passed, failed, errCount, cancelled int64

	for _, inst := range mapped {
		inst := inst
		wg.Add(1)

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Done()
			atomic.AddInt64(&cancelled, 1)
			continue
		}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			params := RunInstanceParams{
				GitRepoName:       plan.GitRepoName,
				GitRepoURL:        plan.GitRepoURL,
				RepoDir:           s.repoDirFn(plan.GitRepoName, plan.GitRepoURL),
				InstanceName:      inst.InstanceName,
				SuiteName:         inst.SuiteName,
				PlatformName:      inst.PlatformName,
				TargetChefVersion: cfg.TargetChefVersion,
				CommitSHA:         plan.CommitSHA,
			}

			runResult := s.runFn(ctx, params, tkConfig, s.executor, s.credResolver)

			// Classify result.
			switch {
			case ctx.Err() != nil && runResult.Passed == nil:
				atomic.AddInt64(&cancelled, 1)
			case runResult.ErrorMessage != "" && runResult.Passed == nil:
				atomic.AddInt64(&errCount, 1)
			case runResult.Passed != nil && *runResult.Passed:
				atomic.AddInt64(&passed, 1)
			case runResult.Passed != nil && !*runResult.Passed:
				atomic.AddInt64(&failed, 1)
			default:
				atomic.AddInt64(&errCount, 1)
			}

			// Persist result.
			upsertParams := buildUpsertParams(plan, inst, cfg, runResult)
			_, storeErr := s.store.UpsertGitKitchenResult(ctx, upsertParams)
			if storeErr != nil {
				// Count store errors as infrastructure errors if the run itself succeeded.
				if runResult.Passed != nil {
					// Reclassify: undo the pass/fail count, count as error.
					if *runResult.Passed {
						atomic.AddInt64(&passed, -1)
					} else {
						atomic.AddInt64(&failed, -1)
					}
					atomic.AddInt64(&errCount, 1)
				}
			}

			// Progress callback.
			if onProgress != nil {
				c := int(atomic.AddInt32(&completed, 1))
				mu.Lock()
				onProgress(c, len(mapped), inst, runResult)
				mu.Unlock()
			} else {
				atomic.AddInt32(&completed, 1)
			}
		}()
	}

	wg.Wait()

	result.Passed = int(atomic.LoadInt64(&passed))
	result.Failed = int(atomic.LoadInt64(&failed))
	result.Errors = int(atomic.LoadInt64(&errCount))
	result.Cancelled = int(atomic.LoadInt64(&cancelled))
	result.Executed = result.Passed + result.Failed + result.Errors
	result.Duration = time.Since(start)

	return result, nil
}

// RunOne runs a single specific instance from the plan.
func (s *Scheduler) RunOne(ctx context.Context, plan *PlanResult, instanceName string,
	cfg SchedulerConfig, tkConfig config.TestKitchenConfig) (*RunOneResult, error) {

	var found *PlannedInstance
	for i := range plan.Instances {
		if plan.Instances[i].InstanceName == instanceName {
			found = &plan.Instances[i]
			break
		}
	}

	if found == nil {
		return nil, fmt.Errorf("%w: %q", ErrInstanceNotFound, instanceName)
	}

	if found.Status != InstanceStatusMapped {
		return nil, fmt.Errorf("%w: instance %q has status %q (%s)",
			ErrInstanceNotRunnable, instanceName, found.Status, found.StatusReason)
	}

	params := RunInstanceParams{
		GitRepoName:       plan.GitRepoName,
		GitRepoURL:        plan.GitRepoURL,
		RepoDir:           s.repoDirFn(plan.GitRepoName, plan.GitRepoURL),
		InstanceName:      found.InstanceName,
		SuiteName:         found.SuiteName,
		PlatformName:      found.PlatformName,
		TargetChefVersion: cfg.TargetChefVersion,
		CommitSHA:         plan.CommitSHA,
	}

	runResult := s.runFn(ctx, params, tkConfig, s.executor, s.credResolver)

	upsertParams := buildUpsertParams(plan, *found, cfg, runResult)
	dbResult, err := s.store.UpsertGitKitchenResult(ctx, upsertParams)
	if err != nil {
		return nil, fmt.Errorf("gitkitchen: persisting result: %w", err)
	}

	return &RunOneResult{
		Instance: *found,
		Result:   runResult,
		DBResult: dbResult,
	}, nil
}

// RunBatch runs mapped instances from multiple repos with bounded concurrency.
// Unlike RunAll (single repo), this flattens all mapped instances across
// repos into a single work queue sharing one semaphore for optimal utilisation.
//
// Cancellation is graceful: when ctx is cancelled, new work is not scheduled
// but in-flight instances run to completion on a detached context so VMs are
// properly destroyed.
func (s *Scheduler) RunBatch(ctx context.Context, plans []*PlanResult, cfg SchedulerConfig,
	tkConfig config.TestKitchenConfig, onProgress BatchProgressCallback) (*RunAllResult, error) {

	start := time.Now()

	// Flatten all mapped instances into a single work queue.
	var items []batchWorkItem
	for _, plan := range plans {
		for _, inst := range plan.Instances {
			if inst.Status == InstanceStatusMapped {
				items = append(items, batchWorkItem{plan: plan, instance: inst})
			}
		}
	}

	concurrency := cfg.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	result := &RunAllResult{Total: len(items)}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	var mu sync.Mutex
	var completed int32
	var passed, failed, errCount, cancelled int64

	for _, item := range items {
		item := item
		wg.Add(1)

		// Acquire semaphore slot or bail on context cancellation.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Done()
			atomic.AddInt64(&cancelled, 1)
			continue
		}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			// Use a detached context for the actual run so in-flight
			// instances complete cleanup even if the batch is cancelled.
			runCtx := context.WithoutCancel(ctx)

			params := RunInstanceParams{
				GitRepoName:       item.plan.GitRepoName,
				GitRepoURL:        item.plan.GitRepoURL,
				RepoDir:           s.repoDirFn(item.plan.GitRepoName, item.plan.GitRepoURL),
				InstanceName:      item.instance.InstanceName,
				SuiteName:         item.instance.SuiteName,
				PlatformName:      item.instance.PlatformName,
				TargetChefVersion: cfg.TargetChefVersion,
				CommitSHA:         item.plan.CommitSHA,
			}

			runResult := s.runFn(runCtx, params, tkConfig, s.executor, s.credResolver)

			// Classify result.
			switch {
			case ctx.Err() != nil && runResult.Passed == nil:
				atomic.AddInt64(&cancelled, 1)
			case runResult.ErrorMessage != "" && runResult.Passed == nil:
				atomic.AddInt64(&errCount, 1)
			case runResult.Passed != nil && *runResult.Passed:
				atomic.AddInt64(&passed, 1)
			case runResult.Passed != nil && !*runResult.Passed:
				atomic.AddInt64(&failed, 1)
			default:
				atomic.AddInt64(&errCount, 1)
			}

			// Persist result.
			upsertParams := buildUpsertParams(item.plan, item.instance, cfg, runResult)
			_, storeErr := s.store.UpsertGitKitchenResult(runCtx, upsertParams)
			if storeErr != nil {
				if runResult.Passed != nil {
					if *runResult.Passed {
						atomic.AddInt64(&passed, -1)
					} else {
						atomic.AddInt64(&failed, -1)
					}
					atomic.AddInt64(&errCount, 1)
				}
			}

			// Progress callback.
			if onProgress != nil {
				c := int(atomic.AddInt32(&completed, 1))
				mu.Lock()
				onProgress(c, len(items), item.plan.GitRepoName, item.instance, runResult)
				mu.Unlock()
			} else {
				atomic.AddInt32(&completed, 1)
			}
		}()
	}

	wg.Wait()

	result.Passed = int(atomic.LoadInt64(&passed))
	result.Failed = int(atomic.LoadInt64(&failed))
	result.Errors = int(atomic.LoadInt64(&errCount))
	result.Cancelled = int(atomic.LoadInt64(&cancelled))
	result.Executed = result.Passed + result.Failed + result.Errors
	result.Duration = time.Since(start)

	return result, nil
}

// buildUpsertParams converts run outputs to datastore params.
func buildUpsertParams(plan *PlanResult, instance PlannedInstance, cfg SchedulerConfig, result RunInstanceResult) datastore.UpsertGitKitchenResultParams {
	now := time.Now()
	return datastore.UpsertGitKitchenResultParams{
		GitRepoName:       plan.GitRepoName,
		GitRepoURL:        plan.GitRepoURL,
		TargetChefVersion: cfg.TargetChefVersion,
		CommitSHA:         plan.CommitSHA,
		PlatformName:      instance.PlatformName,
		SuiteName:         instance.SuiteName,
		InstanceName:      instance.InstanceName,
		DriverUsed:        result.DriverUsed,
		Passed:            result.Passed,
		TimedOut:          result.TimedOut,
		Output:            result.Output,
		DurationSeconds:   result.DurationSeconds,
		ErrorMessage:      result.ErrorMessage,
		CompletedAt:       &now,
	}
}
