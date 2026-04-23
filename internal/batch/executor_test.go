// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockRunner struct {
	mu       sync.Mutex
	calls    []RunInstanceRequest
	resultFn func(req RunInstanceRequest) RunInstanceResult
}

func (m *mockRunner) RunInstance(ctx context.Context, req RunInstanceRequest) RunInstanceResult {
	m.mu.Lock()
	m.calls = append(m.calls, req)
	m.mu.Unlock()
	if m.resultFn != nil {
		return m.resultFn(req)
	}
	passed := true
	now := time.Now()
	dur := 1
	return RunInstanceResult{
		ConvergePassed:  &passed,
		TestsPassed:     &passed,
		DurationSeconds: &dur,
		StartedAt:       &now,
		CompletedAt:     &now,
	}
}

type mockResultStore struct {
	mu        sync.Mutex
	results   []UpsertResultParams
	statuses  []struct{ BatchID, Status string }
	upsertErr error
	statusErr error
}

func (m *mockResultStore) UpsertGitKitchenResult(ctx context.Context, p UpsertResultParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results = append(m.results, p)
	return m.upsertErr
}

func (m *mockResultStore) UpdateBatchStatus(ctx context.Context, batchID string, status string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses = append(m.statuses, struct{ BatchID, Status string }{batchID, status})
	return m.statusErr
}

type mockEnumerator struct {
	instances map[string][]InstanceInfo
}

func (m *mockEnumerator) ListInstances(ctx context.Context, repoName string) ([]InstanceInfo, error) {
	return m.instances[repoName], nil
}

type discardLogger struct{}

func (discardLogger) Info(string)  {}
func (discardLogger) Warn(string)  {}
func (discardLogger) Error(string) {}

