// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Fake DataStore for testing
// ---------------------------------------------------------------------------

type fakeStore struct {
	orgs         []datastore.Organisation
	nodesByOrg   map[string][]datastore.NodeSnapshot
	readiness    map[string][]datastore.NodeReadiness
	cookbooks    map[string][]datastore.ServerCookbook
	complexities map[string][]datastore.ServerCookbookComplexity
	countReady   map[string][3]int // orgID+version -> [total, ready, blocked]
}

func (f *fakeStore) ListOrganisations(_ context.Context) ([]datastore.Organisation, error) {
	return f.orgs, nil
}

func (f *fakeStore) ListNodeSnapshotsByOrganisation(_ context.Context, orgID string) ([]datastore.NodeSnapshot, error) {
	return f.nodesByOrg[orgID], nil
}

func (f *fakeStore) ListNodeReadinessForSnapshot(_ context.Context, snapID string) ([]datastore.NodeReadiness, error) {
	return f.readiness[snapID], nil
}

func (f *fakeStore) ListServerCookbooksByOrganisation(_ context.Context, orgID string) ([]datastore.ServerCookbook, error) {
	return f.cookbooks[orgID], nil
}

func (f *fakeStore) ListGitRepos(_ context.Context) ([]datastore.GitRepo, error) {
	return nil, nil
}

func (f *fakeStore) ListServerCookbookComplexitiesByOrganisation(_ context.Context, orgID string) ([]datastore.ServerCookbookComplexity, error) {
	return f.complexities[orgID], nil
}

func (f *fakeStore) CountNodeReadiness(_ context.Context, orgID, targetVersion string) (int, int, int, error) {
	key := orgID + "+" + targetVersion
	if v, ok := f.countReady[key]; ok {
		return v[0], v[1], v[2], nil
	}
	return 0, 0, 0, nil
}

// Compile-time check.
var _ DataStore = (*fakeStore)(nil)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

func testStore() *fakeStore {
	org := datastore.Organisation{ID: "org-1", Name: "prod-org"}

	nodes := []datastore.NodeSnapshot{
		{
			ID:              "snap-1",
			OrganisationID:  "org-1",
			NodeName:        "web01",
			ChefEnvironment: "production",
			ChefVersion:     "16.0.0",
			Platform:        "ubuntu",
			PlatformVersion: "20.04",
			PolicyName:      "web",
			PolicyGroup:     "prod",
			Roles:           json.RawMessage(`["base","webserver"]`),
			CollectedAt:     time.Now(),
		},
		{
			ID:              "snap-2",
			OrganisationID:  "org-1",
			NodeName:        "db01",
			ChefEnvironment: "production",
			ChefVersion:     "15.0.0",
			Platform:        "centos",
			PlatformVersion: "7",
			PolicyName:      "",
			PolicyGroup:     "",
			Roles:           json.RawMessage(`["base","database"]`),
			CollectedAt:     time.Now(),
		},
		{
			ID:              "snap-3",
			OrganisationID:  "org-1",
			NodeName:        "staging01",
			ChefEnvironment: "staging",
			ChefVersion:     "17.0.0",
			Platform:        "ubuntu",
			PlatformVersion: "22.04",
			Roles:           json.RawMessage(`["base"]`),
			CollectedAt:     time.Now(),
		},
	}

	readiness := map[string][]datastore.NodeReadiness{
		"snap-1": {{
			ID:                     "nr-1",
			NodeSnapshotID:         "snap-1",
			OrganisationID:         "org-1",
			NodeName:               "web01",
			TargetChefVersion:      "18.0.0",
			IsReady:                true,
			AllCookbooksCompatible: true,
		}},
		"snap-2": {{
			ID:                     "nr-2",
			NodeSnapshotID:         "snap-2",
			OrganisationID:         "org-1",
			NodeName:               "db01",
			TargetChefVersion:      "18.0.0",
			IsReady:                false,
			AllCookbooksCompatible: false,
			BlockingCookbooks:      json.RawMessage(`["legacy-db"]`),
		}},
		"snap-3": {{
			ID:                     "nr-3",
			NodeSnapshotID:         "snap-3",
			OrganisationID:         "org-1",
			NodeName:               "staging01",
			TargetChefVersion:      "18.0.0",
			IsReady:                true,
			AllCookbooksCompatible: true,
		}},
	}

	cookbooks := map[string][]datastore.ServerCookbook{
		"org-1": {
			{ID: "cb-1", OrganisationID: "org-1", Name: "legacy-db", Version: "1.0.0"},
			{ID: "cb-2", OrganisationID: "org-1", Name: "webserver", Version: "2.0.0"},
		},
	}

	complexities := map[string][]datastore.ServerCookbookComplexity{
		"org-1": {
			{
				ID:                   "cc-1",
				ServerCookbookID:     "cb-1",
				TargetChefVersion:    "18.0.0",
				ComplexityScore:      75,
				ComplexityLabel:      "high",
				AffectedNodeCount:    5,
				AffectedRoleCount:    2,
				AutoCorrectableCount: 3,
				ManualFixCount:       2,
				DeprecationCount:     4,
				ErrorCount:           1,
			},
			{
				ID:                   "cc-2",
				ServerCookbookID:     "cb-2",
				TargetChefVersion:    "18.0.0",
				ComplexityScore:      10,
				ComplexityLabel:      "low",
				AffectedNodeCount:    10,
				AffectedRoleCount:    3,
				AutoCorrectableCount: 5,
				ManualFixCount:       0,
				DeprecationCount:     1,
				ErrorCount:           0,
			},
		},
	}

	return &fakeStore{
		orgs:         []datastore.Organisation{org},
		nodesByOrg:   map[string][]datastore.NodeSnapshot{"org-1": nodes},
		readiness:    readiness,
		cookbooks:    cookbooks,
		complexities: complexities,
		countReady: map[string][3]int{
			"org-1+18.0.0": {3, 2, 1},
		},
	}
}

