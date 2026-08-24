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

// runTime must accept both wire encodings: the RFC3339 string a raw
// run_converge sends, and the protobuf Timestamp object {"seconds":N,"nanos":M}
// the Automate Data Feed's client_run sends. A string-only assumption drops
// every Data Feed record.
func TestRunTime_UnmarshalJSON(t *testing.T) {
	want := time.Date(2026, 7, 16, 9, 1, 12, 0, time.UTC)
	cases := []struct {
		name string
		in   string
		want time.Time
	}{
		{"rfc3339 string", `"2026-07-16T09:01:12Z"`, want},
		{"rfc3339 nanos", `"2026-07-16T09:01:12.5Z"`, want.Add(500 * time.Millisecond)},
		{"protobuf seconds", `{"seconds":1784192472}`, want},
		{"protobuf seconds+nanos", `{"seconds":1784192472,"nanos":500000000}`, want.Add(500 * time.Millisecond)},
		{"bare epoch", `1784192472`, want},
		{"null", `null`, time.Time{}},
		{"empty string", `""`, time.Time{}},
		{"zero protobuf", `{"seconds":0}`, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rt runTime
			if err := json.Unmarshal([]byte(tc.in), &rt); err != nil {
				t.Fatalf("unmarshal %s: %v", tc.in, err)
			}
			if !rt.Time.UTC().Equal(tc.want) {
				t.Errorf("got %v, want %v", rt.Time.UTC(), tc.want)
			}
		})
	}
}

// A Data Feed record whose client_run carries protobuf-object timestamps must
// normalise (not be dropped on the end_time guard) — the live-found bug.
func TestNormalise_DataFeedProtobufTimestamps(t *testing.T) {
	rec := []byte(`{"node":{"automate_fqdn":"a"},"client_run":{` +
		`"id":"r1","node_name":"n1","organization":"demo","status":"success",` +
		`"start_time":{"seconds":1784192400},"end_time":{"seconds":1784192472}}}`)
	run, err := Normalise(rec)
	if err != nil {
		t.Fatalf("Normalise returned error (record would be dropped): %v", err)
	}
	if run == nil {
		t.Fatal("Normalise returned nil (record dropped) for a valid Data Feed run")
	}
	if run.Shape != ShapeDataFeed || run.RunID != "r1" || run.NodeName != "n1" {
		t.Errorf("mapping wrong: %+v", run)
	}
	if !run.EndTime.Equal(time.Date(2026, 7, 16, 9, 1, 12, 0, time.UTC)) {
		t.Errorf("EndTime = %v, want 2026-07-16T09:01:12Z (from protobuf seconds)", run.EndTime)
	}
}

// The Automate Data Feed sends client_run.cookbooks as a LIST of names with the
// versions in versioned_cookbooks. A map-only decoder fails the whole record on
// the list, dropping every Data Feed record. Verify it normalises and cookbook
// name->version comes from versioned_cookbooks.
func TestNormalise_DataFeedCookbooksList(t *testing.T) {
	rec := []byte(`{"node":{"automate_fqdn":"a"},"client_run":{"id":"r1","node_name":"n1",` +
		`"organization":"o","status":"success","end_time":{"seconds":1784192472},` +
		`"cookbooks":["cron","chef-client"],` +
		`"versioned_cookbooks":[{"name":"cron","version":"7.0.29"},{"name":"chef-client","version":"18.0.0"}]}}`)
	run, err := Normalise(rec)
	if err != nil {
		t.Fatalf("Normalise errored (record would be dropped): %v", err)
	}
	if run == nil {
		t.Fatal("record dropped (nil) for a valid Data Feed run with list cookbooks")
	}
	if run.Cookbooks["cron"] != "7.0.29" || run.Cookbooks["chef-client"] != "18.0.0" {
		t.Errorf("cookbooks = %v, want cron=7.0.29 chef-client=18.0.0", run.Cookbooks)
	}
}

// Freeform resilience: a record whose non-key fields arrive in shapes we don't
// expect (counts as strings, cookbooks as a number, chef_version as an array)
// must still normalise — those fields degrade to zero, only a missing end_time
// drops a record. Guards against the drop-the-whole-record fragility that hid
// the timestamp and cookbooks bugs.
func TestNormalise_ResilientToWeirdFieldShapes(t *testing.T) {
	rec := []byte(`{"client_run":{"id":"r1","node_name":"n1","organization":"o","status":"success",` +
		`"end_time":{"seconds":1784192472},"total_resource_count":"lots","cookbooks":42,` +
		`"chef_version":["nope"],"error":"not-an-object"}}`)
	run, err := Normalise(rec)
	if err != nil {
		t.Fatalf("Normalise errored on weird field shapes (would drop the record): %v", err)
	}
	if run == nil {
		t.Fatal("record dropped over weird non-key field shapes")
	}
	if run.RunID != "r1" || run.Status != "success" || run.EndTime.IsZero() {
		t.Errorf("core fields lost: %+v", run)
	}
	if run.TotalResourceCount != 0 || len(run.Cookbooks) != 0 || run.Error != nil {
		t.Errorf("weird fields should degrade to zero, got %+v", run)
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