func TestExecutor_BasicExecution(t *testing.T) {
	repos := &mockRepoLister{repos: []GitRepo{
		{Name: "repo-a", GitRepoURL: "https://git.example.com/repo-a.git", HasTestSuite: true},
		{Name: "repo-b", GitRepoURL: "https://git.example.com/repo-b.git", HasTestSuite: true},
	}}
	enum := &mockEnumerator{instances: map[string][]InstanceInfo{
		"repo-a": {{PlatformName: "platform-a", SuiteName: "suite-a"}, {PlatformName: "platform-b", SuiteName: "suite-b"}},
		"repo-b": {{PlatformName: "platform-a", SuiteName: "suite-a"}, {PlatformName: "platform-b", SuiteName: "suite-b"}},
	}}
	runner := &mockRunner{}
	store := &mockResultStore{}

	resolver := NewResolver(repos)
	exec := NewExecutor(resolver, runner, store, enum, discardLogger{})

	err := exec.Execute(context.Background(), "batch-1", Filters{}, nil, 5, []string{"18.5.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.mu.Lock()
	callCount := len(runner.calls)
	runner.mu.Unlock()
	if callCount != 4 {
		t.Errorf("expected 4 runner calls, got %d", callCount)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.results) != 4 {
		t.Errorf("expected 4 results, got %d", len(store.results))
	}

	if len(store.statuses) == 0 {
		t.Fatal("expected at least one status update")
	}
	last := store.statuses[len(store.statuses)-1]
	if last.Status != "completed" {
		t.Errorf("expected final status 'completed', got %q", last.Status)
	}

	// Verify result params contain correct data.
	foundRepos := map[string]bool{}
	foundPlatforms := map[string]bool{}
	for _, r := range store.results {
		if r.BatchID != "batch-1" {
			t.Errorf("expected BatchID 'batch-1', got %q", r.BatchID)
		}
		foundRepos[r.GitRepoName] = true
		foundPlatforms[r.PlatformName] = true
	}
	if !foundRepos["repo-a"] || !foundRepos["repo-b"] {
		t.Errorf("expected both repo-a and repo-b in results, got %v", foundRepos)
	}
	if !foundPlatforms["platform-a"] || !foundPlatforms["platform-b"] {
		t.Errorf("expected both platforms in results, got %v", foundPlatforms)
	}
}

func TestExecutor_ConcurrencyBounding(t *testing.T) {
	var repos []GitRepo
	instances := map[string][]InstanceInfo{}
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("repo-%d", i)
		repos = append(repos, GitRepo{Name: name, GitRepoURL: fmt.Sprintf("https://git.example.com/%s.git", name), HasTestSuite: true})
		instances[name] = []InstanceInfo{{PlatformName: "plat", SuiteName: "suite"}}
	}

	runner := &mockRunner{
		resultFn: func(req RunInstanceRequest) RunInstanceResult {
			time.Sleep(10 * time.Millisecond)
			passed := true
			now := time.Now()
			dur := 1
			return RunInstanceResult{
				ConvergePassed:  &passed,
				TestsPassed:     &passed,
				DurationSeconds: &dur,
				StartedAt:       &now,
				CompletedAt:     &now,
			}
		},
	}
	store := &mockResultStore{}
	lister := &mockRepoLister{repos: repos}
	enum := &mockEnumerator{instances: instances}

	resolver := NewResolver(lister)
	exec := NewExecutor(resolver, runner, store, enum, discardLogger{})

	err := exec.Execute(context.Background(), "batch-conc", Filters{}, nil, 2, []string{"18.5.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.results) != 10 {
		t.Errorf("expected 10 results, got %d", len(store.results))
	}
	if len(store.statuses) == 0 || store.statuses[len(store.statuses)-1].Status != "completed" {
		t.Error("expected final status 'completed'")
	}
}

func TestExecutor_Cancellation(t *testing.T) {
	var repos []GitRepo
	instances := map[string][]InstanceInfo{}
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("repo-%d", i)
		repos = append(repos, GitRepo{Name: name, GitRepoURL: fmt.Sprintf("https://git.example.com/%s.git", name), HasTestSuite: true})
		instances[name] = []InstanceInfo{{PlatformName: "plat", SuiteName: "suite"}}
	}

	ctx, cancel := context.WithCancel(context.Background())
	var callCount int64
	var mu sync.Mutex

	runner := &mockRunner{
		resultFn: func(req RunInstanceRequest) RunInstanceResult {
			mu.Lock()
			callCount++
			n := callCount
			mu.Unlock()
			if n >= 3 {
				cancel()
			}
			passed := true
			now := time.Now()
			dur := 1
			return RunInstanceResult{
				ConvergePassed:  &passed,
				TestsPassed:     &passed,
				DurationSeconds: &dur,
				StartedAt:       &now,
				CompletedAt:     &now,
			}
		},
	}
	store := &mockResultStore{}
	lister := &mockRepoLister{repos: repos}
	enum := &mockEnumerator{instances: instances}

	resolver := NewResolver(lister)
	exec := NewExecutor(resolver, runner, store, enum, discardLogger{})

	_ = exec.Execute(ctx, "batch-cancel", Filters{}, nil, 1, []string{"18.5.0"})

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.statuses) == 0 {
		t.Fatal("expected at least one status update")
	}
	last := store.statuses[len(store.statuses)-1]
	if last.Status != "cancelled" {
		t.Errorf("expected final status 'cancelled', got %q", last.Status)
	}

	runner.mu.Lock()
	rc := len(runner.calls)
	runner.mu.Unlock()
	if rc > 3 {
		t.Errorf("expected at most 3 runner calls, got %d", rc)
	}
}

