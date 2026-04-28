// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package gitkitchen

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockResultStore struct {
	mu      sync.Mutex
	results []datastore.UpsertGitKitchenResultParams
	err     error
}

func (m *mockResultStore) UpsertGitKitchenResult(_ context.Context, p datastore.UpsertGitKitchenResultParams) (datastore.GitKitchenResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return datastore.GitKitchenResult{}, m.err
	}
	m.results = append(m.results, p)
	return datastore.GitKitchenResult{ID: fmt.Sprintf("result-%d", len(m.results))}, nil
}

// schedulerMockExecutor allows per-instance control of results.
type schedulerMockExecutor struct {
	mu       sync.Mutex
	calls    int
	results  map[string]mockExecResult // keyed by instance name
	delay    time.Duration
	maxSeen  int64 // track max concurrent runs (atomic)
	active   int64
}

type mockExecResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (m *schedulerMockExecutor) Run(ctx context.Context, _ string, _ []string, args ...string) (string, string, int, error) {
	// Track concurrency
	cur := atomic.AddInt64(&m.active, 1)
	for {
		old := atomic.LoadInt64(&m.maxSeen)
		if cur <= old || atomic.CompareAndSwapInt64(&m.maxSeen, old, cur) {
			break
		}
	}
	defer atomic.AddInt64(&m.active, -1)

	m.mu.Lock()
	m.calls++
	// args[1] is instance name (args = ["test", instanceName, ...])
	var instanceName string
	if len(args) >= 2 {
		instanceName = args[1]
	}
	r, ok := m.results[instanceName]
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", "", 0, ctx.Err()
		}
	}

	if ctx.Err() != nil {
		return "", "", 0, ctx.Err()
	}

	if !ok {
		return "ok\n", "", 0, nil
	}
	return r.stdout, r.stderr, r.exitCode, r.err
}

// schedulerMockCredResolver is a no-op credential resolver for scheduler tests.
type schedulerMockCredResolver struct{}

func (m *schedulerMockCredResolver) ResolveKitchenCredentials(_ context.Context, _ config.TestKitchenConfig) (map[string][]byte, func(), error) {
	return nil, nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testPlan(instances []PlannedInstance) *PlanResult {
	return &PlanResult{
		GitRepoName: "example-cookbook",
		GitRepoURL:  "https://git.example.com/org/example-cookbook.git",
		CommitSHA:   "abc123def",
		Instances:   instances,
		Total:       len(instances),
	}
}

func mappedInstance(name string) PlannedInstance {
	return PlannedInstance{
		InstanceName: name,
		SuiteName:    "default",
		PlatformName: "ubuntu-2204",
		Status:       InstanceStatusMapped,
		ImageName:    "ubuntu22",
	}
}

func testSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		MaxConcurrency:    2,
		TargetChefVersion: "18.4.2",
	}
}

func testTKConfig() config.TestKitchenConfig {
	return config.TestKitchenConfig{Driver: "proxmox"}
}

