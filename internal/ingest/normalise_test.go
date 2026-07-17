// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// load reads a committed golden fixture and normalises it. The fixtures are the
// contract: one record per producer shape (see testdata/event-ingest/README.md).
func load(t *testing.T, name string) (*ConvergeRun, error) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "event-ingest", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return Normalise(json.RawMessage(b))
}

func mustLoad(t *testing.T, name string) *ConvergeRun {
	t.Helper()
	run, err := load(t, name)
	if err != nil {
		t.Fatalf("Normalise(%s) unexpected error: %v", name, err)
	}
	if run == nil {
		t.Fatalf("Normalise(%s) returned nil, want a ConvergeRun", name)
	}
	return run
}

// The shapes we persist must all map to the same run identity, node, and org
// regardless of producer — that invariant is what lets a run arriving twice
// (Server proxy AND Automate) dedup on run_id.
func TestNormalise_Shapes(t *testing.T) {
	end := time.Date(2026, 7, 16, 9, 1, 12, 0, time.UTC)

	tests := []struct {
		fixture     string
		wantShape   string
		wantRunID   string
		wantStatus  string
		wantServer  string
		wantSource  string
		wantEndTime time.Time
	}{
		{"run_converge_success.json", "run_converge", "b2222222-2222-4222-8222-222222222222", "success", "chef.example.com", "", end},
		{"run_converge_proxy.json", "run_converge", "b6666666-6666-4666-8666-666666666666", "success", "chef.example.com", "", time.Date(2026, 7, 16, 11, 0, 58, 0, time.UTC)},
		{"datafeed_success.json", "datafeed", "c1111111-1111-4111-8111-111111111111", "success", "", "automate.example.com", end},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			run := mustLoad(t, tc.fixture)
			if run.Shape != tc.wantShape {
				t.Errorf("Shape = %q, want %q", run.Shape, tc.wantShape)
			}
			if run.RunID != tc.wantRunID {
				t.Errorf("RunID = %q, want %q", run.RunID, tc.wantRunID)
			}
			if run.Organisation != "org-a" {
				t.Errorf("Organisation = %q, want org-a", run.Organisation)
			}
			if run.NodeName != "node-a.example.com" {
				t.Errorf("NodeName = %q, want node-a.example.com", run.NodeName)
			}
			if run.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", run.Status, tc.wantStatus)
			}
			if run.ChefVersion != "18.9.4" {
				t.Errorf("ChefVersion = %q, want 18.9.4", run.ChefVersion)
			}
			if !run.EndTime.Equal(tc.wantEndTime) {
				t.Errorf("EndTime = %v, want %v", run.EndTime, tc.wantEndTime)
			}
			if tc.wantServer != "" && run.ChefServerFQDN != tc.wantServer {
				t.Errorf("ChefServerFQDN = %q, want %q", run.ChefServerFQDN, tc.wantServer)
			}
			if tc.wantSource != "" && run.SourceFQDN != tc.wantSource {
				t.Errorf("SourceFQDN = %q, want %q", run.SourceFQDN, tc.wantSource)
			}
			if run.Error != nil {
				t.Errorf("Error = %+v, want nil for a success", run.Error)
			}
			if run.FailedResource != nil {
				t.Errorf("FailedResource = %+v, want nil for a success", run.FailedResource)
			}
		})
	}
}

func TestNormalise_RunListAndCookbooks(t *testing.T) {
	run := mustLoad(t, "run_converge_success.json")
	wantRL := []string{"recipe[base::default]", "recipe[ntp::default]"}
	if len(run.RunList) != len(wantRL) {
		t.Fatalf("RunList = %v, want %v", run.RunList, wantRL)
	}
	for i := range wantRL {
		if run.RunList[i] != wantRL[i] {
			t.Errorf("RunList[%d] = %q, want %q", i, run.RunList[i], wantRL[i])
		}
	}
	if run.Cookbooks["base"] != "1.2.0" || run.Cookbooks["ntp"] != "3.4.0" {
		t.Errorf("Cookbooks = %v, want base=1.2.0 ntp=3.4.0", run.Cookbooks)
	}
	if run.TotalResourceCount != 12 || run.UpdatedResourceCount != 3 {
		t.Errorf("resource counts = %d/%d, want 12/3", run.TotalResourceCount, run.UpdatedResourceCount)
	}
}

// A converge failure must carry the error + bounded backtrace + the failing
// resource — the whole point of the feature (Automate can't isolate these).
func TestNormalise_Failure(t *testing.T) {
	for _, fixture := range []string{"run_converge_failure.json", "datafeed_failure.json"} {
		t.Run(fixture, func(t *testing.T) {
			run := mustLoad(t, fixture)
			if run.Status != "failure" {
				t.Fatalf("Status = %q, want failure", run.Status)
			}
			if run.Error == nil {
				t.Fatal("Error = nil, want populated")
			}
			if run.Error.Class != "RuntimeError" {
				t.Errorf("Error.Class = %q, want RuntimeError", run.Error.Class)
			}
			if run.Error.Message != "boom from failcb" {
				t.Errorf("Error.Message = %q, want 'boom from failcb'", run.Error.Message)
			}
			if len(run.Error.Backtrace) != 4 {
				t.Errorf("Backtrace lines = %d, want 4", len(run.Error.Backtrace))
			}
			if run.FailedResource == nil {
				t.Fatal("FailedResource = nil, want the failing resource")
			}
			fr := run.FailedResource
			if fr.Type != "ruby_block" || fr.Name != "explode" || fr.CookbookName != "failcb" || fr.RecipeName != "default" {
				t.Errorf("FailedResource = %+v, want ruby_block/explode/failcb/default", fr)
			}
		})
	}
}

// Accepted-but-ignored shapes must produce no row (nil, nil): run_start is not a
// converge, and the attributes-only Data Feed record is the known depsolve-abort gap.
func TestNormalise_Ignored(t *testing.T) {
	for _, fixture := range []string{"run_start.json", "datafeed_attributes_only.json"} {
		t.Run(fixture, func(t *testing.T) {
			run, err := load(t, fixture)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if run != nil {
				t.Errorf("Normalise = %+v, want nil (ignored shape)", run)
			}
		})
	}
}
