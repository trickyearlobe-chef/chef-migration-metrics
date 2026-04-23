// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Mock implementations for runner tests
// ---------------------------------------------------------------------------

type mockDataStore struct {
	snapshot    datastore.NodeSnapshot
	snapshotErr error

	upsertedRun datastore.NodeKitchenRun
	upsertErr   error

	updatedRun datastore.NodeKitchenRun
	updateErr  error

	getSnapshotCalled bool
	upsertCalled      bool
	updateCalled      bool
}

func (m *mockDataStore) GetNodeSnapshotByName(_ context.Context, _, _ string) (datastore.NodeSnapshot, error) {
	m.getSnapshotCalled = true
	return m.snapshot, m.snapshotErr
}

func (m *mockDataStore) UpsertNodeKitchenRun(_ context.Context, _ datastore.UpsertNodeKitchenRunParams) (datastore.NodeKitchenRun, error) {
	m.upsertCalled = true
	return m.upsertedRun, m.upsertErr
}

func (m *mockDataStore) UpdateNodeKitchenRunResult(_ context.Context, _ string, _ datastore.UpdateNodeKitchenRunResultParams) (datastore.NodeKitchenRun, error) {
	m.updateCalled = true
	return m.updatedRun, m.updateErr
}

type mockExecutorResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

type mockExecutor struct {
	responses map[string]mockExecutorResponse
	calls     []string
}

func (m *mockExecutor) Run(_ context.Context, _ string, _ []string, args ...string) (string, string, int, error) {
	phase := ""
	if len(args) > 0 {
		phase = args[0]
	}
	m.calls = append(m.calls, phase)
	resp, ok := m.responses[phase]
	if !ok {
		return "", "", 1, fmt.Errorf("unexpected phase %q", phase)
	}
	return resp.stdout, resp.stderr, resp.exitCode, resp.err
}

type mockLogger struct {
	messages []string
}

func (m *mockLogger) Info(msg string)  { m.messages = append(m.messages, "INFO: "+msg) }
func (m *mockLogger) Warn(msg string)  { m.messages = append(m.messages, "WARN: "+msg) }
func (m *mockLogger) Error(msg string) { m.messages = append(m.messages, "ERROR: "+msg) }

type mockDownloader struct{}

func (m *mockDownloader) DownloadCookbook(_ context.Context, _, _, _ string) error {
	return nil
}

type mockGitLocator struct{}

func (m *mockGitLocator) LocateCookbook(_ string) string { return "" }

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func testNodeSnapshot() datastore.NodeSnapshot {
	return datastore.NodeSnapshot{
		OrganisationName: "test-org",
		NodeName:         "test-node",
		Platform:         "ubuntu",
		PlatformVersion:  "22.04",
		RunList:          json.RawMessage(`["recipe[testcb]"]`),
		Cookbooks:        json.RawMessage(`{"testcb":{"version":"1.0.0"}}`),
	}
}

func allPhasesPass() map[string]mockExecutorResponse {
	return map[string]mockExecutorResponse{
		"converge": {stdout: "converge output", exitCode: 0},
		"verify":   {stdout: "verify output", exitCode: 0},
		"destroy":  {stdout: "destroy output", exitCode: 0},
	}
}

func newTestDeps(db *mockDataStore, exec *mockExecutor, logger *mockLogger) RunnerDeps {
	return RunnerDeps{
		DB:          db,
		RoleFetcher: &mockRoleFetcher{},
		DepResolver: &mockDepResolver{},
		Downloader:  &mockDownloader{},
		GitLocator:  &mockGitLocator{},
		Executor:    exec,
		Logger:      logger,
		Concurrency: 1,
	}
}

func validRequest() RunRequest {
	return RunRequest{
		NodeName:          "test-node",
		OrganisationName:  "test-org",
		TargetChefVersion: "18.4.2",
		CookbookSource:    "server",
	}
}

