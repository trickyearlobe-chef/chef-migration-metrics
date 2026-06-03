// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
)

// ---------------------------------------------------------------------------
// Bulk-load interface methods on fakeReadinessDS
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Fake datastore for testing
// ---------------------------------------------------------------------------

type fakeReadinessDS struct {
	mu sync.Mutex

	snapshots       []datastore.NodeSnapshot
	cookbookIDs     map[string]map[string]string // name → version → id
	serverCookbooks []datastore.ServerCookbook
	gitRepos        map[string]datastore.GitRepo // name → GitRepo
	csResults       map[string]*datastore.ServerCookbookCookstyleResult
	complexities    map[string]*datastore.ServerCookbookComplexity
	gitCSResults    map[string]*datastore.GitRepoCookstyleResult
	gitComplexities map[string]*datastore.GitRepoComplexity
	gitTKStatuses   map[string]string // repoName|target → "passed"/"failed"/"partial"
	upserted        []datastore.UpsertNodeReadinessParams

	// Error injection
	listSnapshotsErr       error
	listServerCookbooksErr error
	gitRepoErr             error
	csErr                  error
	complexityErr          error
	gitCSErr               error
	gitComplexityErr       error
	gitTKErr               error
	upsertErr              error

	// Call counters
	upsertCount atomic.Int64
}

func newFakeReadinessDS() *fakeReadinessDS {
	return &fakeReadinessDS{
		cookbookIDs:     make(map[string]map[string]string),
		gitRepos:        make(map[string]datastore.GitRepo),
		csResults:       make(map[string]*datastore.ServerCookbookCookstyleResult),
		complexities:    make(map[string]*datastore.ServerCookbookComplexity),
		gitCSResults:    make(map[string]*datastore.GitRepoCookstyleResult),
		gitComplexities: make(map[string]*datastore.GitRepoComplexity),
		gitTKStatuses:   make(map[string]string),
	}
}

func (f *fakeReadinessDS) ListNodeSnapshotsByOrganisation(_ context.Context, _ string) ([]datastore.NodeSnapshot, error) {
	if f.listSnapshotsErr != nil {
		return nil, f.listSnapshotsErr
	}
	return f.snapshots, nil
}

func (f *fakeReadinessDS) ListServerCookbooksByOrganisation(_ context.Context, _ string) ([]datastore.ServerCookbook, error) {
	if f.listServerCookbooksErr != nil {
		return nil, f.listServerCookbooksErr
	}
	return f.serverCookbooks, nil
}

func (f *fakeReadinessDS) GetGitRepoByName(_ context.Context, name string) (datastore.GitRepo, error) {
	if f.gitRepoErr != nil {
		return datastore.GitRepo{}, f.gitRepoErr
	}
	gr, ok := f.gitRepos[name]
	if !ok {
		return datastore.GitRepo{}, fmt.Errorf("not found")
	}
	return gr, nil
}

func csKey(orgName, cookbookName, cookbookVersion, targetChefVersion string) string {
	return orgName + "/" + cookbookName + "/" + cookbookVersion + "|" + targetChefVersion
}

func ccKey(orgName, cookbookName, cookbookVersion, targetChefVersion string) string {
	return orgName + "/" + cookbookName + "/" + cookbookVersion + "|" + targetChefVersion
}

func (f *fakeReadinessDS) GetServerCookbookCookstyleResult(_ context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error) {
	if f.csErr != nil {
		return nil, f.csErr
	}
	r := f.csResults[csKey(orgName, cookbookName, cookbookVersion, targetChefVersion)]
	return r, nil
}

func (f *fakeReadinessDS) GetServerCookbookComplexity(_ context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookComplexity, error) {
	if f.complexityErr != nil {
		return nil, f.complexityErr
	}
	r := f.complexities[ccKey(orgName, cookbookName, cookbookVersion, targetChefVersion)]
	return r, nil
}

func gitCSKey(gitRepoName, targetChefVersion string) string {
	return gitRepoName + "|" + targetChefVersion
}

func gcKey(gitRepoName, targetChefVersion string) string {
	return gitRepoName + "|" + targetChefVersion
}

func (f *fakeReadinessDS) GetGitRepoCookstyleResult(_ context.Context, gitRepoName, _, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error) {
	if f.gitCSErr != nil {
		return nil, f.gitCSErr
	}
	r := f.gitCSResults[gitCSKey(gitRepoName, targetChefVersion)]
	return r, nil
}

func (f *fakeReadinessDS) GetGitRepoComplexity(_ context.Context, gitRepoName, _, targetChefVersion string) (*datastore.GitRepoComplexity, error) {
	if f.gitComplexityErr != nil {
		return nil, f.gitComplexityErr
	}
	r := f.gitComplexities[gcKey(gitRepoName, targetChefVersion)]
	return r, nil
}

func (f *fakeReadinessDS) UpsertNodeReadiness(_ context.Context, p datastore.UpsertNodeReadinessParams) (*datastore.NodeReadiness, error) {
	f.upsertCount.Add(1)
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	f.mu.Lock()
	f.upserted = append(f.upserted, p)
	f.mu.Unlock()
	return &datastore.NodeReadiness{
		OrganisationName:  p.OrganisationName,
		NodeName:          p.NodeName,
		TargetChefVersion: p.TargetChefVersion,
		IsReady:           p.IsReady,
	}, nil
}

// --- Bulk-load methods (ReadinessDataStore interface) ---

func (f *fakeReadinessDS) ListGitRepos(_ context.Context) ([]datastore.GitRepo, error) {
	if f.gitRepoErr != nil {
		return nil, f.gitRepoErr
	}
	repos := make([]datastore.GitRepo, 0, len(f.gitRepos))
	for _, gr := range f.gitRepos {
		repos = append(repos, gr)
	}
	return repos, nil
}

func (f *fakeReadinessDS) ListGitRepoCookstyleResultsByTargetVersions(_ context.Context, targetChefVersions []string) ([]datastore.GitRepoCookstyleResult, error) {
	if f.gitCSErr != nil {
		return nil, f.gitCSErr
	}
	tvSet := make(map[string]bool, len(targetChefVersions))
	for _, tv := range targetChefVersions {
		tvSet[tv] = true
	}
	var results []datastore.GitRepoCookstyleResult
	for _, r := range f.gitCSResults {
		if r != nil && tvSet[r.TargetChefVersion] {
			results = append(results, *r)
		}
	}
	return results, nil
}

func (f *fakeReadinessDS) ListServerCookbookCookstyleResultsByOrganisationAndVersions(_ context.Context, _ string, targetChefVersions []string) ([]datastore.ServerCookbookCookstyleResult, error) {
	if f.csErr != nil {
		return nil, f.csErr
	}
	tvSet := make(map[string]bool, len(targetChefVersions))
	for _, tv := range targetChefVersions {
		tvSet[tv] = true
	}
	var results []datastore.ServerCookbookCookstyleResult
	for _, r := range f.csResults {
		if r != nil && (tvSet[r.TargetChefVersion] || r.TargetChefVersion == "") {
			results = append(results, *r)
		}
	}
	return results, nil
}

func (f *fakeReadinessDS) ListServerCookbookComplexities(_ context.Context, _ string, targetChefVersions []string) ([]datastore.ServerCookbookComplexity, error) {
	if f.complexityErr != nil {
		return nil, f.complexityErr
	}
	tvSet := make(map[string]bool, len(targetChefVersions))
	for _, tv := range targetChefVersions {
		tvSet[tv] = true
	}
	var results []datastore.ServerCookbookComplexity
	for _, r := range f.complexities {
		if r != nil && tvSet[r.TargetChefVersion] {
			results = append(results, *r)
		}
	}
	return results, nil
}

func (f *fakeReadinessDS) ListGitRepoComplexities(_ context.Context, targetChefVersions []string) ([]datastore.GitRepoComplexity, error) {
	if f.gitComplexityErr != nil {
		return nil, f.gitComplexityErr
	}
	tvSet := make(map[string]bool, len(targetChefVersions))
	for _, tv := range targetChefVersions {
		tvSet[tv] = true
	}
	var results []datastore.GitRepoComplexity
	for _, r := range f.gitComplexities {
		if r != nil && tvSet[r.TargetChefVersion] {
			results = append(results, *r)
		}
	}
	return results, nil
}

func (f *fakeReadinessDS) ListGitKitchenCountsByTargetVersions(_ context.Context, targetChefVersions []string) (map[string]tkstatus.Counts, error) {
	if f.gitTKErr != nil {
		return nil, f.gitTKErr
	}
	tvSet := make(map[string]bool, len(targetChefVersions))
	for _, tv := range targetChefVersions {
		tvSet[tv] = true
	}
	result := make(map[string]tkstatus.Counts)
	for k, v := range f.gitTKStatuses {
		parts := strings.SplitN(k, "|", 2)
		if len(parts) == 2 && tvSet[parts[1]] {
			switch v {
			case "passed":
				result[k] = tkstatus.Counts{Passed: 1}
			case "failed":
				result[k] = tkstatus.Counts{Failed: 1}
			case "partial":
				result[k] = tkstatus.Counts{Passed: 1, Failed: 1}
			}
		}
	}
	return result, nil
}

// buildFakeCache constructs a readinessCache directly from the fake's in-memory
// maps without going through the bulk-load DB path. This lets unit tests for
// checkCookbookCompatibility and evaluateOne work with the cache directly.
func (f *fakeReadinessDS) buildFakeCache() *readinessCache {
	cache := &readinessCache{
		gitRepos:         make(map[string]datastore.GitRepo),
		gitCSResults:     make(map[string]*datastore.GitRepoCookstyleResult),
		serverCSResults:  make(map[string]*datastore.ServerCookbookCookstyleResult),
		serverComplexity: make(map[string]*datastore.ServerCookbookComplexity),
		gitComplexity:    make(map[string]*datastore.GitRepoComplexity),
		gitTKStatuses:    make(map[string]string),
	}
	for name, gr := range f.gitRepos {
		cache.gitRepos[name] = gr
	}
	for k, v := range f.gitCSResults {
		cache.gitCSResults[k] = v
	}
	for k, v := range f.csResults {
		cache.serverCSResults[k] = v
	}
	for k, v := range f.complexities {
		cache.serverComplexity[k] = v
	}
	for k, v := range f.gitComplexities {
		cache.gitComplexity[k] = v
	}
	for k, v := range f.gitTKStatuses {
		cache.gitTKStatuses[k] = v
	}
	return cache
}

