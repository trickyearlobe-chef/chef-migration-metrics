// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Mock store
// ---------------------------------------------------------------------------

type mockAnalyserStore struct {
	mu       sync.Mutex
	upserted []datastore.UpsertKitchenAnalysisResultParams
	rebuilt  int
	err      error
}

func (m *mockAnalyserStore) UpsertKitchenAnalysisResult(_ context.Context, p datastore.UpsertKitchenAnalysisResultParams) (*datastore.KitchenAnalysisResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	m.upserted = append(m.upserted, p)
	return &datastore.KitchenAnalysisResult{GitRepoName: p.GitRepoName, GitRepoURL: p.GitRepoURL}, nil
}

func (m *mockAnalyserStore) RebuildDiscoveredPlatforms(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rebuilt++
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const testKitchenYML = `driver:
  name: vagrant
provisioner:
  name: chef_zero
  require_chef_omnibus: false
platforms:
  - name: centos-7
suites:
  - name: default
    run_list:
      - recipe[test::default]
`

func writeKitchenYML(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".kitchen.yml"), []byte(testKitchenYML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dirFnFor(dirs map[string]string) func(datastore.GitRepo) string {
	return func(r datastore.GitRepo) string {
		return dirs[r.Name]
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestKitchenAnalyser_AnalyseRepo(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYML(t, dir)

	store := &mockAnalyserStore{}
	a := NewKitchenAnalyser(store, testLogger(), 1)

	repo := datastore.GitRepo{
		Name:          "my-cookbook",
		GitRepoURL:    "https://example.com/my-cookbook.git",
		HeadCommitSHA: "abc123",
	}

	result, err := a.AnalyseRepo(context.Background(), repo, func(_ datastore.GitRepo) string { return dir })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.GitRepoName != "my-cookbook" {
		t.Errorf("GitRepoName = %q, want %q", result.GitRepoName, "my-cookbook")
	}

	if len(store.upserted) != 1 {
		t.Fatalf("upserted count = %d, want 1", len(store.upserted))
	}
	p := store.upserted[0]
	if p.GitRepoName != "my-cookbook" {
		t.Errorf("param GitRepoName = %q, want %q", p.GitRepoName, "my-cookbook")
	}
	if p.GitRepoURL != "https://example.com/my-cookbook.git" {
		t.Errorf("param GitRepoURL = %q, want %q", p.GitRepoURL, "https://example.com/my-cookbook.git")
	}
	if p.HeadCommitSHA != "abc123" {
		t.Errorf("param HeadCommitSHA = %q, want %q", p.HeadCommitSHA, "abc123")
	}
	if p.DriverName != "vagrant" {
		t.Errorf("param DriverName = %q, want %q", p.DriverName, "vagrant")
	}
	if p.ProvisionerName != "chef_zero" {
		t.Errorf("param ProvisionerName = %q, want %q", p.ProvisionerName, "chef_zero")
	}
	if p.RequireChefOmnibus == nil {
		t.Fatal("RequireChefOmnibus is nil, want *false")
	} else if *p.RequireChefOmnibus {
		t.Error("RequireChefOmnibus = true, want false")
	}
	if p.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", p.ErrorMessage)
	}
}

func TestKitchenAnalyser_AnalyseRepo_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// No kitchen files written.

	store := &mockAnalyserStore{}
	a := NewKitchenAnalyser(store, testLogger(), 1)

	repo := datastore.GitRepo{
		Name:          "empty-cookbook",
		GitRepoURL:    "https://example.com/empty.git",
		HeadCommitSHA: "def456",
	}

	result, err := a.AnalyseRepo(context.Background(), repo, func(_ datastore.GitRepo) string { return dir })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The analyser should still upsert — with an error message about no
	// kitchen config found — because AnalyseKitchenDir returns an entry
	// with an ErrorMessage rather than failing.
	if result == nil {
		t.Fatal("expected non-nil result (error entry should be upserted)")
	}
	if len(store.upserted) != 1 {
		t.Fatalf("upserted count = %d, want 1", len(store.upserted))
	}
	if store.upserted[0].ErrorMessage == "" {
		t.Error("expected non-empty ErrorMessage for dir with no kitchen files")
	}
}

func TestKitchenAnalyser_AnalyseRepo_NotCloned(t *testing.T) {
	store := &mockAnalyserStore{}
	a := NewKitchenAnalyser(store, testLogger(), 1)

	repo := datastore.GitRepo{
		Name:       "not-cloned",
		GitRepoURL: "https://example.com/not-cloned.git",
	}

	result, err := a.AnalyseRepo(context.Background(), repo, func(_ datastore.GitRepo) string { return "" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for not-cloned repo, got %+v", result)
	}
	if len(store.upserted) != 0 {
		t.Errorf("upserted count = %d, want 0", len(store.upserted))
	}
}

func TestKitchenAnalyser_AnalyseAll(t *testing.T) {
	dir1 := t.TempDir()
	writeKitchenYML(t, dir1)

	dir2 := t.TempDir()
	writeKitchenYML(t, dir2)

	dir3 := t.TempDir()
	// No kitchen files — will still be analysed (upserted with error message).

	repos := []datastore.GitRepo{
		{Name: "cb1", GitRepoURL: "https://example.com/cb1.git", HeadCommitSHA: "aaa"},
		{Name: "cb2", GitRepoURL: "https://example.com/cb2.git", HeadCommitSHA: "bbb"},
		{Name: "cb3", GitRepoURL: "https://example.com/cb3.git", HeadCommitSHA: "ccc"},
		{Name: "not-cloned", GitRepoURL: "https://example.com/nc.git"},
	}

	dirs := map[string]string{
		"cb1":        dir1,
		"cb2":        dir2,
		"cb3":        dir3,
		"not-cloned": "", // not cloned
	}

	store := &mockAnalyserStore{}
	a := NewKitchenAnalyser(store, testLogger(), 2)

	batch := a.AnalyseAll(context.Background(), repos, dirFnFor(dirs))

	if batch.Total != 4 {
		t.Errorf("Total = %d, want 4", batch.Total)
	}
	if batch.Analysed != 3 {
		t.Errorf("Analysed = %d, want 3", batch.Analysed)
	}
	if batch.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", batch.Skipped)
	}
	if batch.Errors != 0 {
		t.Errorf("Errors = %d, want 0", batch.Errors)
	}
	if batch.Duration <= 0 {
		t.Error("Duration should be positive")
	}

	store.mu.Lock()
	rebuilt := store.rebuilt
	upsertCount := len(store.upserted)
	store.mu.Unlock()

	if rebuilt != 1 {
		t.Errorf("RebuildDiscoveredPlatforms called %d times, want 1", rebuilt)
	}
	if upsertCount != 3 {
		t.Errorf("upserted count = %d, want 3", upsertCount)
	}
}

func TestKitchenAnalyser_AnalyseAll_DBError(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYML(t, dir)

	repos := []datastore.GitRepo{
		{Name: "fail-cb", GitRepoURL: "https://example.com/fail.git", HeadCommitSHA: "fff"},
	}

	store := &mockAnalyserStore{err: errors.New("db connection lost")}
	a := NewKitchenAnalyser(store, testLogger(), 1)

	batch := a.AnalyseAll(context.Background(), repos, func(_ datastore.GitRepo) string { return dir })

	if batch.Errors != 1 {
		t.Errorf("Errors = %d, want 1", batch.Errors)
	}
	if batch.Analysed != 0 {
		t.Errorf("Analysed = %d, want 0", batch.Analysed)
	}
}

func TestKitchenAnalyser_Concurrency(t *testing.T) {
	for _, conc := range []int{1, 4} {
		t.Run("concurrency="+string(rune('0'+conc)), func(t *testing.T) {
			dirs := make(map[string]string)
			var repos []datastore.GitRepo
			for i := 0; i < 8; i++ {
				name := "cb-" + string(rune('a'+i))
				d := t.TempDir()
				writeKitchenYML(t, d)
				dirs[name] = d
				repos = append(repos, datastore.GitRepo{
					Name:          name,
					GitRepoURL:    "https://example.com/" + name + ".git",
					HeadCommitSHA: "sha-" + name,
				})
			}

			store := &mockAnalyserStore{}
			a := NewKitchenAnalyser(store, testLogger(), conc)

			batch := a.AnalyseAll(context.Background(), repos, dirFnFor(dirs))

			if batch.Analysed != 8 {
				t.Errorf("concurrency=%d: Analysed = %d, want 8", conc, batch.Analysed)
			}
			if batch.Errors != 0 {
				t.Errorf("concurrency=%d: Errors = %d, want 0", conc, batch.Errors)
			}

			store.mu.Lock()
			upsertCount := len(store.upserted)
			store.mu.Unlock()

			if upsertCount != 8 {
				t.Errorf("concurrency=%d: upserted = %d, want 8", conc, upsertCount)
			}
		})
	}
}

func TestNewKitchenAnalyser_DefaultConcurrency(t *testing.T) {
	a := NewKitchenAnalyser(&mockAnalyserStore{}, testLogger(), 0)
	if a.concurrency != 4 {
		t.Errorf("concurrency = %d, want 4 (default)", a.concurrency)
	}

	a = NewKitchenAnalyser(&mockAnalyserStore{}, testLogger(), -1)
	if a.concurrency != 4 {
		t.Errorf("concurrency = %d, want 4 (default)", a.concurrency)
	}
}