// ---------------------------------------------------------------------------
// Ready node export tests
// ---------------------------------------------------------------------------

func TestGenerateReadyNodeExport_CSV(t *testing.T) {
	store := testStore()
	ctx := context.Background()

	result, err := GenerateReadyNodeExport(ctx, store, ReadyNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "csv",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", result.RowCount)
	}
	if result.ContentType != "text/csv; charset=utf-8" {
		t.Errorf("ContentType = %q, want text/csv", result.ContentType)
	}
	if !strings.HasSuffix(result.Filename, ".csv") {
		t.Errorf("Filename = %q, want .csv suffix", result.Filename)
	}

	// Parse CSV and verify header + 2 data rows.
	r := csv.NewReader(strings.NewReader(string(result.Data)))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}
	// header + 2 rows
	if len(records) != 3 {
		t.Fatalf("CSV rows = %d, want 3 (1 header + 2 data)", len(records))
	}
	if records[0][0] != "node_name" {
		t.Errorf("first header = %q, want node_name", records[0][0])
	}
	// Check that web01 and staging01 are present (both ready).
	names := map[string]bool{}
	for _, row := range records[1:] {
		names[row[0]] = true
	}
	if !names["web01"] || !names["staging01"] {
		t.Errorf("expected web01 and staging01 in results, got %v", names)
	}
}

func TestGenerateReadyNodeExport_JSON(t *testing.T) {
	store := testStore()
	ctx := context.Background()

	result, err := GenerateReadyNodeExport(ctx, store, ReadyNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", result.RowCount)
	}

	var rows []readyNodeRow
	if err := json.Unmarshal(result.Data, &rows); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("JSON rows = %d, want 2", len(rows))
	}
}