// --- Add helpers ---

func (f *fakeReadinessDS) addCookbookID(name, version, orgName string) {
	// orgName is used as the organisation name for the server cookbook.
	// The composite ID (orgName/name/version) is stored in cookbookIDs and
	// also matches what buildCookbookIDMap produces from serverCookbooks.
	compositeID := orgName + "/" + name + "/" + version
	if f.cookbookIDs[name] == nil {
		f.cookbookIDs[name] = make(map[string]string)
	}
	f.cookbookIDs[name][version] = compositeID
	f.serverCookbooks = append(f.serverCookbooks, datastore.ServerCookbook{
		OrganisationName: orgName,
		Name:             name,
		Version:          version,
	})
}

func (f *fakeReadinessDS) addCSResult(orgName, cookbookName, cookbookVersion, targetChefVersion string, passed bool) {
	f.csResults[csKey(orgName, cookbookName, cookbookVersion, targetChefVersion)] = &datastore.ServerCookbookCookstyleResult{
		OrganisationName:  orgName,
		CookbookName:      cookbookName,
		CookbookVersion:   cookbookVersion,
		TargetChefVersion: targetChefVersion,
		Passed:            passed,
	}
}

func (f *fakeReadinessDS) addComplexity(orgName, cookbookName, cookbookVersion, targetChefVersion string, score int, label string) {
	f.complexities[ccKey(orgName, cookbookName, cookbookVersion, targetChefVersion)] = &datastore.ServerCookbookComplexity{
		OrganisationName:  orgName,
		CookbookName:      cookbookName,
		CookbookVersion:   cookbookVersion,
		TargetChefVersion: targetChefVersion,
		ComplexityScore:   score,
		ComplexityLabel:   label,
	}
}

func (f *fakeReadinessDS) addGitCSResult(gitRepoName, targetChefVersion string, passed bool) {
	f.gitCSResults[gitCSKey(gitRepoName, targetChefVersion)] = &datastore.GitRepoCookstyleResult{
		GitRepoName:       gitRepoName,
		TargetChefVersion: targetChefVersion,
		Passed:            passed,
	}
}

func (f *fakeReadinessDS) addGitRepo(name, headSHA string) {
	f.gitRepos[name] = datastore.GitRepo{
		Name:          name,
		HeadCommitSHA: headSHA,
	}
}

func (f *fakeReadinessDS) addGitRepoWithTK(name, headSHA string, hasTestSuite, kitchenExcluded bool) {
	f.gitRepos[name] = datastore.GitRepo{
		Name:            name,
		HeadCommitSHA:   headSHA,
		HasTestSuite:    hasTestSuite,
		KitchenExcluded: kitchenExcluded,
	}
}

func (f *fakeReadinessDS) addGitTKStatus(repoName, targetVersion, status string) {
	if f.gitTKStatuses == nil {
		f.gitTKStatuses = make(map[string]string)
	}
	f.gitTKStatuses[repoName+"|"+targetVersion] = status
}

// ---------------------------------------------------------------------------
// Helper to make a node snapshot
// ---------------------------------------------------------------------------

func makeSnapshot(orgName, nodeName string, isStale bool, cookbooks, filesystem json.RawMessage) datastore.NodeSnapshot {
	return datastore.NodeSnapshot{
		OrganisationName: orgName,
		NodeName:         nodeName,
		IsStale:          isStale,
		Cookbooks:        cookbooks,
		Filesystem:       filesystem,
		CollectedAt:      time.Now().UTC(),
	}
}

func cookbooksJSON(cookbooks map[string]string) json.RawMessage {
	// Convert to {"name": {"version": "X.Y.Z"}, ...}
	m := make(map[string]map[string]string, len(cookbooks))
	for name, ver := range cookbooks {
		m[name] = map[string]string{"version": ver}
	}
	b, _ := json.Marshal(m)
	return b
}

func linuxFilesystemJSON(mounts map[string]linuxMount) json.RawMessage {
	m := make(map[string]map[string]interface{}, len(mounts))
	for dev, info := range mounts {
		entry := make(map[string]interface{})
		entry["kb_size"] = info.KBSize
		entry["kb_used"] = info.KBUsed
		entry["kb_available"] = info.KBAvailable
		entry["percent_used"] = info.PercentUsed
		entry["mount"] = info.Mount
		m[dev] = entry
	}
	b, _ := json.Marshal(m)
	return b
}

type linuxMount struct {
	KBSize      interface{}
	KBUsed      interface{}
	KBAvailable interface{}
	PercentUsed interface{}
	Mount       interface{}
}

func windowsFilesystemJSON(drives map[string]windowsDrive) json.RawMessage {
	m := make(map[string]map[string]interface{}, len(drives))
	for key, info := range drives {
		entry := make(map[string]interface{})
		entry["kb_size"] = info.KBSize
		entry["kb_used"] = info.KBUsed
		entry["kb_available"] = info.KBAvailable
		entry["percent_used"] = info.PercentUsed
		m[key] = entry
	}
	b, _ := json.Marshal(m)
	return b
}

type windowsDrive struct {
	KBSize      interface{}
	KBUsed      interface{}
	KBAvailable interface{}
	PercentUsed interface{}
}

// ---------------------------------------------------------------------------
// Ohai 14+ filesystem format helpers (by_pair / by_device / by_mountpoint)
// ---------------------------------------------------------------------------

type ohaiPairEntry struct {
	Mount       string      `json:"mount"`
	Device      string      `json:"device"`
	FSType      string      `json:"fs_type"`
	KBSize      interface{} `json:"kb_size,omitempty"`
	KBUsed      interface{} `json:"kb_used,omitempty"`
	KBAvailable interface{} `json:"kb_available,omitempty"`
	PercentUsed interface{} `json:"percent_used,omitempty"`
}

// ohai14LinuxFilesystemJSON builds an Ohai 14+ filesystem JSON with by_pair,
// by_device, and by_mountpoint sections. The pairs map is keyed by "device,mount".
func ohai14LinuxFilesystemJSON(pairs map[string]ohaiPairEntry) json.RawMessage {
	byPair := make(map[string]interface{}, len(pairs))
	byMount := make(map[string]interface{}, len(pairs))
	byDevice := make(map[string]interface{}, len(pairs))
	for key, entry := range pairs {
		byPair[key] = entry
		byMount[entry.Mount] = entry
		byDevice[entry.Device] = entry
	}
	m := map[string]interface{}{
		"by_pair":       byPair,
		"by_mountpoint": byMount,
		"by_device":     byDevice,
	}
	b, _ := json.Marshal(m)
	return b
}

// ohai14WindowsFilesystemJSON builds an Ohai 14+ filesystem JSON for Windows.
func ohai14WindowsFilesystemJSON(pairs map[string]ohaiPairEntry) json.RawMessage {
	return ohai14LinuxFilesystemJSON(pairs) // same structure
}

// ---------------------------------------------------------------------------
// parseCookbooksAttribute tests
// ---------------------------------------------------------------------------

func TestParseCookbooksAttribute_StandardFormat(t *testing.T) {
	raw := json.RawMessage(`{
		"apt": {"version": "7.4.0"},
		"nginx": {"version": "2.0.0"},
		"java": {"version": "8.5.0"}
	}`)
	result := parseCookbooksAttribute(raw)
	if len(result) != 3 {
		t.Fatalf("expected 3 cookbooks, got %d", len(result))
	}
	if result["apt"] != "7.4.0" {
		t.Errorf("apt: expected 7.4.0, got %s", result["apt"])
	}
	if result["nginx"] != "2.0.0" {
		t.Errorf("nginx: expected 2.0.0, got %s", result["nginx"])
	}
}

func TestParseCookbooksAttribute_SimpleFormat(t *testing.T) {
	raw := json.RawMessage(`{"apt": "7.4.0", "nginx": "2.0.0"}`)
	result := parseCookbooksAttribute(raw)
	if len(result) != 2 {
		t.Fatalf("expected 2 cookbooks, got %d", len(result))
	}
	if result["apt"] != "7.4.0" {
		t.Errorf("apt: expected 7.4.0, got %s", result["apt"])
	}
}