func newTestScheduler(exec *schedulerMockExecutor, store *mockResultStore) *Scheduler {
	s := NewScheduler(
		exec,
		&schedulerMockCredResolver{},
		store,
		func(name, url string) string { return "/repos/" + name },
	)
	// Override runFn to bypass filesystem operations and call executor directly.
	s.runFn = func(ctx context.Context, params RunInstanceParams, tkConfig config.TestKitchenConfig,
		executor KitchenExecutor, _ CredentialResolver) RunInstanceResult {
		stdout, stderr, exitCode, err := executor.Run(ctx, params.RepoDir, nil,
			"test", params.InstanceName, "--destroy=always", "--no-color")
		result := RunInstanceResult{
			Output:     stdout + stderr,
			DriverUsed: tkConfig.Driver,
		}
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("gitkitchen: executor error: %v", err)
		} else {
			passed := exitCode == 0
			result.Passed = &passed
			dur := 1
			result.DurationSeconds = &dur
		}
		return result
	}
	return s
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestScheduler_RunAll_Success(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{
		results: map[string]mockExecResult{
			"default-ubuntu-2204":  {stdout: "pass1", exitCode: 0},
			"default-centos-8":     {stdout: "pass2", exitCode: 0},
			"default-amazonlinux2": {stdout: "pass3", exitCode: 0},
		},
	}

	plan := testPlan([]PlannedInstance{
		mappedInstance("default-ubuntu-2204"),
		mappedInstance("default-centos-8"),
		mappedInstance("default-amazonlinux2"),
	})

	s := newTestScheduler(exec, store)
	result, err := s.RunAll(context.Background(), plan, testSchedulerConfig(), testTKConfig(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Executed != 3 {
		t.Errorf("expected 3 executed, got %d", result.Executed)
	}
	if result.Passed != 3 {
		t.Errorf("expected 3 passed, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", result.Failed)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.results) != 3 {
		t.Errorf("expected 3 persisted results, got %d", len(store.results))
	}
}

func TestScheduler_RunAll_MixedResults(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{
		results: map[string]mockExecResult{
			"default-ubuntu-2204": {stdout: "pass", exitCode: 0},
			"default-centos-8":    {stderr: "fail", exitCode: 1},
			"default-debian-11":   {err: errors.New("infra error")},
		},
	}

	plan := testPlan([]PlannedInstance{
		mappedInstance("default-ubuntu-2204"),
		mappedInstance("default-centos-8"),
		mappedInstance("default-debian-11"),
	})

	s := newTestScheduler(exec, store)
	result, err := s.RunAll(context.Background(), plan, testSchedulerConfig(), testTKConfig(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
	if result.Errors != 1 {
		t.Errorf("expected 1 error, got %d", result.Errors)
	}
}

func TestScheduler_RunAll_SkipsNonMapped(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{
		results: map[string]mockExecResult{
			"default-ubuntu-2204": {exitCode: 0},
		},
	}

	plan := testPlan([]PlannedInstance{
		mappedInstance("default-ubuntu-2204"),
		{InstanceName: "default-centos-8", SuiteName: "default", PlatformName: "centos-8", Status: InstanceStatusUnmapped, StatusReason: "no mapping"},
		{InstanceName: "default-debian-11", SuiteName: "default", PlatformName: "debian-11", Status: InstanceStatusSkipped, StatusReason: "skip=true"},
		{InstanceName: "default-win-2019", SuiteName: "default", PlatformName: "win-2019", Status: InstanceStatusExcluded, StatusReason: "excluded"},
	})

	s := newTestScheduler(exec, store)
	result, err := s.RunAll(context.Background(), plan, testSchedulerConfig(), testTKConfig(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("expected total=1 (mapped only), got %d", result.Total)
	}
	if result.Executed != 1 {
		t.Errorf("expected 1 executed, got %d", result.Executed)
	}

	exec.mu.Lock()
	calls := exec.calls
	exec.mu.Unlock()
	if calls != 1 {
		t.Errorf("expected executor called once, got %d", calls)
	}
}

func TestScheduler_RunAll_ContextCancelled(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{
		delay: 500 * time.Millisecond,
		results: map[string]mockExecResult{
			"default-ubuntu-2204": {exitCode: 0},
			"default-centos-8":    {exitCode: 0},
			"default-debian-11":   {exitCode: 0},
			"default-fedora-38":   {exitCode: 0},
		},
	}

	plan := testPlan([]PlannedInstance{
		mappedInstance("default-ubuntu-2204"),
		mappedInstance("default-centos-8"),
		mappedInstance("default-debian-11"),
		mappedInstance("default-fedora-38"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cfg := SchedulerConfig{MaxConcurrency: 1, TargetChefVersion: "18.4.2"}
	s := newTestScheduler(exec, store)
	result, err := s.RunAll(ctx, plan, cfg, testTKConfig(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Cancelled == 0 {
		t.Error("expected at least some cancelled instances")
	}
}

func TestScheduler_RunAll_Progress(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{
		results: map[string]mockExecResult{
			"default-ubuntu-2204": {exitCode: 0},
			"default-centos-8":    {exitCode: 0},
		},
	}

	plan := testPlan([]PlannedInstance{
		mappedInstance("default-ubuntu-2204"),
		mappedInstance("default-centos-8"),
	})

	var mu sync.Mutex
	var progressCalls []int
	onProgress := func(completed, total int, _ PlannedInstance, _ RunInstanceResult) {
		mu.Lock()
		progressCalls = append(progressCalls, completed)
		mu.Unlock()
	}

	s := newTestScheduler(exec, store)
	_, err := s.RunAll(context.Background(), plan, testSchedulerConfig(), testTKConfig(), onProgress)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(progressCalls) != 2 {
		t.Errorf("expected 2 progress callbacks, got %d", len(progressCalls))
	}
}

func TestScheduler_RunAll_ConcurrencyBound(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{
		delay: 50 * time.Millisecond,
		results: map[string]mockExecResult{
			"i1": {exitCode: 0},
			"i2": {exitCode: 0},
			"i3": {exitCode: 0},
			"i4": {exitCode: 0},
			"i5": {exitCode: 0},
		},
	}

	plan := testPlan([]PlannedInstance{
		mappedInstance("i1"),
		mappedInstance("i2"),
		mappedInstance("i3"),
		mappedInstance("i4"),
		mappedInstance("i5"),
	})

	cfg := SchedulerConfig{MaxConcurrency: 2, TargetChefVersion: "18.4.2"}
	s := newTestScheduler(exec, store)
	_, err := s.RunAll(context.Background(), plan, cfg, testTKConfig(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	maxConcurrent := atomic.LoadInt64(&exec.maxSeen)
	if maxConcurrent > 2 {
		t.Errorf("concurrency exceeded limit: max seen %d, limit 2", maxConcurrent)
	}
}

func TestScheduler_RunOne_Success(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{
		results: map[string]mockExecResult{
			"default-ubuntu-2204": {stdout: "converged", exitCode: 0},
		},
	}

	plan := testPlan([]PlannedInstance{
		mappedInstance("default-ubuntu-2204"),
		mappedInstance("default-centos-8"),
	})

	s := newTestScheduler(exec, store)
	result, err := s.RunOne(context.Background(), plan, "default-ubuntu-2204", testSchedulerConfig(), testTKConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Instance.InstanceName != "default-ubuntu-2204" {
		t.Errorf("expected instance name default-ubuntu-2204, got %s", result.Instance.InstanceName)
	}
	if result.Result.Passed == nil || !*result.Result.Passed {
		t.Error("expected result to be passed")
	}
	if result.DBResult.ID == "" {
		t.Error("expected DBResult to have an ID")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.results) != 1 {
		t.Errorf("expected 1 persisted result, got %d", len(store.results))
	}
}

func TestScheduler_RunOne_NotFound(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{}

	plan := testPlan([]PlannedInstance{
		mappedInstance("default-ubuntu-2204"),
	})

	s := newTestScheduler(exec, store)
	_, err := s.RunOne(context.Background(), plan, "nonexistent-instance", testSchedulerConfig(), testTKConfig())
	if err == nil {
		t.Fatal("expected error for not-found instance")
	}
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound, got: %v", err)
	}
}

func TestScheduler_RunOne_NotMapped(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{}

	plan := testPlan([]PlannedInstance{
		{InstanceName: "default-centos-8", SuiteName: "default", PlatformName: "centos-8", Status: InstanceStatusUnmapped, StatusReason: "no mapping for centos-8"},
	})

	s := newTestScheduler(exec, store)
	_, err := s.RunOne(context.Background(), plan, "default-centos-8", testSchedulerConfig(), testTKConfig())
	if err == nil {
		t.Fatal("expected error for unmapped instance")
	}
	if !errors.Is(err, ErrInstanceNotRunnable) {
		t.Errorf("expected ErrInstanceNotRunnable, got: %v", err)
	}
}

func TestScheduler_RunAll_StoreError(t *testing.T) {
	store := &mockResultStore{err: errors.New("database unavailable")}
	exec := &schedulerMockExecutor{
		results: map[string]mockExecResult{
			"default-ubuntu-2204": {exitCode: 0},
			"default-centos-8":    {exitCode: 0},
		},
	}

	plan := testPlan([]PlannedInstance{
		mappedInstance("default-ubuntu-2204"),
		mappedInstance("default-centos-8"),
	})

	s := newTestScheduler(exec, store)
	result, err := s.RunAll(context.Background(), plan, testSchedulerConfig(), testTKConfig(), nil)
	if err != nil {
		t.Fatalf("RunAll should not return error on store failures, got: %v", err)
	}

	if result.Errors != 2 {
		t.Errorf("expected 2 errors (store failures), got %d", result.Errors)
	}
}

func TestScheduler_RunOne_UsesCallerTKConfig(t *testing.T) {
	store := &mockResultStore{}
	exec := &schedulerMockExecutor{
		results: map[string]mockExecResult{
			"default-ubuntu-2204": {exitCode: 0},
		},
	}

	plan := testPlan([]PlannedInstance{mappedInstance("default-ubuntu-2204")})

	s := newTestScheduler(exec, store)

	// Capture what tkConfig the runFn receives.
	var receivedDriver string
	s.runFn = func(_ context.Context, _ RunInstanceParams, tkConfig config.TestKitchenConfig,
		_ KitchenExecutor, _ CredentialResolver) RunInstanceResult {
		receivedDriver = tkConfig.Driver
		passed := true
		return RunInstanceResult{Passed: &passed, DriverUsed: tkConfig.Driver}
	}

	callerCfg := config.TestKitchenConfig{Driver: "proxmox-caller"}
	_, err := s.RunOne(context.Background(), plan, "default-ubuntu-2204", testSchedulerConfig(), callerCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedDriver != "proxmox-caller" {
		t.Errorf("expected runFn to receive driver %q, got %q", "proxmox-caller", receivedDriver)
	}
}