func TestGenerateReadyNodeExport_ChefSearchQuery(t *testing.T) {
	store := testStore()
	ctx := context.Background()

	result, err := GenerateReadyNodeExport(ctx, store, ReadyNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "chef_search_query",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	query := strings.TrimSpace(string(result.Data))
	if !strings.Contains(query, "name:web01") {
		t.Errorf("query missing web01: %s", query)
	}
	if !strings.Contains(query, " OR ") {
		t.Errorf("query missing OR separator: %s", query)
	}
}

func TestGenerateReadyNodeExport_MissingTargetVersion(t *testing.T) {
	store := testStore()
	_, err := GenerateReadyNodeExport(context.Background(), store, ReadyNodeExportParams{
		Format: "csv",
	})
	if err == nil {
		t.Fatal("expected error for missing target version")
	}
}

func TestGenerateReadyNodeExport_MaxRows(t *testing.T) {
	store := testStore()
	result, err := GenerateReadyNodeExport(context.Background(), store, ReadyNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "json",
		MaxRows:           1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RowCount != 1 {
		t.Errorf("RowCount = %d, want 1", result.RowCount)
	}
}

func TestGenerateReadyNodeExport_EnvironmentFilter(t *testing.T) {
	store := testStore()
	result, err := GenerateReadyNodeExport(context.Background(), store, ReadyNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "json",
		Filters:           Filters{Environment: "production"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only web01 is in production and ready.
	if result.RowCount != 1 {
		t.Errorf("RowCount = %d, want 1 (only production+ready)", result.RowCount)
	}
}

func TestGenerateReadyNodeExport_WriteToDisk(t *testing.T) {
	store := testStore()
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "test_export.csv")

	result, err := GenerateReadyNodeExport(context.Background(), store, ReadyNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "csv",
		OutputPath:        outputPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FilePath != outputPath {
		t.Errorf("FilePath = %q, want %q", result.FilePath, outputPath)
	}
	if result.FileSizeBytes <= 0 {
		t.Error("FileSizeBytes should be > 0")
	}
	if result.Data != nil {
		t.Error("Data should be nil when writing to disk")
	}

	// Verify file exists and is readable.
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	if len(data) == 0 {
		t.Error("output file is empty")
	}
}

// ---------------------------------------------------------------------------
// Blocked node export tests
// ---------------------------------------------------------------------------

func TestGenerateBlockedNodeExport_CSV(t *testing.T) {
	store := testStore()
	result, err := GenerateBlockedNodeExport(context.Background(), store, BlockedNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "csv",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only db01 is blocked.
	if result.RowCount != 1 {
		t.Errorf("RowCount = %d, want 1", result.RowCount)
	}

	r := csv.NewReader(strings.NewReader(string(result.Data)))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}
	if len(records) != 2 { // header + 1 data row
		t.Fatalf("CSV rows = %d, want 2", len(records))
	}
	// Verify blocking_cookbooks column contains "legacy-db".
	blockingCol := records[1][9] // blocking_cookbooks is column index 9
	if !strings.Contains(blockingCol, "legacy-db") {
		t.Errorf("blocking_cookbooks = %q, want to contain legacy-db", blockingCol)
	}
}

func TestGenerateBlockedNodeExport_JSON(t *testing.T) {
	store := testStore()
	result, err := GenerateBlockedNodeExport(context.Background(), store, BlockedNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rows []blockedNodeRow
	if err := json.Unmarshal(result.Data, &rows); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("JSON rows = %d, want 1", len(rows))
	}
	if rows[0].NodeName != "db01" {
		t.Errorf("NodeName = %q, want db01", rows[0].NodeName)
	}
	if len(rows[0].BlockingCookbooks) == 0 {
		t.Error("BlockingCookbooks should not be empty")
	}
	if len(rows[0].BlockingReasons) == 0 {
		t.Error("BlockingReasons should not be empty")
	}
}

func TestGenerateBlockedNodeExport_ChefSearchQueryRejected(t *testing.T) {
	store := testStore()
	_, err := GenerateBlockedNodeExport(context.Background(), store, BlockedNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "chef_search_query",
	})
	if err == nil {
		t.Fatal("expected error for chef_search_query format on blocked nodes")
	}
}

func TestGenerateBlockedNodeExport_ComplexityScore(t *testing.T) {
	store := testStore()
	result, err := GenerateBlockedNodeExport(context.Background(), store, BlockedNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rows []blockedNodeRow
	if err := json.Unmarshal(result.Data, &rows); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	// db01 is blocked by legacy-db (cb-1) which has complexity score 75.
	if len(rows) > 0 && rows[0].ComplexityScore != 75 {
		t.Errorf("ComplexityScore = %d, want 75", rows[0].ComplexityScore)
	}
}

// ---------------------------------------------------------------------------
// Cookbook remediation export tests
// ---------------------------------------------------------------------------

func TestGenerateCookbookRemediationExport_CSV(t *testing.T) {
	store := testStore()
	result, err := GenerateCookbookRemediationExport(context.Background(), store, CookbookRemediationExportParams{
		Format: "csv",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 cookbooks × 1 target version = 2 rows.
	if result.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", result.RowCount)
	}
}

func TestGenerateCookbookRemediationExport_JSON(t *testing.T) {
	store := testStore()
	result, err := GenerateCookbookRemediationExport(context.Background(), store, CookbookRemediationExportParams{
		Format: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rows []cookbookRemediationRow
	if err := json.Unmarshal(result.Data, &rows); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("JSON rows = %d, want 2", len(rows))
	}

	// Verify fields from the first cookbook.
	found := false
	for _, r := range rows {
		if r.CookbookName == "legacy-db" {
			found = true
			if r.ComplexityScore != 75 {
				t.Errorf("ComplexityScore = %d, want 75", r.ComplexityScore)
			}
			if r.ComplexityLabel != "high" {
				t.Errorf("ComplexityLabel = %q, want high", r.ComplexityLabel)
			}
			if r.Organisation != "prod-org" {
				t.Errorf("Organisation = %q, want prod-org", r.Organisation)
			}
		}
	}
	if !found {
		t.Error("legacy-db cookbook not found in results")
	}
}

func TestGenerateCookbookRemediationExport_TargetVersionFilter(t *testing.T) {
	store := testStore()
	result, err := GenerateCookbookRemediationExport(context.Background(), store, CookbookRemediationExportParams{
		Format:  "json",
		Filters: Filters{TargetChefVersion: "99.0.0"}, // non-existent
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RowCount != 0 {
		t.Errorf("RowCount = %d, want 0 for non-existent target version", result.RowCount)
	}
}

func TestGenerateCookbookRemediationExport_ComplexityLabelFilter(t *testing.T) {
	store := testStore()
	result, err := GenerateCookbookRemediationExport(context.Background(), store, CookbookRemediationExportParams{
		Format:  "json",
		Filters: Filters{ComplexityLabel: "high"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only legacy-db has "high" complexity.
	if result.RowCount != 1 {
		t.Errorf("RowCount = %d, want 1", result.RowCount)
	}
}

func TestGenerateCookbookRemediationExport_ChefSearchQueryRejected(t *testing.T) {
	store := testStore()
	_, err := GenerateCookbookRemediationExport(context.Background(), store, CookbookRemediationExportParams{
		Format: "chef_search_query",
	})
	if err == nil {
		t.Fatal("expected error for chef_search_query format on cookbook remediation")
	}
}

// ---------------------------------------------------------------------------
// Filter tests
// ---------------------------------------------------------------------------

func TestFilterNodes_Environment(t *testing.T) {
	nodes := testStore().nodesByOrg["org-1"]
	filtered := FilterNodes(nodes, Filters{Environment: "staging"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (staging only)", len(filtered))
	}
	if filtered[0].NodeName != "staging01" {
		t.Errorf("NodeName = %q, want staging01", filtered[0].NodeName)
	}
}

func TestFilterNodes_Platform(t *testing.T) {
	nodes := testStore().nodesByOrg["org-1"]
	filtered := FilterNodes(nodes, Filters{Platform: "centos"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (centos only)", len(filtered))
	}
}

func TestFilterNodes_Role(t *testing.T) {
	nodes := testStore().nodesByOrg["org-1"]
	filtered := FilterNodes(nodes, Filters{Role: "webserver"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (webserver role)", len(filtered))
	}
}

func TestFilterNodes_PartialMatch(t *testing.T) {
	nodes := testStore().nodesByOrg["org-1"]

	// "prod" is a substring of "production" — should match web01 and db01.
	filtered := FilterNodes(nodes, Filters{Environment: "prod"})
	if len(filtered) != 2 {
		t.Errorf("got %d nodes, want 2 (environment substring 'prod' matches 'production')", len(filtered))
	}

	// "bunt" is a substring of "ubuntu" — should match web01 and staging01.
	filtered = FilterNodes(nodes, Filters{Platform: "bunt"})
	if len(filtered) != 2 {
		t.Errorf("got %d nodes, want 2 (platform substring 'bunt' matches 'ubuntu')", len(filtered))
	}

	// "base" is a substring match on role — all 3 nodes have the "base" role.
	filtered = FilterNodes(nodes, Filters{Role: "base"})
	if len(filtered) != 3 {
		t.Errorf("got %d nodes, want 3 (role substring 'base' matches all nodes)", len(filtered))
	}

	// "web" is a substring of "webserver" role — should match web01.
	filtered = FilterNodes(nodes, Filters{Role: "web"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (role substring 'web' matches 'webserver')", len(filtered))
	}

	// "16" is a substring of "16.0.0" — should match web01 only.
	filtered = FilterNodes(nodes, Filters{ChefVersion: "16"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (chef_version substring '16' matches '16.0.0')", len(filtered))
	}
}

func TestFilterNodes_CaseInsensitive(t *testing.T) {
	nodes := testStore().nodesByOrg["org-1"]

	// Upper-case "STAGING" should match "staging" environment.
	filtered := FilterNodes(nodes, Filters{Environment: "STAGING"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (case-insensitive 'STAGING' matches 'staging')", len(filtered))
	}
	if len(filtered) > 0 && filtered[0].NodeName != "staging01" {
		t.Errorf("NodeName = %q, want staging01", filtered[0].NodeName)
	}

	// Mixed-case "Ubuntu" should match "ubuntu" platform.
	filtered = FilterNodes(nodes, Filters{Platform: "Ubuntu"})
	if len(filtered) != 2 {
		t.Errorf("got %d nodes, want 2 (case-insensitive 'Ubuntu' matches 'ubuntu')", len(filtered))
	}

	// Upper-case "CENTOS" should match "centos" platform.
	filtered = FilterNodes(nodes, Filters{Platform: "CENTOS"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (case-insensitive 'CENTOS' matches 'centos')", len(filtered))
	}

	// Upper-case role filter "WEBSERVER" should match "webserver" role.
	filtered = FilterNodes(nodes, Filters{Role: "WEBSERVER"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (case-insensitive 'WEBSERVER' matches 'webserver')", len(filtered))
	}

	// Case-insensitive partial: "PROD" matches "production".
	filtered = FilterNodes(nodes, Filters{Environment: "PROD"})
	if len(filtered) != 2 {
		t.Errorf("got %d nodes, want 2 (case-insensitive partial 'PROD' matches 'production')", len(filtered))
	}
}

func TestFilterNodes_NodeName(t *testing.T) {
	nodes := testStore().nodesByOrg["org-1"]
	filtered := FilterNodes(nodes, Filters{NodeName: "web"})
	if len(filtered) != 1 {
		t.Errorf("got %d nodes, want 1 (web01 only)", len(filtered))
	}
}

func TestFilterNodes_NoFilter(t *testing.T) {
	nodes := testStore().nodesByOrg["org-1"]
	filtered := FilterNodes(nodes, Filters{})
	if len(filtered) != 3 {
		t.Errorf("got %d nodes, want 3 (no filter)", len(filtered))
	}
}

func TestFilterOrganisations(t *testing.T) {
	orgs := testStore().orgs
	filtered := FilterOrganisations(orgs, "prod-org")
	if len(filtered) != 1 {
		t.Errorf("got %d orgs, want 1", len(filtered))
	}

	filtered = FilterOrganisations(orgs, "nonexistent")
	if len(filtered) != 0 {
		t.Errorf("got %d orgs, want 0", len(filtered))
	}

	filtered = FilterOrganisations(orgs, "")
	if len(filtered) != len(orgs) {
		t.Errorf("got %d orgs, want %d (all)", len(filtered), len(orgs))
	}
}

func TestFilterComplexities(t *testing.T) {
	cc := testStore().complexities["org-1"]

	filtered := FilterComplexities(cc, "18.0.0", "")
	if len(filtered) != 2 {
		t.Errorf("got %d, want 2", len(filtered))
	}

	filtered = FilterComplexities(cc, "18.0.0", "high")
	if len(filtered) != 1 {
		t.Errorf("got %d, want 1 (high only)", len(filtered))
	}

	filtered = FilterComplexities(cc, "99.0.0", "")
	if len(filtered) != 0 {
		t.Errorf("got %d, want 0 (nonexistent version)", len(filtered))
	}
}

func TestParseFilters_Roundtrip(t *testing.T) {
	original := Filters{
		Organisation:      "prod",
		Environment:       "staging",
		TargetChefVersion: "18.0.0",
	}
	data, err := original.MarshalToJSON()
	if err != nil {
		t.Fatalf("MarshalToJSON: %v", err)
	}
	parsed, err := ParseFilters(data)
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}
	if parsed.Organisation != original.Organisation {
		t.Errorf("Organisation = %q, want %q", parsed.Organisation, original.Organisation)
	}
	if parsed.Environment != original.Environment {
		t.Errorf("Environment = %q, want %q", parsed.Environment, original.Environment)
	}
}

func TestParseFilters_NilInput(t *testing.T) {
	f, err := ParseFilters(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.IsEmpty() {
		t.Error("parsed nil should produce empty filters")
	}
}

// ---------------------------------------------------------------------------
// Cleanup tests
// ---------------------------------------------------------------------------

type fakeCleanupStore struct {
	expired []datastore.ExportJob
	marked  []string
}

func (f *fakeCleanupStore) ListExpiredExportJobs(_ context.Context, _ time.Time) ([]datastore.ExportJob, error) {
	return f.expired, nil
}

func (f *fakeCleanupStore) UpdateExportJobExpired(_ context.Context, id string) error {
	f.marked = append(f.marked, id)
	return nil
}

var _ CleanupStore = (*fakeCleanupStore)(nil)

func TestCleanupExpiredExports_DeletesFileAndMarksExpired(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test-export.csv")
	if err := os.WriteFile(filePath, []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}

	store := &fakeCleanupStore{
		expired: []datastore.ExportJob{{
			ID:       "job-1",
			FilePath: filePath,
			Status:   "completed",
		}},
	}

	result, err := CleanupExpiredExports(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.JobsExpired != 1 {
		t.Errorf("JobsExpired = %d, want 1", result.JobsExpired)
	}
	if result.FilesDeleted != 1 {
		t.Errorf("FilesDeleted = %d, want 1", result.FilesDeleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", result.Errors)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
	if len(store.marked) != 1 || store.marked[0] != "job-1" {
		t.Errorf("marked = %v, want [job-1]", store.marked)
	}
}

func TestCleanupExpiredExports_MissingFileStillMarksExpired(t *testing.T) {
	store := &fakeCleanupStore{
		expired: []datastore.ExportJob{{
			ID:       "job-2",
			FilePath: "/nonexistent/path.csv",
			Status:   "completed",
		}},
	}

	result, err := CleanupExpiredExports(context.Background(), store, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.JobsExpired != 1 {
		t.Errorf("JobsExpired = %d, want 1", result.JobsExpired)
	}
	if len(store.marked) != 1 {
		t.Errorf("should still mark job as expired even if file missing")
	}
}

func TestCleanupExpiredExports_NoExpiredJobs(t *testing.T) {
	store := &fakeCleanupStore{expired: nil}
	result, err := CleanupExpiredExports(context.Background(), store, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.JobsExpired != 0 {
		t.Errorf("JobsExpired = %d, want 0", result.JobsExpired)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestGenerateReadyNodeExport_EmptyStore(t *testing.T) {
	store := &fakeStore{orgs: nil}
	result, err := GenerateReadyNodeExport(context.Background(), store, ReadyNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RowCount != 0 {
		t.Errorf("RowCount = %d, want 0", result.RowCount)
	}
	// Should produce valid empty JSON array.
	var rows []readyNodeRow
	if err := json.Unmarshal(result.Data, &rows); err != nil {
		t.Fatalf("invalid JSON for empty result: %v", err)
	}
	if len(rows) != 0 {
		t.Error("expected empty array")
	}
}

func TestGenerateBlockedNodeExport_EmptyStore(t *testing.T) {
	store := &fakeStore{orgs: nil}
	result, err := GenerateBlockedNodeExport(context.Background(), store, BlockedNodeExportParams{
		TargetChefVersion: "18.0.0",
		Format:            "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RowCount != 0 {
		t.Errorf("RowCount = %d, want 0", result.RowCount)
	}
}

func TestGenerateCookbookRemediationExport_EmptyStore(t *testing.T) {
	store := &fakeStore{orgs: nil}
	result, err := GenerateCookbookRemediationExport(context.Background(), store, CookbookRemediationExportParams{
		Format: "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RowCount != 0 {
		t.Errorf("RowCount = %d, want 0", result.RowCount)
	}
}

func TestParseBlockingCookbooks_StringArray(t *testing.T) {
	raw := json.RawMessage(`["cb1","cb2"]`)
	names := parseBlockingCookbooks(raw)
	if len(names) != 2 || names[0] != "cb1" || names[1] != "cb2" {
		t.Errorf("got %v, want [cb1, cb2]", names)
	}
}

func TestParseBlockingCookbooks_ObjectArray(t *testing.T) {
	raw := json.RawMessage(`[{"name":"cb1","version":"1.0"},{"name":"cb2"}]`)
	names := parseBlockingCookbooks(raw)
	if len(names) != 2 {
		t.Fatalf("got %d names, want 2", len(names))
	}
	if names[0] != "cb1@1.0" {
		t.Errorf("names[0] = %q, want cb1@1.0", names[0])
	}
	if names[1] != "cb2" {
		t.Errorf("names[1] = %q, want cb2", names[1])
	}
}

func TestParseBlockingCookbooks_Null(t *testing.T) {
	names := parseBlockingCookbooks(nil)
	if len(names) != 0 {
		t.Errorf("got %v, want empty", names)
	}
	names = parseBlockingCookbooks(json.RawMessage("null"))
	if len(names) != 0 {
		t.Errorf("got %v, want empty", names)
	}
}

func TestDeriveBlockingReasons(t *testing.T) {
	diskFalse := false
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		SufficientDiskSpace:    &diskFalse,
		StaleData:              true,
	}
	reasons := deriveBlockingReasons(nr)
	if len(reasons) != 3 {
		t.Errorf("got %d reasons, want 3", len(reasons))
	}
}

func TestDeriveBlockingReasons_Unspecified(t *testing.T) {
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: true,
		IsReady:                false,
	}
	reasons := deriveBlockingReasons(nr)
	if len(reasons) != 1 || reasons[0] != "blocked (reason unspecified)" {
		t.Errorf("got %v, want [blocked (reason unspecified)]", reasons)
	}
}

func TestFilters_IsEmpty(t *testing.T) {
	if !(Filters{}).IsEmpty() {
		t.Error("zero Filters should be empty")
	}
	if (Filters{Organisation: "x"}).IsEmpty() {
		t.Error("Filters with Organisation set should not be empty")
	}
}