func TestParseCookbooksAttribute_Nil(t *testing.T) {
	result := parseCookbooksAttribute(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseCookbooksAttribute_Empty(t *testing.T) {
	result := parseCookbooksAttribute(json.RawMessage(`{}`))
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseCookbooksAttribute_InvalidJSON(t *testing.T) {
	result := parseCookbooksAttribute(json.RawMessage(`not json`))
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseCookbooksAttribute_EmptyVersion(t *testing.T) {
	raw := json.RawMessage(`{"apt": {"version": ""}, "nginx": {"version": "2.0.0"}}`)
	result := parseCookbooksAttribute(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1 cookbook (empty version skipped), got %d", len(result))
	}
	if result["nginx"] != "2.0.0" {
		t.Errorf("nginx: expected 2.0.0, got %s", result["nginx"])
	}
}

// ---------------------------------------------------------------------------
// parseFilesystemAttribute tests
// ---------------------------------------------------------------------------

func TestParseFilesystemAttribute_Linux(t *testing.T) {
	raw := linuxFilesystemJSON(map[string]linuxMount{
		"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
	})
	result := parseFilesystemAttribute(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	entry := result["/dev/sda1"]
	if toString(entry.Mount) != "/" {
		t.Errorf("mount: expected /, got %s", toString(entry.Mount))
	}
}

func TestParseFilesystemAttribute_Ohai14_ByPair(t *testing.T) {
	raw := ohai14LinuxFilesystemJSON(map[string]ohaiPairEntry{
		"/dev/vda2,/": {
			Mount: "/", Device: "/dev/vda2", FSType: "ext4",
			KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%",
		},
		"tmpfs,/run": {
			Mount: "/run", Device: "tmpfs", FSType: "tmpfs",
			KBSize: "3206072", KBUsed: "75484", KBAvailable: "3130588", PercentUsed: "3%",
		},
		"none,/dev": {
			Mount: "/dev", Device: "none", FSType: "devtmpfs",
		},
	})
	result := parseFilesystemAttribute(raw)
	if result == nil {
		t.Fatal("expected non-nil result for Ohai 14+ format")
	}
	// Should have extracted from by_pair.
	if len(result) != 3 {
		t.Fatalf("expected 3 entries from by_pair, got %d", len(result))
	}
	// Verify the root entry has the right data.
	root := result["/dev/vda2,/"]
	if toString(root.Mount) != "/" {
		t.Errorf("root mount: expected /, got %s", toString(root.Mount))
	}
	kbAvail := toInt64(root.KBAvailable)
	if kbAvail != 14340800 {
		t.Errorf("root kb_available: expected 14340800, got %d", kbAvail)
	}
}

func TestParseFilesystemAttribute_Ohai14_Windows(t *testing.T) {
	raw := ohai14WindowsFilesystemJSON(map[string]ohaiPairEntry{
		",C:": {
			Mount: "C:", Device: "", FSType: "ntfs",
			KBSize: 41949327, KBUsed: 41488511, KBAvailable: 460816, PercentUsed: 98,
		},
	})
	result := parseFilesystemAttribute(raw)
	if result == nil {
		t.Fatal("expected non-nil result for Ohai 14+ Windows format")
	}
	entry := result[",C:"]
	if toString(entry.Mount) != "C:" {
		t.Errorf("mount: expected C:, got %s", toString(entry.Mount))
	}
	kbAvail := toInt64(entry.KBAvailable)
	if kbAvail != 460816 {
		t.Errorf("kb_available: expected 460816, got %d", kbAvail)
	}
}

func TestParseFilesystemAttribute_Ohai14_EmptyByPair(t *testing.T) {
	// by_pair exists but is empty — falls through to legacy parse which
	// produces 3 entries (by_pair, by_device, by_mountpoint as keys with
	// empty filesystemEntry values). These have no mount or kb_available
	// so findBestMount will correctly return nothing.
	raw := json.RawMessage(`{"by_pair": {}, "by_device": {}, "by_mountpoint": {}}`)
	result := parseFilesystemAttribute(raw)
	if len(result) != 3 {
		t.Errorf("expected 3 entries from legacy fallback, got %d", len(result))
	}
}

func TestParseFilesystemAttribute_Nil(t *testing.T) {
	result := parseFilesystemAttribute(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseFilesystemAttribute_InvalidJSON(t *testing.T) {
	result := parseFilesystemAttribute(json.RawMessage(`bad`))
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// toInt64 tests
// ---------------------------------------------------------------------------

func TestToInt64_String(t *testing.T) {
	cases := []struct {
		input    interface{}
		expected int64
	}{
		{"1024", 1024},
		{"  2048  ", 2048},
		{"0", 0},
		{"", -1},
		{nil, -1},
		{"abc", -1},
		{"12345.0", 12345},
		{"12345.9", 12345},
	}
	for _, tc := range cases {
		got := toInt64(tc.input)
		if got != tc.expected {
			t.Errorf("toInt64(%v): expected %d, got %d", tc.input, tc.expected, got)
		}
	}
}

func TestToInt64_Numeric(t *testing.T) {
	cases := []struct {
		input    interface{}
		expected int64
	}{
		{float64(1024), 1024},
		{float64(1024.9), 1024},
		{float32(512), 512},
		{int(256), 256},
		{int64(128), 128},
		{int32(64), 64},
	}
	for _, tc := range cases {
		got := toInt64(tc.input)
		if got != tc.expected {
			t.Errorf("toInt64(%v): expected %d, got %d", tc.input, tc.expected, got)
		}
	}
}

func TestToInt64_UnknownType(t *testing.T) {
	got := toInt64([]string{"not a number"})
	if got != -1 {
		t.Errorf("expected -1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// toString tests
// ---------------------------------------------------------------------------

func TestToString(t *testing.T) {
	cases := []struct {
		input    interface{}
		expected string
	}{
		{nil, ""},
		{"hello", "hello"},
		{float64(42), "42"},
		{int(7), "7"},
		{int64(99), "99"},
		{true, "true"},
	}
	for _, tc := range cases {
		got := toString(tc.input)
		if got != tc.expected {
			t.Errorf("toString(%v): expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// determineInstallPath tests
// ---------------------------------------------------------------------------

func TestDetermineInstallPath(t *testing.T) {
	cases := []struct {
		platform string
		expected string
	}{
		{"ubuntu", "/hab"},
		{"centos", "/hab"},
		{"", "/hab"},
		{"windows", `C:\hab`},
		{"Windows", `C:\hab`},
		{"WINDOWS", `C:\hab`},
	}
	for _, tc := range cases {
		got := determineInstallPath(tc.platform)
		if got != tc.expected {
			t.Errorf("determineInstallPath(%q): expected %q, got %q", tc.platform, tc.expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// isPathPrefix tests
// ---------------------------------------------------------------------------

func TestIsPathPrefix(t *testing.T) {
	cases := []struct {
		prefix   string
		path     string
		expected bool
	}{
		{"/", "/hab", true},
		{"/", "/", true},
		{"/hab", "/hab", true},
		{"/hab", "/hab/svc", true},
		{"/opt", "/opt/hab", true},
		{"/opt", "/optional", false},
		{"/opt", "/opt", true},
		{"/opt/data", "/opt/data/hab", true},
		{"/opt/data", "/opt/database", false},
		{"/var", "/hab", false},
	}
	for _, tc := range cases {
		got := isPathPrefix(tc.prefix, tc.path)
		if got != tc.expected {
			t.Errorf("isPathPrefix(%q, %q): expected %v, got %v", tc.prefix, tc.path, tc.expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// findBestMount — Linux tests
// ---------------------------------------------------------------------------

func TestFindBestMountLinux_RootOnly(t *testing.T) {
	fsMap := parseFilesystemAttribute(linuxFilesystemJSON(map[string]linuxMount{
		"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
	}))
	_, entry := findBestMount(fsMap, "/hab", "ubuntu")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if toInt64(entry.KBAvailable) != 14340800 {
		t.Errorf("expected 14340800, got %d", toInt64(entry.KBAvailable))
	}
}

func TestFindBestMountLinux_DedicatedHabMount(t *testing.T) {
	fsMap := parseFilesystemAttribute(linuxFilesystemJSON(map[string]linuxMount{
		"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		"/dev/sdb1": {KBSize: "102400000", KBUsed: "50000000", KBAvailable: "47360000", PercentUsed: "51%", Mount: "/hab"},
	}))
	_, entry := findBestMount(fsMap, "/hab", "ubuntu")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	// Should prefer /hab over /
	if toInt64(entry.KBAvailable) != 47360000 {
		t.Errorf("expected 47360000 (dedicated /hab), got %d", toInt64(entry.KBAvailable))
	}
}

func TestFindBestMountLinux_OptMount(t *testing.T) {
	fsMap := parseFilesystemAttribute(linuxFilesystemJSON(map[string]linuxMount{
		"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		"/dev/sdb1": {KBSize: "102400000", KBUsed: "50000000", KBAvailable: "47360000", PercentUsed: "51%", Mount: "/opt"},
	}))
	// /opt is NOT a prefix of /hab, so root should match
	_, entry := findBestMount(fsMap, "/hab", "centos")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if toInt64(entry.KBAvailable) != 14340800 {
		t.Errorf("expected root mount (14340800), got %d", toInt64(entry.KBAvailable))
	}
}

func TestFindBestMountLinux_NoMountField(t *testing.T) {
	// Filesystem entries without a mount field should be skipped.
	raw := json.RawMessage(`{"/dev/sda1": {"kb_available": "1000"}}`)
	fsMap := parseFilesystemAttribute(raw)
	_, entry := findBestMount(fsMap, "/hab", "ubuntu")
	if entry != nil {
		t.Errorf("expected nil when no mount field, got %+v", entry)
	}
}

func TestFindBestMountLinux_Empty(t *testing.T) {
	_, entry := findBestMount(nil, "/hab", "ubuntu")
	if entry != nil {
		t.Errorf("expected nil, got %+v", entry)
	}
}

// ---------------------------------------------------------------------------
// findBestMount — Windows tests
// ---------------------------------------------------------------------------

func TestFindBestMountWindows_DriveKey(t *testing.T) {
	fsMap := parseFilesystemAttribute(windowsFilesystemJSON(map[string]windowsDrive{
		"C:": {KBSize: "104857600", KBUsed: "52428800", KBAvailable: "52428800", PercentUsed: "50%"},
	}))
	_, entry := findBestMount(fsMap, `C:\hab`, "windows")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if toInt64(entry.KBAvailable) != 52428800 {
		t.Errorf("expected 52428800, got %d", toInt64(entry.KBAvailable))
	}
}

func TestFindBestMountWindows_DriveKeyWithBackslash(t *testing.T) {
	fsMap := parseFilesystemAttribute(windowsFilesystemJSON(map[string]windowsDrive{
		`C:\`: {KBSize: "104857600", KBUsed: "52428800", KBAvailable: "52428800", PercentUsed: "50%"},
	}))
	_, entry := findBestMount(fsMap, `C:\hab`, "windows")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
}

func TestFindBestMountWindows_NoDriveMatch(t *testing.T) {
	fsMap := parseFilesystemAttribute(windowsFilesystemJSON(map[string]windowsDrive{
		"D:": {KBSize: "104857600", KBUsed: "52428800", KBAvailable: "52428800", PercentUsed: "50%"},
	}))
	_, entry := findBestMount(fsMap, `C:\hab`, "windows")
	if entry != nil {
		t.Errorf("expected nil when drive not found, got %+v", entry)
	}
}

// ---------------------------------------------------------------------------
// evaluateDiskSpace tests
// ---------------------------------------------------------------------------

func TestEvaluateDiskSpace_LinuxSufficientSpace(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))
	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known, got unknown")
	}
	expected := 14340800 / 1024 // ~14004 MB
	if availMB != expected {
		t.Errorf("expected %d MB, got %d MB", expected, availMB)
	}
}

func TestEvaluateDiskSpace_LinuxInsufficientSpace(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "2097152", KBUsed: "1048576", KBAvailable: "1048576", PercentUsed: "50%", Mount: "/"},
		}))
	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known, got unknown")
	}
	expected := 1048576 / 1024 // 1024 MB
	if availMB != expected {
		t.Errorf("expected %d MB, got %d MB", expected, availMB)
	}
}

func TestEvaluateDiskSpace_MissingFilesystem(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil, nil)
	_, _, known := e.evaluateDiskSpace(snap)
	if known {
		t.Error("expected unknown for missing filesystem")
	}
}

func TestEvaluateDiskSpace_EmptyFilesystem(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil, json.RawMessage(`{}`))
	_, _, known := e.evaluateDiskSpace(snap)
	if known {
		t.Error("expected unknown for empty filesystem")
	}
}

func TestEvaluateDiskSpace_StringValues(t *testing.T) {
	// Chef Client versions may report string or integer values.
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))
	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known")
	}
	if availMB != 14340800/1024 {
		t.Errorf("expected %d, got %d", 14340800/1024, availMB)
	}
}

func TestEvaluateDiskSpace_IntegerValues(t *testing.T) {
	// Use raw JSON with numeric (non-string) values.
	raw := json.RawMessage(`{"/dev/sda1": {"kb_size": 20511356, "kb_used": 5123456, "kb_available": 10240000, "percent_used": "26%", "mount": "/"}}`)
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil, raw)
	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known")
	}
	if availMB != 10240000/1024 {
		t.Errorf("expected %d, got %d", 10240000/1024, availMB)
	}
}

func TestEvaluateDiskSpace_MissingKBAvailable(t *testing.T) {
	raw := json.RawMessage(`{"/dev/sda1": {"kb_size": "20511356", "mount": "/"}}`)
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil, raw)
	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known (with 0 available)")
	}
	if availMB != 0 {
		t.Errorf("expected 0 MB when kb_available missing, got %d", availMB)
	}
}

func TestEvaluateDiskSpace_WindowsDrive(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil,
		windowsFilesystemJSON(map[string]windowsDrive{
			"C:": {KBSize: "104857600", KBUsed: "52428800", KBAvailable: "52428800", PercentUsed: "50%"},
		}))
	snap.Platform = "windows"
	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known")
	}
	if availMB != 52428800/1024 {
		t.Errorf("expected %d, got %d", 52428800/1024, availMB)
	}
}

// ---------------------------------------------------------------------------
// lookupCookbookID tests
// ---------------------------------------------------------------------------

func TestLookupCookbookID(t *testing.T) {
	// Note: lookupCookbookID works with whatever ID strings are stored in the map.
	// After the natural-keys migration, IDs are composite "org/name/version" strings,
	// but this test exercises the lookup logic with simple values.
	idMap := map[string]map[string]string{
		"apt":   {"7.4.0": "id-apt-740", "7.3.0": "id-apt-730"},
		"nginx": {"2.0.0": "id-nginx-200"},
	}
	cases := []struct {
		name, version, expected string
	}{
		{"apt", "7.4.0", "id-apt-740"},
		{"apt", "7.3.0", "id-apt-730"},
		{"apt", "9.9.9", ""},
		{"nginx", "2.0.0", "id-nginx-200"},
		{"unknown", "1.0.0", ""},
	}
	for _, tc := range cases {
		got := lookupCookbookID(idMap, tc.name, tc.version)
		if got != tc.expected {
			t.Errorf("lookupCookbookID(%q, %q): expected %q, got %q", tc.name, tc.version, tc.expected, got)
		}
	}
}

func TestLookupCookbookID_NilMap(t *testing.T) {
	got := lookupCookbookID(nil, "apt", "1.0.0")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// checkCookbookCompatibility tests
// ---------------------------------------------------------------------------

// NOTE: Test Kitchen results have been removed from checkCookbookCompatibility.
// All compatibility evaluation is now based on CookStyle results only.
// Cookbooks with no CookStyle results appear as untested.

func TestCheckCookbookCompatibility_TKOnlyIsUntested(t *testing.T) {
	// With TK removed, a cookbook with only a git repo (no CookStyle results)
	// is untested.
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addGitRepo("apt", "abc123")

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusUntested {
		t.Errorf("expected %s (no TK or CS results), got %s", StatusUntested, status)
	}
	if source != SourceNone {
		t.Errorf("expected %s, got %s", SourceNone, source)
	}
}

func TestCheckCookbookCompatibility_TKConvergeFailIsUntested(t *testing.T) {
	// With TK removed, a cookbook with only a git repo (no CookStyle results)
	// is untested regardless of what TK would have reported.
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addGitRepo("apt", "abc123")

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusUntested {
		t.Errorf("expected %s (no CookStyle results), got %s", StatusUntested, status)
	}
	if source != SourceNone {
		t.Errorf("expected %s, got %s", SourceNone, source)
	}
}

func TestCheckCookbookCompatibility_TKTestFailIsUntested(t *testing.T) {
	// With TK removed, a cookbook with only a git repo (no CookStyle results)
	// is untested regardless of what TK would have reported.
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addGitRepo("apt", "abc123")

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusUntested {
		t.Errorf("expected %s (no CookStyle results), got %s", StatusUntested, status)
	}
	if source != SourceNone {
		t.Errorf("expected %s, got %s", SourceNone, source)
	}
}

func TestCheckCookbookCompatibility_CSPass_NoTK(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected %s, got %s", StatusCompatibleCookstyleOnly, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected %s, got %s", SourceCookstyle, source)
	}
}

func TestCheckCookbookCompatibility_CSFail_NoTK(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false)

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("expected %s, got %s", StatusIncompatible, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected %s, got %s", SourceCookstyle, source)
	}
}

func TestCheckCookbookCompatibility_CSPassNoTargetVersion(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	// CookStyle result with empty target version (server-sourced scan).
	ds.addCSResult("org-1", "apt", "7.4.0", "", true)

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected %s, got %s", StatusCompatibleCookstyleOnly, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected %s, got %s", SourceCookstyle, source)
	}
}

func TestCheckCookbookCompatibility_Untested(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	// No TK or CS results.

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusUntested {
		t.Errorf("expected %s, got %s", StatusUntested, status)
	}
	if source != SourceNone {
		t.Errorf("expected %s, got %s", SourceNone, source)
	}
}

func TestCheckCookbookCompatibility_CookbookNotInInventory(t *testing.T) {
	ds := newFakeReadinessDS()
	// Cookbook "unknown" not in the ID map.

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("unknown", "1.0.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusUntested {
		t.Errorf("expected %s, got %s", StatusUntested, status)
	}
	if source != SourceNone {
		t.Errorf("expected %s, got %s", SourceNone, source)
	}
}

func TestCheckCookbookCompatibility_CSFailWhenGitRepoExists(t *testing.T) {
	// Server CS fails with git repo present but no git CS result → incompatible.
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addGitRepo("apt", "abc123")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false) // server CS fails

	cache := ds.buildFakeCache()
	status, source, verdicts := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("expected %s (CS fails, no TK), got %s", StatusIncompatible, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected %s, got %s", SourceCookstyle, source)
	}
	if len(verdicts) < 1 {
		t.Errorf("expected at least 1 verdict, got %d", len(verdicts))
	}
}

// ---------------------------------------------------------------------------
// evaluateOne — integration tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Multi-source checkCookbookCompatibility tests
// ---------------------------------------------------------------------------

func TestCheckCookbookCompatibility_MultiSource_ServerIncompatibleGitCompatible(t *testing.T) {
	// Server CookStyle fails but git repo CookStyle passes → compatible
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false) // server CS fails
	ds.addGitRepo("apt", "abc123")
	ds.addGitCSResult("apt", "18.0", true) // git CS passes

	cache := ds.buildFakeCache()
	status, _, verdicts := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected compatible (git CS passes), got %s", status)
	}
	if len(verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(verdicts))
	}
}

func TestCheckCookbookCompatibility_MultiSource_AllIncompatible(t *testing.T) {
	// All sources incompatible
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false)
	ds.addGitRepo("apt", "abc123")
	ds.addGitCSResult("apt", "18.0", false)

	cache := ds.buildFakeCache()
	status, source, verdicts := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("expected incompatible, got %s", status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected primary source cookstyle, got %s", source)
	}
	if len(verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(verdicts))
	}
}

func TestCheckCookbookCompatibility_MultiSource_VerdictFields(t *testing.T) {
	// Verify verdict fields are populated correctly
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false)
	ds.addGitRepo("apt", "sha-abc")
	ds.addGitCSResult("apt", "18.0", true)

	cache := ds.buildFakeCache()
	_, _, verdicts := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)

	// Find git cookstyle verdict
	var gitV *CookbookSourceVerdict
	var serverV *CookbookSourceVerdict
	for i, v := range verdicts {
		if v.Source == SourceGitCookstyle {
			gitV = &verdicts[i]
		}
		if v.Source == SourceServerCookstyle {
			serverV = &verdicts[i]
		}
	}
	if gitV == nil {
		t.Fatal("missing git cookstyle verdict")
	}
	if gitV.Version != "HEAD" {
		t.Errorf("expected git version HEAD, got %s", gitV.Version)
	}
	if gitV.CommitSHA != "sha-abc" {
		t.Errorf("expected commit SHA sha-abc, got %s", gitV.CommitSHA)
	}
	if gitV.Status != StatusCompatible {
		t.Errorf("expected compatible, got %s", gitV.Status)
	}
	if serverV == nil {
		t.Fatal("missing server cookstyle verdict")
	}
	if serverV.Version != "7.4.0" {
		t.Errorf("expected server version 7.4.0, got %s", serverV.Version)
	}
}

func TestCheckCookbookCompatibility_MultiSource_CSFailOnly(t *testing.T) {
	// Server CS fails, no other sources → incompatible
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false) // server CS fails
	ds.addGitRepo("apt", "abc123")

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("expected incompatible (CS fails, no TK), got %s", status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected primary source cookstyle, got %s", source)
	}
}

func TestCheckCookbookCompatibility_MultiSource_NoGitRepo(t *testing.T) {
	// No git repo exists — only server CS checked
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	cache := ds.buildFakeCache()
	status, source, verdicts := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected compatible_cookstyle_only, got %s", status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected cookstyle source, got %s", source)
	}
	if len(verdicts) != 1 {
		t.Fatalf("expected 1 verdict, got %d", len(verdicts))
	}
	if verdicts[0].Source != SourceServerCookstyle {
		t.Errorf("expected server_cookstyle verdict, got %s", verdicts[0].Source)
	}
}

func TestCheckCookbookCompatibility_ErrorResultTreatedAsUntested(t *testing.T) {
	// A CookStyle result with ErrorMessage set (exit code >= 2) should be
	// skipped — not treated as compatible or incompatible. The cookbook
	// should appear as "untested".
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	// Server CS result with an error message (CookStyle crashed).
	ds.csResults[csKey("org-1", "apt", "7.4.0", "18.0")] = &datastore.ServerCookbookCookstyleResult{
		OrganisationName:  "org-1",
		CookbookName:      "apt",
		CookbookVersion:   "7.4.0",
		TargetChefVersion: "18.0",
		Passed:            false,
		ErrorMessage:      "CookStyle error (exit 2): Invalid .rubocop.yml",
	}

	cache := ds.buildFakeCache()
	status, source, verdicts := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusUntested {
		t.Errorf("expected %s (error result should be skipped), got %s", StatusUntested, status)
	}
	if source != SourceNone {
		t.Errorf("expected %s, got %s", SourceNone, source)
	}
	if len(verdicts) != 0 {
		t.Errorf("expected 0 verdicts (error result skipped), got %d", len(verdicts))
	}
}

func TestCheckCookbookCompatibility_GitCSErrorResultSkipped(t *testing.T) {
	// Git CookStyle result with ErrorMessage should also be skipped.
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addGitRepo("apt", "sha-abc")
	// Git CS result with error.
	ds.gitCSResults[gitCSKey("apt", "18.0")] = &datastore.GitRepoCookstyleResult{
		GitRepoName:       "apt",
		TargetChefVersion: "18.0",
		Passed:            false,
		ErrorMessage:      "CookStyle error (exit 2): bad config",
	}
	// No server CS result, no TK result.

	cache := ds.buildFakeCache()
	status, _, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusUntested {
		t.Errorf("expected %s (git CS error result skipped), got %s", StatusUntested, status)
	}
}

func TestCheckCookbookCompatibility_ErrorResultDoesNotOverrideGoodResult(t *testing.T) {
	// Server CS errored, but git CS passed → should be compatible.
	// The error result is skipped, the good result counts.
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	// Server CS errored.
	ds.csResults[csKey("org-1", "apt", "7.4.0", "18.0")] = &datastore.ServerCookbookCookstyleResult{
		OrganisationName:  "org-1",
		CookbookName:      "apt",
		CookbookVersion:   "7.4.0",
		TargetChefVersion: "18.0",
		Passed:            false,
		ErrorMessage:      "CookStyle error (exit 2): crash",
	}
	// Git CS passed.
	ds.addGitRepo("apt", "sha-abc")
	ds.addGitCSResult("apt", "18.0", true)

	cache := ds.buildFakeCache()
	status, _, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected %s (git CS passed despite server CS error), got %s", StatusCompatibleCookstyleOnly, status)
	}
}

// ---------------------------------------------------------------------------
// checkCookbookCompatibility — TK integration tests
// ---------------------------------------------------------------------------

func TestCheckCookbookCompatibility_CSPass_TKPass(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addGitRepoWithTK("apt", "sha-abc", true, false)
	ds.addGitTKStatus("apt", "18.0", "passed")

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatible {
		t.Errorf("expected %s, got %s", StatusCompatible, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected source %s, got %s", SourceCookstyle, source)
	}
}

func TestCheckCookbookCompatibility_CSPass_TKFail(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addGitRepoWithTK("apt", "sha-abc", true, false)
	ds.addGitTKStatus("apt", "18.0", "failed")

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("expected %s, got %s", StatusIncompatible, status)
	}
	if source != SourceGitTestKitchen {
		t.Errorf("expected source %s, got %s", SourceGitTestKitchen, source)
	}
}

func TestCheckCookbookCompatibility_CSPass_TKPartial(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addGitRepoWithTK("apt", "sha-abc", true, false)
	ds.addGitTKStatus("apt", "18.0", "partial")

	cache := ds.buildFakeCache()
	status, _, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("expected %s, got %s", StatusIncompatible, status)
	}
}

func TestCheckCookbookCompatibility_CSFail_TKPass(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false)
	ds.addGitRepoWithTK("apt", "sha-abc", true, false)
	ds.addGitTKStatus("apt", "18.0", "passed")

	cache := ds.buildFakeCache()
	status, source, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("expected %s, got %s", StatusIncompatible, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected source %s, got %s", SourceCookstyle, source)
	}
}

func TestCheckCookbookCompatibility_CSFail_TKFail(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false)
	ds.addGitRepoWithTK("apt", "sha-abc", true, false)
	ds.addGitTKStatus("apt", "18.0", "failed")

	cache := ds.buildFakeCache()
	status, _, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusIncompatible {
		t.Errorf("expected %s, got %s", StatusIncompatible, status)
	}
}

func TestCheckCookbookCompatibility_CSPass_NoTKResults(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addGitRepoWithTK("apt", "sha-abc", true, false)
	// No TK status in cache

	cache := ds.buildFakeCache()
	status, _, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected %s, got %s", StatusCompatibleCookstyleOnly, status)
	}
}

func TestCheckCookbookCompatibility_CSPass_TKExcluded(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addGitRepoWithTK("apt", "sha-abc", true, true) // KitchenExcluded = true
	ds.addGitTKStatus("apt", "18.0", "failed")         // TK failed, but excluded

	cache := ds.buildFakeCache()
	status, _, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected %s (TK excluded), got %s", StatusCompatibleCookstyleOnly, status)
	}
}

func TestCheckCookbookCompatibility_CSPass_NoTestSuite(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addGitRepoWithTK("apt", "sha-abc", false, false) // HasTestSuite = false
	ds.addGitTKStatus("apt", "18.0", "failed")

	cache := ds.buildFakeCache()
	status, _, _ := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected %s (no test suite), got %s", StatusCompatibleCookstyleOnly, status)
	}
}

func TestCheckCookbookCompatibility_TKPass_Verdicts(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addGitRepoWithTK("apt", "sha-abc", true, false)
	ds.addGitTKStatus("apt", "18.0", "passed")

	cache := ds.buildFakeCache()
	_, _, verdicts := checkCookbookCompatibility("apt", "7.4.0", "18.0", ds.cookbookIDs, cache)

	foundTK := false
	for _, v := range verdicts {
		if v.Source == SourceGitTestKitchen {
			foundTK = true
			if v.Status != StatusCompatible {
				t.Errorf("expected TK verdict status %s, got %s", StatusCompatible, v.Status)
			}
			if v.Version != "HEAD" {
				t.Errorf("expected TK verdict version HEAD, got %s", v.Version)
			}
		}
	}
	if !foundTK {
		t.Error("expected a git_test_kitchen verdict in results")
	}
}

// ---------------------------------------------------------------------------
// evaluateOne — integration tests
// ---------------------------------------------------------------------------

func TestEvaluateOne_AllCompatibleSufficientDisk(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCookbookID("nginx", "2.0.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addCSResult("org-1", "nginx", "2.0.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if !result.IsReady {
		t.Error("expected node to be ready")
	}
	if !result.AllCookbooksCompatible {
		t.Error("expected all cookbooks compatible")
	}
	if result.SufficientDiskSpace == nil || !*result.SufficientDiskSpace {
		t.Error("expected sufficient disk space")
	}
	if len(result.BlockingCookbooks) != 0 {
		t.Errorf("expected 0 blocking, got %d", len(result.BlockingCookbooks))
	}
}

func TestEvaluateOne_IncompatibleCookbook(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCookbookID("nginx", "2.0.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addCSResult("org-1", "nginx", "2.0.0", "18.0", false) // FAIL

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if result.IsReady {
		t.Error("expected node NOT ready")
	}
	if result.AllCookbooksCompatible {
		t.Error("expected NOT all cookbooks compatible")
	}
	if len(result.BlockingCookbooks) != 1 {
		t.Fatalf("expected 1 blocking cookbook, got %d", len(result.BlockingCookbooks))
	}
	bc := result.BlockingCookbooks[0]
	if bc.Name != "nginx" {
		t.Errorf("expected blocking cookbook nginx, got %s", bc.Name)
	}
	if bc.Reason != StatusIncompatible {
		t.Errorf("expected reason %s, got %s", StatusIncompatible, bc.Reason)
	}
	if bc.Source != SourceCookstyle {
		t.Errorf("expected source %s, got %s", SourceCookstyle, bc.Source)
	}
	// Verify verdicts are populated.
	if len(bc.Verdicts) == 0 {
		t.Error("expected verdicts to be populated on blocking cookbook")
	}
}

func TestEvaluateOne_UntestedCookbook(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	// "nginx" is in the ID map but has no test results.
	ds.addCookbookID("nginx", "2.0.0", "org-1")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if result.IsReady {
		t.Error("expected NOT ready (untested cookbook)")
	}
	if len(result.BlockingCookbooks) != 1 {
		t.Fatalf("expected 1 blocking, got %d", len(result.BlockingCookbooks))
	}
	if result.BlockingCookbooks[0].Reason != StatusUntested {
		t.Errorf("expected reason %s, got %s", StatusUntested, result.BlockingCookbooks[0].Reason)
	}
}

func TestEvaluateOne_InsufficientDisk(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "2097152", KBUsed: "1048576", KBAvailable: "1048576", PercentUsed: "50%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if result.IsReady {
		t.Error("expected NOT ready (insufficient disk)")
	}
	if !result.AllCookbooksCompatible {
		t.Error("expected all cookbooks compatible")
	}
	if result.SufficientDiskSpace == nil {
		t.Fatal("expected disk space known")
	}
	if *result.SufficientDiskSpace {
		t.Error("expected insufficient disk space")
	}
}

func TestEvaluateOne_UnknownDiskSpace(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		nil) // no filesystem data

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if result.IsReady {
		t.Error("expected NOT ready (unknown disk space)")
	}
	if !result.AllCookbooksCompatible {
		t.Error("expected all cookbooks compatible")
	}
	if result.SufficientDiskSpace != nil {
		t.Error("expected disk space unknown (nil)")
	}
	if result.AvailableDiskMB != nil {
		t.Error("expected available disk unknown (nil)")
	}
}

func TestEvaluateOne_StaleNode(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "stale-node", true,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if !result.StaleData {
		t.Error("expected stale_data = true")
	}
	// Stale nodes: disk space treated as unknown.
	if result.SufficientDiskSpace != nil {
		t.Error("expected disk space unknown for stale node")
	}
	if result.AvailableDiskMB != nil {
		t.Error("expected available disk unknown for stale node")
	}
	// Even with all cookbooks compatible, unknown disk space blocks readiness.
	if result.IsReady {
		t.Error("expected NOT ready (stale node → unknown disk)")
	}
}

func TestEvaluateOne_NoCookbooks(t *testing.T) {
	ds := newFakeReadinessDS()

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "bare-node", false,
		nil, // no cookbooks
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if !result.AllCookbooksCompatible {
		t.Error("expected all_cookbooks_compatible = true for node with no cookbooks")
	}
	if len(result.BlockingCookbooks) != 0 {
		t.Errorf("expected 0 blocking, got %d", len(result.BlockingCookbooks))
	}
	if !result.IsReady {
		t.Error("expected ready (no cookbooks + sufficient disk)")
	}
}

func TestEvaluateOne_ComplexityEnrichment(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("nginx", "2.0.0", "org-1")
	ds.addCSResult("org-1", "nginx", "2.0.0", "18.0", false) // incompatible
	ds.addComplexity("org-1", "nginx", "2.0.0", "18.0", 45, "high")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"nginx": "2.0.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if len(result.BlockingCookbooks) != 1 {
		t.Fatalf("expected 1 blocking, got %d", len(result.BlockingCookbooks))
	}
	bc := result.BlockingCookbooks[0]
	if bc.ComplexityScore != 45 {
		t.Errorf("expected complexity score 45, got %d", bc.ComplexityScore)
	}
	if bc.ComplexityLabel != "high" {
		t.Errorf("expected complexity label 'high', got %q", bc.ComplexityLabel)
	}
}

func TestEvaluateOne_MultipleBlockingCookbooks(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCookbookID("nginx", "2.0.0", "org-1")
	ds.addCookbookID("java", "8.5.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true) // pass
	// nginx and java: no results → untested

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0", "java": "8.5.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if result.IsReady {
		t.Error("expected NOT ready")
	}
	if len(result.BlockingCookbooks) != 2 {
		t.Fatalf("expected 2 blocking, got %d", len(result.BlockingCookbooks))
	}
	// Check both are untested.
	for _, bc := range result.BlockingCookbooks {
		if bc.Reason != StatusUntested {
			t.Errorf("expected untested, got %s for %s", bc.Reason, bc.Name)
		}
	}
}

func TestEvaluateOne_CookstyleOnlyPassIsNotBlocking(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true) // CookStyle pass, no TK

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if !result.AllCookbooksCompatible {
		t.Error("expected cookstyle-only pass to not block")
	}
	if !result.IsReady {
		t.Error("expected ready (cookstyle-only compatible + sufficient disk)")
	}
}

// ---------------------------------------------------------------------------
// EvaluateOrganisation — batch tests
// ---------------------------------------------------------------------------

func TestEvaluateOrganisation_Basic(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false,
			cookbooksJSON(map[string]string{"apt": "7.4.0"}),
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
		makeSnapshot("org-1", "node-2", false,
			cookbooksJSON(map[string]string{"apt": "7.4.0"}),
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
	}
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 4, 2048)
	results, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.IsReady {
			t.Errorf("expected node %s to be ready", r.NodeName)
		}
	}
	// Check upserts
	if int(ds.upsertCount.Load()) != 2 {
		t.Errorf("expected 2 upserts, got %d", ds.upsertCount.Load())
	}
}

func TestEvaluateOrganisation_MultipleTargetVersions(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false,
			cookbooksJSON(map[string]string{"apt": "7.4.0"}),
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
	}
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addCSResult("org-1", "apt", "7.4.0", "17.0", false) // fails for 17.0

	e := NewReadinessEvaluator(ds, nil, 4, 2048)
	results, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"17.0", "18.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results (1 node × 2 versions), got %d", len(results))
	}

	readyCount := 0
	for _, r := range results {
		if r.IsReady {
			readyCount++
		}
	}
	// 18.0 has passing CS result, 17.0 has failing CS result.
	if readyCount != 1 {
		t.Errorf("expected 1 ready result (18.0), got %d", readyCount)
	}
}

func TestEvaluateOrganisation_NoSnapshots(t *testing.T) {
	ds := newFakeReadinessDS()
	// No snapshots.

	e := NewReadinessEvaluator(ds, nil, 4, 2048)
	results, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %d", len(results))
	}
}

func TestEvaluateOrganisation_NoTargetVersions(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil, nil),
	}

	e := NewReadinessEvaluator(ds, nil, 4, 2048)
	results, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %d", len(results))
	}
}

func TestEvaluateOrganisation_ListSnapshotsError(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.listSnapshotsErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 4, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "listing node snapshots") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_CookbookIDMapError(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil, nil),
	}
	ds.listServerCookbooksErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 4, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "listing server cookbooks") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_UpsertErrorDoesNotAbortBatch(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil,
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
		makeSnapshot("org-1", "node-2", false, nil,
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
	}
	ds.upsertErr = fmt.Errorf("disk full")

	e := NewReadinessEvaluator(ds, nil, 4, 2048)
	results, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("batch should not fail: %v", err)
	}
	// Results are still collected even though persistence failed.
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestEvaluateOrganisation_ContextCancellation(t *testing.T) {
	ds := newFakeReadinessDS()
	// Create many snapshots.
	for i := 0; i < 50; i++ {
		ds.snapshots = append(ds.snapshots,
			makeSnapshot("org-1", fmt.Sprintf("node-%d", i), false, nil,
				linuxFilesystemJSON(map[string]linuxMount{
					"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
				})))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	e := NewReadinessEvaluator(ds, nil, 1, 2048) // concurrency=1 to make cancellation more observable
	results, err := e.EvaluateOrganisation(ctx, "org-1", "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("unexpected error (context cancellation is not a batch error): %v", err)
	}
	// With immediate cancellation and concurrency=1, we should get fewer results than 50.
	// But due to goroutine scheduling, we can't assert an exact count.
	if len(results) >= 50 {
		t.Logf("got %d results despite cancellation (goroutine scheduling may allow all to complete)", len(results))
	}
}

func TestEvaluateOrganisation_ConcurrencyBounded(t *testing.T) {
	ds := newFakeReadinessDS()
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("node-%d", i)
		ds.snapshots = append(ds.snapshots,
			makeSnapshot("org-1", name, false,
				cookbooksJSON(map[string]string{"apt": "7.4.0"}),
				linuxFilesystemJSON(map[string]linuxMount{
					"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
				})))
	}
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 3, 2048) // concurrency=3
	results, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 20 {
		t.Errorf("expected 20 results, got %d", len(results))
	}
	// All should be ready.
	for _, r := range results {
		if !r.IsReady {
			t.Errorf("expected node %s to be ready", r.NodeName)
		}
	}
}

// ---------------------------------------------------------------------------
// buildReadinessCache tests
// ---------------------------------------------------------------------------

func TestBuildReadinessCache_PopulatesMaps(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addGitRepo("apt", "sha-abc")
	ds.addGitCSResult("apt", "18.0", true)
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", false)
	ds.addComplexity("org-1", "apt", "7.4.0", "18.0", 42, "medium")
	ds.gitComplexities[gcKey("apt", "18.0")] = &datastore.GitRepoComplexity{
		GitRepoName:       "apt",
		TargetChefVersion: "18.0",
		ComplexityScore:   10,
		ComplexityLabel:   "low",
	}

	cache, err := buildReadinessCache(context.Background(), ds, "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Git repos
	if gr, ok := cache.gitRepos["apt"]; !ok {
		t.Error("expected git repo 'apt' in cache")
	} else if gr.HeadCommitSHA != "sha-abc" {
		t.Errorf("expected HeadCommitSHA 'sha-abc', got %q", gr.HeadCommitSHA)
	}

	// Git CS results
	if gcs := cache.gitCSResults[cacheKey("apt", "18.0")]; gcs == nil {
		t.Error("expected git CS result in cache")
	} else if !gcs.Passed {
		t.Error("expected git CS result to have passed")
	}

	// Server CS results
	if scs := cache.serverCSResults[cacheKey("org-1/apt/7.4.0", "18.0")]; scs == nil {
		t.Error("expected server CS result in cache")
	} else if scs.Passed {
		t.Error("expected server CS result to have failed")
	}

	// Server complexity
	if sc := cache.serverComplexity[cacheKey("org-1/apt/7.4.0", "18.0")]; sc == nil {
		t.Error("expected server complexity in cache")
	} else if sc.ComplexityScore != 42 {
		t.Errorf("expected complexity score 42, got %d", sc.ComplexityScore)
	}

	// Git complexity
	if gc := cache.gitComplexity[cacheKey("apt", "18.0")]; gc == nil {
		t.Error("expected git complexity in cache")
	} else if gc.ComplexityScore != 10 {
		t.Errorf("expected complexity score 10, got %d", gc.ComplexityScore)
	}

	// Git TK statuses (none added — should be empty but present)
	if cache.gitTKStatuses == nil {
		t.Error("expected gitTKStatuses map to be initialised")
	}
}

func TestBuildReadinessCache_FiltersTargetVersions(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)
	ds.addCSResult("org-1", "apt", "7.4.0", "17.0", false)

	// Build cache for only 18.0
	cache, err := buildReadinessCache(context.Background(), ds, "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scs := cache.serverCSResults[cacheKey("org-1/apt/7.4.0", "18.0")]; scs == nil {
		t.Error("expected server CS result for 18.0")
	}
	if scs := cache.serverCSResults[cacheKey("org-1/apt/7.4.0", "17.0")]; scs != nil {
		t.Error("did not expect server CS result for 17.0 (not in target versions)")
	}
}

func TestBuildReadinessCache_IncludesNullTargetVersionCSResults(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	// Server CS result with empty target version (scanned without profile)
	ds.addCSResult("org-1", "apt", "7.4.0", "", true)

	cache, err := buildReadinessCache(context.Background(), ds, "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The empty-target-version result should be in the cache
	if scs := cache.serverCSResults[cacheKey("org-1/apt/7.4.0", "")]; scs == nil {
		t.Error("expected server CS result with empty target version in cache")
	}
}

func TestEvaluateOrganisation_BulkLoadError_GitRepos(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil, nil),
	}
	ds.gitRepoErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error from bulk-load failure")
	}
	if !contains(err.Error(), "bulk-loading git repos") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_BulkLoadError_ServerCSResults(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil, nil),
	}
	ds.csErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error from bulk-load failure")
	}
	if !contains(err.Error(), "bulk-loading server CookStyle results") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_BulkLoadError_GitCSResults(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil, nil),
	}
	ds.gitCSErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error from bulk-load failure")
	}
	if !contains(err.Error(), "bulk-loading git CookStyle results") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_BulkLoadError_ServerComplexities(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil, nil),
	}
	ds.complexityErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error from bulk-load failure")
	}
	if !contains(err.Error(), "bulk-loading server complexities") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_BulkLoadError_GitComplexities(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil, nil),
	}
	ds.gitComplexityErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error from bulk-load failure")
	}
	if !contains(err.Error(), "bulk-loading git complexities") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_BulkLoadError_GitTK(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false, nil, nil),
	}
	ds.gitTKErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error from bulk-load failure")
	}
	if !contains(err.Error(), "bulk-loading git TK counts") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_EmptyCache(t *testing.T) {
	// No CookStyle/complexity data at all — all cookbooks should show as untested
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("org-1", "node-1", false,
			cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0"}),
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
	}
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCookbookID("nginx", "2.0.0", "org-1")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	results, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.IsReady {
		t.Error("node should not be ready (all cookbooks untested)")
	}
	if r.AllCookbooksCompatible {
		t.Error("expected AllCookbooksCompatible=false (untested)")
	}
	if len(r.BlockingCookbooks) != 2 {
		t.Errorf("expected 2 blocking cookbooks, got %d", len(r.BlockingCookbooks))
	}
	for _, bc := range r.BlockingCookbooks {
		if bc.Reason != StatusUntested {
			t.Errorf("expected blocking reason %q, got %q for %s", StatusUntested, bc.Reason, bc.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// NewReadinessEvaluator option tests
// ---------------------------------------------------------------------------

func TestNewReadinessEvaluator_Defaults(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 0, 0)
	if e.concurrency != 1 {
		t.Errorf("expected concurrency 1, got %d", e.concurrency)
	}
	if e.installSizeMBLinux != 2048 {
		t.Errorf("expected installSizeMBLinux 2048, got %d", e.installSizeMBLinux)
	}
}

func TestNewReadinessEvaluator_NegativeValues(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, -5, -100)
	if e.concurrency != 1 {
		t.Errorf("expected concurrency 1, got %d", e.concurrency)
	}
	if e.installSizeMBLinux != 2048 {
		t.Errorf("expected installSizeMBLinux 2048, got %d", e.installSizeMBLinux)
	}
}

func TestNewReadinessEvaluator_CustomValues(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 10, 4096)
	if e.concurrency != 10 {
		t.Errorf("expected concurrency 10, got %d", e.concurrency)
	}
	if e.installSizeMBLinux != 4096 {
		t.Errorf("expected installSizeMBLinux 4096, got %d", e.installSizeMBLinux)
	}
}

func TestNewReadinessEvaluator_WithDataStoreOption(t *testing.T) {
	ds1 := newFakeReadinessDS()
	ds2 := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds1, nil, 1, 2048, WithReadinessDataStore(ds2))
	// ds should be ds2 due to the option.
	if e.db != ds2 {
		t.Error("expected WithReadinessDataStore to override the datastore")
	}
}

// ---------------------------------------------------------------------------
// persistResult tests
// ---------------------------------------------------------------------------

func TestPersistResult_Success(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	sufficient := true
	availMB := 5000
	result := ReadinessResult{
		OrganisationName:       "org-1",
		NodeName:               "node-1",
		TargetChefVersion:      "18.0",
		IsReady:                false,
		AllCookbooksCompatible: false,
		SufficientDiskSpace:    &sufficient,
		BlockingCookbooks: []BlockingCookbook{
			{Name: "nginx", Version: "2.0.0", Reason: StatusIncompatible, Source: SourceCookstyle, ComplexityScore: 30, ComplexityLabel: "high"},
		},
		AvailableDiskMB: &availMB,
		RequiredDiskMB:  2048,
		StaleData:       false,
		EvaluatedAt:     time.Now().UTC(),
	}

	err := e.persistResult(context.Background(), result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ds.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(ds.upserted))
	}
	p := ds.upserted[0]
	if p.NodeName != "node-1" {
		t.Errorf("expected node_name node-1, got %s", p.NodeName)
	}
	if p.IsReady {
		t.Error("expected is_ready false")
	}
	if p.BlockingCookbooks == nil {
		t.Fatal("expected blocking_cookbooks JSON")
	}
	// Verify the blocking cookbooks JSON is valid.
	var bcs []BlockingCookbook
	if err := json.Unmarshal(p.BlockingCookbooks, &bcs); err != nil {
		t.Fatalf("invalid blocking_cookbooks JSON: %v", err)
	}
	if len(bcs) != 1 {
		t.Fatalf("expected 1 blocking, got %d", len(bcs))
	}
	if bcs[0].Name != "nginx" {
		t.Errorf("expected nginx, got %s", bcs[0].Name)
	}
}

func TestPersistResult_NoBlockingCookbooks(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	result := ReadinessResult{
		OrganisationName:       "org-1",
		NodeName:               "node-1",
		TargetChefVersion:      "18.0",
		IsReady:                true,
		AllCookbooksCompatible: true,
		EvaluatedAt:            time.Now().UTC(),
	}

	err := e.persistResult(context.Background(), result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ds.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(ds.upserted))
	}
	p := ds.upserted[0]
	if p.BlockingCookbooks != nil {
		t.Errorf("expected nil blocking_cookbooks, got %s", string(p.BlockingCookbooks))
	}
}

func TestPersistResult_UpsertError(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.upsertErr = fmt.Errorf("connection lost")
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	result := ReadinessResult{
		OrganisationName:  "org-1",
		NodeName:          "node-1",
		TargetChefVersion: "18.0",
		EvaluatedAt:       time.Now().UTC(),
	}

	err := e.persistResult(context.Background(), result)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// BlockingCookbook JSON serialisation
// ---------------------------------------------------------------------------

func TestBlockingCookbook_JSON(t *testing.T) {
	bc := BlockingCookbook{
		Name:            "nginx",
		Version:         "2.0.0",
		Reason:          StatusIncompatible,
		Source:          SourceCookstyle,
		ComplexityScore: 45,
		ComplexityLabel: "high",
		Verdicts: []CookbookSourceVerdict{
			{
				Source:    SourceGitCookstyle,
				Status:    StatusIncompatible,
				Version:   "HEAD",
				CommitSHA: "abc123",
			},
			{
				Source:  SourceServerCookstyle,
				Status:  StatusIncompatible,
				Version: "2.0.0",
			},
		},
	}
	b, err := json.Marshal(bc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded BlockingCookbook
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.Name != "nginx" || decoded.Version != "2.0.0" {
		t.Errorf("unexpected decoded values: %+v", decoded)
	}
	if decoded.ComplexityScore != 45 || decoded.ComplexityLabel != "high" {
		t.Errorf("unexpected complexity: %+v", decoded)
	}
	if len(decoded.Verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(decoded.Verdicts))
	}
	if decoded.Verdicts[0].Source != SourceGitCookstyle {
		t.Errorf("expected first verdict source %s, got %s", SourceGitCookstyle, decoded.Verdicts[0].Source)
	}
	if decoded.Verdicts[0].CommitSHA != "abc123" {
		t.Errorf("expected commit SHA abc123, got %s", decoded.Verdicts[0].CommitSHA)
	}
	if decoded.Verdicts[1].Source != SourceServerCookstyle {
		t.Errorf("expected second verdict source %s, got %s", SourceServerCookstyle, decoded.Verdicts[1].Source)
	}
}

// ---------------------------------------------------------------------------
// Status and source constant tests
// ---------------------------------------------------------------------------

func TestStatusConstants(t *testing.T) {
	statuses := []string{StatusCompatible, StatusCompatibleCookstyleOnly, StatusIncompatible, StatusUntested}
	seen := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		if s == "" {
			t.Error("status constant should not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate status constant: %s", s)
		}
		seen[s] = true
	}
}

func TestSourceConstants(t *testing.T) {
	sources := []string{SourceTestKitchen, SourceCookstyle, SourceNone}
	seen := make(map[string]bool, len(sources))
	for _, s := range sources {
		if s == "" {
			t.Error("source constant should not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate source constant: %s", s)
		}
		seen[s] = true
	}
}

func TestMultiSourceConstants(t *testing.T) {
	sources := []string{SourceServerCookstyle, SourceGitCookstyle, SourceGitTestKitchen}
	seen := make(map[string]bool, len(sources))
	for _, s := range sources {
		if s == "" {
			t.Error("multi-source constant should not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate multi-source constant: %s", s)
		}
		seen[s] = true
	}
	// Ensure no overlap with legacy source constants.
	legacy := map[string]bool{
		SourceTestKitchen: true,
		SourceCookstyle:   true,
		SourceNone:        true,
	}
	for _, s := range sources {
		if legacy[s] {
			t.Errorf("multi-source constant %q collides with legacy source constant", s)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge case: disk space with /hab as a sub-mount under /opt
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// evaluateDiskSpace — Ohai 14+ format tests
// ---------------------------------------------------------------------------

func TestEvaluateDiskSpace_Ohai14_LinuxSufficientSpace(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	snap := makeSnapshot("org-1", "node-1", false, nil,
		ohai14LinuxFilesystemJSON(map[string]ohaiPairEntry{
			"/dev/vda2,/": {
				Mount: "/", Device: "/dev/vda2", FSType: "ext4",
				KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%",
			},
			"tmpfs,/run": {
				Mount: "/run", Device: "tmpfs", FSType: "tmpfs",
				KBSize: "3206072", KBUsed: "75484", KBAvailable: "3130588", PercentUsed: "3%",
			},
		}))

	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected disk space to be known for Ohai 14+ format")
	}
	// 14340800 KB / 1024 = 14004 MB
	if availMB != 14004 {
		t.Errorf("expected 14004 MB available, got %d", availMB)
	}
}

func TestEvaluateDiskSpace_Ohai14_LinuxInsufficientSpace(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	snap := makeSnapshot("org-1", "node-1", false, nil,
		ohai14LinuxFilesystemJSON(map[string]ohaiPairEntry{
			"/dev/vda2,/": {
				Mount: "/", Device: "/dev/vda2", FSType: "ext4",
				KBSize: "2097152", KBUsed: "1048576", KBAvailable: "1048576", PercentUsed: "50%",
			},
		}))

	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected disk space to be known")
	}
	// 1048576 KB / 1024 = 1024 MB — below the 2048 MB threshold.
	if availMB != 1024 {
		t.Errorf("expected 1024 MB, got %d", availMB)
	}
}

func TestEvaluateDiskSpace_Ohai14_LinuxDedicatedHabMount(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	snap := makeSnapshot("org-1", "node-1", false, nil,
		ohai14LinuxFilesystemJSON(map[string]ohaiPairEntry{
			"/dev/vda2,/": {
				Mount: "/", Device: "/dev/vda2", FSType: "ext4",
				KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%",
			},
			"/dev/vdb1,/hab": {
				Mount: "/hab", Device: "/dev/vdb1", FSType: "ext4",
				KBSize: "10000000", KBUsed: "1000000", KBAvailable: "8000000", PercentUsed: "10%",
			},
		}))

	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected disk space to be known")
	}
	// Should prefer /hab mount (longest prefix of /hab).
	// 8000000 KB / 1024 = 7812 MB
	if availMB != 7812 {
		t.Errorf("expected 7812 MB (from /hab mount), got %d", availMB)
	}
}

func TestEvaluateDiskSpace_Ohai14_WindowsDrive(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	snap := makeSnapshot("org-1", "win-node", false, nil,
		ohai14WindowsFilesystemJSON(map[string]ohaiPairEntry{
			",C:": {
				Mount: "C:", Device: "", FSType: "ntfs",
				KBSize: 41949327, KBUsed: 41488511, KBAvailable: 460816, PercentUsed: 98,
			},
		}))
	snap.Platform = "windows"

	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected disk space to be known for Ohai 14+ Windows format")
	}
	// 460816 KB / 1024 = 450 MB (integer truncation of 450.015625)
	if availMB != 450 {
		t.Errorf("expected 450 MB available, got %d", availMB)
	}
}

func TestEvaluateDiskSpace_Ohai14_IntegerValues(t *testing.T) {
	// Ohai may return integer values instead of strings for kb_* fields.
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	snap := makeSnapshot("org-1", "node-1", false, nil,
		ohai14LinuxFilesystemJSON(map[string]ohaiPairEntry{
			"/dev/vda2,/": {
				Mount: "/", Device: "/dev/vda2", FSType: "ext4",
				KBSize: 20511356, KBUsed: 5123456, KBAvailable: 14340800, PercentUsed: 26,
			},
		}))

	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected disk space to be known with integer values")
	}
	if availMB != 14004 {
		t.Errorf("expected 14004 MB available, got %d", availMB)
	}
}

func TestEvaluateOne_Ohai14_ReadyWithSufficientDisk(t *testing.T) {
	// End-to-end: all cookbooks compatible, Ohai 14+ filesystem, sufficient disk.
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		ohai14LinuxFilesystemJSON(map[string]ohaiPairEntry{
			"/dev/vda2,/": {
				Mount: "/", Device: "/dev/vda2", FSType: "ext4",
				KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%",
			},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if !result.IsReady {
		t.Error("expected node to be ready with Ohai 14+ filesystem")
	}
	if result.SufficientDiskSpace == nil || !*result.SufficientDiskSpace {
		t.Error("expected sufficient disk space")
	}
	if result.AvailableDiskMB == nil || *result.AvailableDiskMB != 14004 {
		t.Errorf("expected 14004 MB, got %v", result.AvailableDiskMB)
	}
}

func TestEvaluateOne_Ohai14_BlockedByDisk(t *testing.T) {
	// End-to-end: all cookbooks compatible, Ohai 14+ filesystem, insufficient disk.
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "org-1")
	ds.addCSResult("org-1", "apt", "7.4.0", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		ohai14LinuxFilesystemJSON(map[string]ohaiPairEntry{
			"/dev/vda2,/": {
				Mount: "/", Device: "/dev/vda2", FSType: "ext4",
				KBSize: "2097152", KBUsed: "1048576", KBAvailable: "1048576", PercentUsed: "50%",
			},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if result.IsReady {
		t.Error("expected node NOT ready (insufficient disk)")
	}
	if !result.AllCookbooksCompatible {
		t.Error("expected all cookbooks compatible")
	}
	if result.SufficientDiskSpace == nil {
		t.Fatal("expected disk space known")
	}
	if *result.SufficientDiskSpace {
		t.Error("expected insufficient disk space")
	}
}

func TestEvaluateDiskSpace_HabUnderOpt(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	// /opt is mounted, but /hab is not under /opt — root should match.
	snap := makeSnapshot("org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			"/dev/sdb1": {KBSize: "10000000", KBUsed: "1000000", KBAvailable: "8000000", PercentUsed: "10%", Mount: "/opt"},
		}))

	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known")
	}
	// /opt is not a prefix of /hab, so root should be used.
	expected := 14340800 / 1024
	if availMB != expected {
		t.Errorf("expected %d MB (root), got %d MB", expected, availMB)
	}
}

func TestEvaluateDiskSpace_DedicatedHabOverridesRoot(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			"/dev/sdb1": {KBSize: "5000000", KBUsed: "1000000", KBAvailable: "3000000", PercentUsed: "20%", Mount: "/hab"},
		}))

	availMB, _, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known")
	}
	// /hab mount should be preferred over root.
	expected := 3000000 / 1024
	if availMB != expected {
		t.Errorf("expected %d MB (dedicated /hab), got %d MB", expected, availMB)
	}
}

// ---------------------------------------------------------------------------
// Edge case: DiskSpace evaluation exact boundary
// ---------------------------------------------------------------------------

func TestEvaluateOne_ExactDiskSpaceBoundary(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	// 2048 MB free on a 10240 MB filesystem (20 GB).
	// After install: remaining = 0 MB. Remaining% = 0/10240 = 0% — fails 20% threshold.
	// For the absolute check to pass AND the percentage check:
	// Need 2048 + 20% of total. With 10240 total, need 2048 + 2048 = 4096 free.
	// Use exactly 4096 MB free: absolute OK (4096 >= 2048), remaining = 2048/10240 = 20% — passes.
	snap := makeSnapshot("org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "10485760", KBUsed: "6291456", KBAvailable: "4194304", PercentUsed: "60%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if result.SufficientDiskSpace == nil {
		t.Fatal("expected disk space known")
	}
	if !*result.SufficientDiskSpace {
		t.Error("expected sufficient disk space (absolute OK and remaining% = 20%)")
	}
}

func TestEvaluateOne_OneBelowDiskSpaceBoundary(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	// 2047 MB free = 2047 * 1024 = 2096128 KB — fails absolute threshold (2047 < 2048).
	snap := makeSnapshot("org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "10485760", KBUsed: "8389632", KBAvailable: "2096128", PercentUsed: "80%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)

	if result.SufficientDiskSpace == nil {
		t.Fatal("expected disk space known")
	}
	if *result.SufficientDiskSpace {
		t.Error("expected insufficient disk space (2047 < 2048)")
	}
}

// ---------------------------------------------------------------------------
// ReadinessResult field tests
// ---------------------------------------------------------------------------

func TestReadinessResult_RequiredDiskMBDefaultsToMinFreeDisk(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 4096)

	snap := makeSnapshot("org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)
	if result.RequiredDiskMB != 4096 {
		t.Errorf("expected requiredDiskMB 4096, got %d", result.RequiredDiskMB)
	}
}

func TestReadinessResult_EvaluatedAtSet(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	before := time.Now().UTC().Add(-time.Second)
	snap := makeSnapshot("org-1", "node-1", false, nil, nil)
	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)
	after := time.Now().UTC().Add(time.Second)

	if result.EvaluatedAt.Before(before) || result.EvaluatedAt.After(after) {
		t.Errorf("evaluatedAt %v not in range [%v, %v]", result.EvaluatedAt, before, after)
	}
}

func TestNewReadinessEvaluatorFromConfig_DualThreshold(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluatorFromConfig(ds, nil, 1, ReadinessEvalConfig{
		InstallPathLinux:        "/hab",
		InstallPathWindows:      `C:\hab`,
		InstallSizeMBLinux:      3072,
		InstallSizeMBWindows:    6144,
		MinRemainingFreePercent: 20,
	})

	// 10 GB total, 5 GB free. After 3 GB install: 2 GB remaining = 20% of 10 GB. Passes.
	snap := makeSnapshot("org-1", "node-pass", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "10485760", KBUsed: "5242880", KBAvailable: "5242880", PercentUsed: "50%", Mount: "/"},
		}))
	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)
	if result.SufficientDiskSpace == nil || !*result.SufficientDiskSpace {
		t.Error("expected sufficient: absolute OK (5120 >= 3072) and remaining% = 20%")
	}

	// 10 GB total, 4 GB free. After 3 GB install: 1 GB remaining = 10% < 20%. Fails pct.
	snap2 := makeSnapshot("org-1", "node-fail-pct", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "10485760", KBUsed: "6291456", KBAvailable: "4194304", PercentUsed: "60%", Mount: "/"},
		}))
	result2 := e.evaluateOne(snap2, "18.0", ds.cookbookIDs, cache)
	if result2.SufficientDiskSpace == nil || *result2.SufficientDiskSpace {
		t.Error("expected insufficient: remaining% = 10% < 20%")
	}
}

func TestNewReadinessEvaluatorFromConfig_WindowsUsesWindowsSize(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluatorFromConfig(ds, nil, 1, ReadinessEvalConfig{
		InstallPathLinux:        "/hab",
		InstallPathWindows:      `C:\hab`,
		InstallSizeMBLinux:      3072,
		InstallSizeMBWindows:    6144,
		MinRemainingFreePercent: 0, // disable percentage check
	})

	// Windows node with 5 GB free — less than 6144 MB required.
	snap := makeSnapshot("org-1", "win-node", false, nil,
		windowsFilesystemJSON(map[string]windowsDrive{
			"C:": {KBSize: "104857600", KBUsed: "99614720", KBAvailable: "5242880", PercentUsed: "95%"},
		}))
	snap.Platform = "windows"
	cache := ds.buildFakeCache()
	result := e.evaluateOne(snap, "18.0", ds.cookbookIDs, cache)
	if result.RequiredDiskMB != 6144 {
		t.Errorf("expected RequiredDiskMB 6144 for windows, got %d", result.RequiredDiskMB)
	}
	if result.SufficientDiskSpace == nil || *result.SufficientDiskSpace {
		t.Error("expected insufficient: 5120 MB < 6144 MB required for Windows")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsInner(s, substr))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