func TestExecutor_EmptyBatch(t *testing.T) {
	repos := &mockRepoLister{repos: []GitRepo{
		{Name: "repo-a", GitRepoURL: "https://git.example.com/repo-a.git", HasTestSuite: true},
	}}
	runner := &mockRunner{}
	store := &mockResultStore{}
	enum := &mockEnumerator{instances: map[string][]InstanceInfo{}}

	resolver := NewResolver(repos)
	exec := NewExecutor(resolver, runner, store, enum, discardLogger{})

	err := exec.Execute(context.Background(), "batch-empty", Filters{CookbookNames: []string{"nonexistent"}}, nil, 5, []string{"18.5.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.mu.Lock()
	if len(runner.calls) != 0 {
		t.Errorf("expected 0 runner calls, got %d", len(runner.calls))
	}
	runner.mu.Unlock()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.results) != 0 {
		t.Errorf("expected 0 results, got %d", len(store.results))
	}
	if len(store.statuses) == 0 || store.statuses[len(store.statuses)-1].Status != "completed" {
		t.Error("expected final status 'completed'")
	}
}

func TestExecutor_DefaultInstanceWhenEnumeratorEmpty(t *testing.T) {
	repos := &mockRepoLister{repos: []GitRepo{
		{Name: "repo-x", GitRepoURL: "https://git.example.com/repo-x.git", HasTestSuite: true},
	}}
	runner := &mockRunner{}
	store := &mockResultStore{}
	enum := &mockEnumerator{instances: map[string][]InstanceInfo{
		"repo-x": {},
	}}

	resolver := NewResolver(repos)
	exec := NewExecutor(resolver, runner, store, enum, discardLogger{})

	err := exec.Execute(context.Background(), "batch-def", Filters{}, nil, 5, []string{"18.5.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(runner.calls))
	}
	if runner.calls[0].PlatformName != "default" {
		t.Errorf("expected PlatformName 'default', got %q", runner.calls[0].PlatformName)
	}
	if runner.calls[0].SuiteName != "default" {
		t.Errorf("expected SuiteName 'default', got %q", runner.calls[0].SuiteName)
	}
}

func TestExecutor_MultipleTargetVersions(t *testing.T) {
	repos := &mockRepoLister{repos: []GitRepo{
		{Name: "repo-v", GitRepoURL: "https://git.example.com/repo-v.git", HasTestSuite: true},
	}}
	runner := &mockRunner{}
	store := &mockResultStore{}
	enum := &mockEnumerator{instances: map[string][]InstanceInfo{
		"repo-v": {{PlatformName: "plat", SuiteName: "suite"}},
	}}

	resolver := NewResolver(repos)
	exec := NewExecutor(resolver, runner, store, enum, discardLogger{})

	err := exec.Execute(context.Background(), "batch-ver", Filters{}, nil, 5, []string{"18.5.0", "17.10.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 runner calls, got %d", len(runner.calls))
	}

	versions := map[string]bool{}
	for _, c := range runner.calls {
		versions[c.TargetChefVersion] = true
	}
	if !versions["18.5.0"] || !versions["17.10.0"] {
		t.Errorf("expected both target versions, got %v", versions)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.results) != 2 {
		t.Errorf("expected 2 results, got %d", len(store.results))
	}
}

func TestExecutor_UpsertErrorDoesNotAbortBatch(t *testing.T) {
	repos := &mockRepoLister{repos: []GitRepo{
		{Name: "repo-e1", GitRepoURL: "https://git.example.com/repo-e1.git", HasTestSuite: true},
		{Name: "repo-e2", GitRepoURL: "https://git.example.com/repo-e2.git", HasTestSuite: true},
	}}
	runner := &mockRunner{}
	store := &mockResultStore{upsertErr: fmt.Errorf("db error")}
	enum := &mockEnumerator{instances: map[string][]InstanceInfo{
		"repo-e1": {{PlatformName: "plat", SuiteName: "suite"}},
		"repo-e2": {{PlatformName: "plat", SuiteName: "suite"}},
	}}

	resolver := NewResolver(repos)
	exec := NewExecutor(resolver, runner, store, enum, discardLogger{})

	err := exec.Execute(context.Background(), "batch-err", Filters{}, nil, 5, []string{"18.5.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runner.mu.Lock()
	if len(runner.calls) != 2 {
		t.Errorf("expected 2 runner calls despite upsert errors, got %d", len(runner.calls))
	}
	runner.mu.Unlock()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.statuses) == 0 || store.statuses[len(store.statuses)-1].Status != "completed" {
		t.Error("expected final status 'completed' despite upsert errors")
	}
}