// ---------------------------------------------------------------------------
// validateRequest
// ---------------------------------------------------------------------------

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     RunRequest
		wantErr string
	}{
		{
			name:    "missing node_name",
			req:     RunRequest{OrganisationName: "org", TargetChefVersion: "18.0.0", CookbookSource: "server"},
			wantErr: "node_name",
		},
		{
			name:    "missing organisation_name",
			req:     RunRequest{NodeName: "n", TargetChefVersion: "18.0.0", CookbookSource: "server"},
			wantErr: "organisation_name",
		},
		{
			name:    "missing target_chef_version",
			req:     RunRequest{NodeName: "n", OrganisationName: "org", CookbookSource: "server"},
			wantErr: "target_chef_version",
		},
		{
			name:    "invalid cookbook_source",
			req:     RunRequest{NodeName: "n", OrganisationName: "org", TargetChefVersion: "18.0.0", CookbookSource: "bad"},
			wantErr: "invalid cookbook_source",
		},
		{
			name: "valid server source",
			req:  RunRequest{NodeName: "n", OrganisationName: "org", TargetChefVersion: "18.0.0", CookbookSource: "server"},
		},
		{
			name: "valid git source",
			req:  RunRequest{NodeName: "n", OrganisationName: "org", TargetChefVersion: "18.0.0", CookbookSource: "git"},
		},
		{
			name: "valid hybrid source",
			req:  RunRequest{NodeName: "n", OrganisationName: "org", TargetChefVersion: "18.0.0", CookbookSource: "hybrid"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequest(tt.req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// collectRoleNames
// ---------------------------------------------------------------------------

func TestCollectRoleNames(t *testing.T) {
	tests := []struct {
		name    string
		entries []RunListEntry
		want    []string
	}{
		{
			name:    "no entries",
			entries: nil,
			want:    nil,
		},
		{
			name: "no roles",
			entries: []RunListEntry{
				{Type: "recipe", Name: "nginx", RecipeName: "default"},
				{Type: "recipe", Name: "apt", RecipeName: "default"},
			},
			want: nil,
		},
		{
			name: "roles present in order",
			entries: []RunListEntry{
				{Type: "role", Name: "webserver"},
				{Type: "recipe", Name: "nginx", RecipeName: "default"},
				{Type: "role", Name: "base"},
			},
			want: []string{"webserver", "base"},
		},
		{
			name: "duplicate roles deduplicated",
			entries: []RunListEntry{
				{Type: "role", Name: "webserver"},
				{Type: "recipe", Name: "nginx", RecipeName: "default"},
				{Type: "role", Name: "webserver"},
				{Type: "role", Name: "base"},
			},
			want: []string{"webserver", "base"},
		},
		{
			name: "mixed roles and recipes extracts only roles",
			entries: []RunListEntry{
				{Type: "recipe", Name: "apt", RecipeName: "default"},
				{Type: "role", Name: "monitoring"},
				{Type: "recipe", Name: "nginx", RecipeName: "ssl"},
				{Type: "role", Name: "base"},
				{Type: "recipe", Name: "logrotate", RecipeName: "default"},
			},
			want: []string{"monitoring", "base"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectRoleNames(tt.entries)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d names, got %d: %v", len(tt.want), len(got), got)
			}
			for i, name := range got {
				if name != tt.want[i] {
					t.Errorf("index %d: expected %q, got %q", i, tt.want[i], name)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatEntries
// ---------------------------------------------------------------------------

func TestFormatEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []RunListEntry
		want    []string
	}{
		{
			name:    "empty entries",
			entries: nil,
			want:    []string{},
		},
		{
			name: "multiple entries formatted correctly",
			entries: []RunListEntry{
				{Type: "recipe", Name: "nginx", RecipeName: "default"},
				{Type: "recipe", Name: "app", RecipeName: "ssl"},
				{Type: "role", Name: "base"},
			},
			want: []string{
				"recipe[nginx::default]",
				"recipe[app::ssl]",
				"role[base]",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatEntries(tt.entries)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d entries, got %d: %v", len(tt.want), len(got), got)
			}
			for i, s := range got {
				if s != tt.want[i] {
					t.Errorf("index %d: expected %q, got %q", i, tt.want[i], s)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// credEnvSlice
// ---------------------------------------------------------------------------

func TestCredEnvSlice(t *testing.T) {
	t.Run("nil map returns nil", func(t *testing.T) {
		got := credEnvSlice(nil)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		got := credEnvSlice(map[string][]byte{})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("single entry", func(t *testing.T) {
		got := credEnvSlice(map[string][]byte{"MY_KEY": []byte("my_val")})
		if len(got) != 1 || got[0] != "MY_KEY=my_val" {
			t.Errorf("expected [MY_KEY=my_val], got %v", got)
		}
	})

	t.Run("multiple entries as KEY=VALUE strings", func(t *testing.T) {
		m := map[string][]byte{
			"ALPHA": []byte("a"),
			"BETA":  []byte("b"),
		}
		got := credEnvSlice(m)
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(got))
		}
		sort.Strings(got)
		if got[0] != "ALPHA=a" || got[1] != "BETA=b" {
			t.Errorf("got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Runner.Run — integration-style tests with mocked dependencies
// ---------------------------------------------------------------------------

func TestRunnerRun_HappyPath(t *testing.T) {
	db := &mockDataStore{
		snapshot:    testNodeSnapshot(),
		upsertedRun: datastore.NodeKitchenRun{ID: "run-001"},
	}
	exec := &mockExecutor{responses: allPhasesPass()}
	logger := &mockLogger{}
	deps := newTestDeps(db, exec, logger)
	runner := NewRunner(deps)

	result := runner.Run(context.Background(), validRequest())

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.ID != "run-001" {
		t.Errorf("expected ID %q, got %q", "run-001", result.ID)
	}
	if result.ConvergePassed == nil || !*result.ConvergePassed {
		t.Error("expected ConvergePassed=true")
	}
	if result.VerifyPassed == nil || !*result.VerifyPassed {
		t.Error("expected VerifyPassed=true")
	}
	if result.DurationSeconds == nil {
		t.Error("expected DurationSeconds to be set")
	}
	if result.ConvergeOutput != "converge output" {
		t.Errorf("expected converge output, got %q", result.ConvergeOutput)
	}
	if result.VerifyOutput != "verify output" {
		t.Errorf("expected verify output, got %q", result.VerifyOutput)
	}
	if result.DestroyOutput != "destroy output" {
		t.Errorf("expected destroy output, got %q", result.DestroyOutput)
	}
	if !db.upsertCalled {
		t.Error("expected UpsertNodeKitchenRun to be called")
	}
	if !db.updateCalled {
		t.Error("expected UpdateNodeKitchenRunResult to be called")
	}
	if len(exec.calls) != 3 {
		t.Fatalf("expected 3 executor calls, got %d: %v", len(exec.calls), exec.calls)
	}
	if exec.calls[0] != "converge" || exec.calls[1] != "verify" || exec.calls[2] != "destroy" {
		t.Errorf("unexpected call order: %v", exec.calls)
	}
}

func TestRunnerRun_ConvergeFails(t *testing.T) {
	db := &mockDataStore{
		snapshot:    testNodeSnapshot(),
		upsertedRun: datastore.NodeKitchenRun{ID: "run-002"},
	}
	exec := &mockExecutor{
		responses: map[string]mockExecutorResponse{
			"converge": {stdout: "converge failed", exitCode: 1},
			"destroy":  {stdout: "destroy output", exitCode: 0},
		},
	}
	logger := &mockLogger{}
	deps := newTestDeps(db, exec, logger)
	runner := NewRunner(deps)

	result := runner.Run(context.Background(), validRequest())

	if result.ConvergePassed == nil || *result.ConvergePassed {
		t.Error("expected ConvergePassed=false")
	}
	if result.VerifyPassed != nil {
		t.Errorf("expected VerifyPassed=nil (verify should not run), got %v", *result.VerifyPassed)
	}
	// Verify must NOT be called; destroy must still run.
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 executor calls, got %d: %v", len(exec.calls), exec.calls)
	}
	if exec.calls[0] != "converge" || exec.calls[1] != "destroy" {
		t.Errorf("unexpected call order: %v", exec.calls)
	}
	if result.DestroyOutput != "destroy output" {
		t.Errorf("expected destroy output, got %q", result.DestroyOutput)
	}
}

func TestRunnerRun_NodeNotFound(t *testing.T) {
	db := &mockDataStore{
		snapshotErr: fmt.Errorf("node not found in database"),
	}
	exec := &mockExecutor{}
	logger := &mockLogger{}
	deps := newTestDeps(db, exec, logger)
	runner := NewRunner(deps)

	req := validRequest()
	req.NodeName = "missing-node"
	result := runner.Run(context.Background(), req)

	if result.Error == nil {
		t.Fatal("expected error for missing node")
	}
	if !strings.Contains(result.ErrorMessage, "node snapshot") {
		t.Errorf("expected error about node snapshot, got %q", result.ErrorMessage)
	}
	if db.upsertCalled {
		t.Error("UpsertNodeKitchenRun should not be called when node lookup fails")
	}
	if len(exec.calls) != 0 {
		t.Errorf("executor should not be called, got %v", exec.calls)
	}
}

func TestRunnerRun_InvalidRequest(t *testing.T) {
	db := &mockDataStore{}
	exec := &mockExecutor{}
	logger := &mockLogger{}
	deps := newTestDeps(db, exec, logger)
	runner := NewRunner(deps)

	result := runner.Run(context.Background(), RunRequest{
		// NodeName intentionally empty.
		OrganisationName:  "test-org",
		TargetChefVersion: "18.4.2",
		CookbookSource:    "server",
	})

	if result.Error == nil {
		t.Fatal("expected error for invalid request")
	}
	if !strings.Contains(result.ErrorMessage, "node_name") {
		t.Errorf("expected error about node_name, got %q", result.ErrorMessage)
	}
	if db.getSnapshotCalled {
		t.Error("GetNodeSnapshotByName should not be called for invalid request")
	}
	if db.upsertCalled {
		t.Error("UpsertNodeKitchenRun should not be called for invalid request")
	}
	if len(exec.calls) != 0 {
		t.Errorf("executor should not be called, got %v", exec.calls)
	}
}

func TestRunnerRun_CredentialResolverNil(t *testing.T) {
	db := &mockDataStore{
		snapshot:    testNodeSnapshot(),
		upsertedRun: datastore.NodeKitchenRun{ID: "run-003"},
	}
	exec := &mockExecutor{responses: allPhasesPass()}
	logger := &mockLogger{}
	deps := newTestDeps(db, exec, logger)
	deps.CredResolver = nil // explicitly nil — must not panic
	runner := NewRunner(deps)

	result := runner.Run(context.Background(), validRequest())

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.ConvergePassed == nil || !*result.ConvergePassed {
		t.Error("expected ConvergePassed=true")
	}
	if result.VerifyPassed == nil || !*result.VerifyPassed {
		t.Error("expected VerifyPassed=true")
	}
	if len(exec.calls) != 3 {
		t.Fatalf("expected 3 executor calls, got %d: %v", len(exec.calls), exec.calls)
	}
}
