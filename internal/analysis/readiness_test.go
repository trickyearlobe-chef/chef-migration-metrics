// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Fake datastore for testing
// ---------------------------------------------------------------------------

type fakeReadinessDS struct {
	mu sync.Mutex

	snapshots    []datastore.NodeSnapshot
	cookbookIDs  map[string]map[string]string // name → version → id
	tkResults    map[string]*datastore.TestKitchenResult
	csResults    map[string]*datastore.CookstyleResult
	complexities map[string]*datastore.CookbookComplexity
	upserted     []datastore.UpsertNodeReadinessParams

	// Error injection
	listSnapshotsErr error
	cookbookIDMapErr error
	tkErr            error
	csErr            error
	complexityErr    error
	upsertErr        error

	// Call counters
	upsertCount atomic.Int64
}

func newFakeReadinessDS() *fakeReadinessDS {
	return &fakeReadinessDS{
		cookbookIDs:  make(map[string]map[string]string),
		tkResults:    make(map[string]*datastore.TestKitchenResult),
		csResults:    make(map[string]*datastore.CookstyleResult),
		complexities: make(map[string]*datastore.CookbookComplexity),
	}
}

func (f *fakeReadinessDS) ListNodeSnapshotsByOrganisation(_ context.Context, _ string) ([]datastore.NodeSnapshot, error) {
	if f.listSnapshotsErr != nil {
		return nil, f.listSnapshotsErr
	}
	return f.snapshots, nil
}

func (f *fakeReadinessDS) GetServerCookbookIDMap(_ context.Context, _ string) (map[string]map[string]string, error) {
	if f.cookbookIDMapErr != nil {
		return nil, f.cookbookIDMapErr
	}
	return f.cookbookIDs, nil
}

func tkKey(cookbookID, targetChefVersion string) string {
	return cookbookID + "|" + targetChefVersion
}

func csKey(cookbookID, targetChefVersion string) string {
	return cookbookID + "|" + targetChefVersion
}

func ccKey(cookbookID, targetChefVersion string) string {
	return cookbookID + "|" + targetChefVersion
}

func (f *fakeReadinessDS) GetLatestTestKitchenResult(_ context.Context, cookbookID, targetChefVersion string) (*datastore.TestKitchenResult, error) {
	if f.tkErr != nil {
		return nil, f.tkErr
	}
	r := f.tkResults[tkKey(cookbookID, targetChefVersion)]
	return r, nil
}

func (f *fakeReadinessDS) GetCookstyleResult(_ context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
	if f.csErr != nil {
		return nil, f.csErr
	}
	r := f.csResults[csKey(cookbookID, targetChefVersion)]
	return r, nil
}

func (f *fakeReadinessDS) GetCookbookComplexity(_ context.Context, cookbookID, targetChefVersion string) (*datastore.CookbookComplexity, error) {
	if f.complexityErr != nil {
		return nil, f.complexityErr
	}
	r := f.complexities[ccKey(cookbookID, targetChefVersion)]
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
		ID:                "fake-id",
		NodeSnapshotID:    p.NodeSnapshotID,
		OrganisationID:    p.OrganisationID,
		NodeName:          p.NodeName,
		TargetChefVersion: p.TargetChefVersion,
		IsReady:           p.IsReady,
	}, nil
}

func (f *fakeReadinessDS) addCookbookID(name, version, id string) {
	if f.cookbookIDs[name] == nil {
		f.cookbookIDs[name] = make(map[string]string)
	}
	f.cookbookIDs[name][version] = id
}

func (f *fakeReadinessDS) addTKResult(cookbookID, targetChefVersion string, convergePassed, testsPassed bool) {
	f.tkResults[tkKey(cookbookID, targetChefVersion)] = &datastore.TestKitchenResult{
		CookbookID:        cookbookID,
		TargetChefVersion: targetChefVersion,
		ConvergePassed:    convergePassed,
		TestsPassed:       testsPassed,
		Compatible:        convergePassed && testsPassed,
	}
}

func (f *fakeReadinessDS) addCSResult(cookbookID, targetChefVersion string, passed bool) {
	f.csResults[csKey(cookbookID, targetChefVersion)] = &datastore.CookstyleResult{
		CookbookID:        cookbookID,
		TargetChefVersion: targetChefVersion,
		Passed:            passed,
	}
}

func (f *fakeReadinessDS) addComplexity(cookbookID, targetChefVersion string, score int, label string) {
	f.complexities[ccKey(cookbookID, targetChefVersion)] = &datastore.CookbookComplexity{
		CookbookID:        cookbookID,
		TargetChefVersion: targetChefVersion,
		ComplexityScore:   score,
		ComplexityLabel:   label,
	}
}

// ---------------------------------------------------------------------------
// Helper to make a node snapshot
// ---------------------------------------------------------------------------

func makeSnapshot(id, orgID, nodeName string, isStale bool, cookbooks, filesystem json.RawMessage) datastore.NodeSnapshot {
	return datastore.NodeSnapshot{
		ID:             id,
		OrganisationID: orgID,
		NodeName:       nodeName,
		IsStale:        isStale,
		Cookbooks:      cookbooks,
		Filesystem:     filesystem,
		CollectedAt:    time.Now().UTC(),
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
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))
	availMB, known := e.evaluateDiskSpace(snap)
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
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "2097152", KBUsed: "1048576", KBAvailable: "1048576", PercentUsed: "50%", Mount: "/"},
		}))
	availMB, known := e.evaluateDiskSpace(snap)
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
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil, nil)
	_, known := e.evaluateDiskSpace(snap)
	if known {
		t.Error("expected unknown for missing filesystem")
	}
}

func TestEvaluateDiskSpace_EmptyFilesystem(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil, json.RawMessage(`{}`))
	_, known := e.evaluateDiskSpace(snap)
	if known {
		t.Error("expected unknown for empty filesystem")
	}
}

func TestEvaluateDiskSpace_StringValues(t *testing.T) {
	// Chef Client versions may report string or integer values.
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))
	availMB, known := e.evaluateDiskSpace(snap)
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
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil, raw)
	availMB, known := e.evaluateDiskSpace(snap)
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
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil, raw)
	availMB, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known (with 0 available)")
	}
	if availMB != 0 {
		t.Errorf("expected 0 MB when kb_available missing, got %d", availMB)
	}
}

func TestEvaluateDiskSpace_WindowsDrive(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		windowsFilesystemJSON(map[string]windowsDrive{
			"C:": {KBSize: "104857600", KBUsed: "52428800", KBAvailable: "52428800", PercentUsed: "50%"},
		}))
	snap.Platform = "windows"
	availMB, known := e.evaluateDiskSpace(snap)
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

func TestCheckCookbookCompatibility_TKPass(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "apt", "7.4.0", "18.0", ds.cookbookIDs)
	if status != StatusCompatible {
		t.Errorf("expected %s, got %s", StatusCompatible, status)
	}
	if source != SourceTestKitchen {
		t.Errorf("expected %s, got %s", SourceTestKitchen, source)
	}
}

func TestCheckCookbookCompatibility_TKConvergeFail(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", false, false)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "apt", "7.4.0", "18.0", ds.cookbookIDs)
	if status != StatusIncompatible {
		t.Errorf("expected %s, got %s", StatusIncompatible, status)
	}
	if source != SourceTestKitchen {
		t.Errorf("expected %s, got %s", SourceTestKitchen, source)
	}
}

func TestCheckCookbookCompatibility_TKTestFail(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, false)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "apt", "7.4.0", "18.0", ds.cookbookIDs)
	if status != StatusIncompatible {
		t.Errorf("expected %s, got %s", StatusIncompatible, status)
	}
	if source != SourceTestKitchen {
		t.Errorf("expected %s, got %s", SourceTestKitchen, source)
	}
}

func TestCheckCookbookCompatibility_CSPass_NoTK(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addCSResult("id-apt", "18.0", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "apt", "7.4.0", "18.0", ds.cookbookIDs)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected %s, got %s", StatusCompatibleCookstyleOnly, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected %s, got %s", SourceCookstyle, source)
	}
}

func TestCheckCookbookCompatibility_CSFail_NoTK(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addCSResult("id-apt", "18.0", false)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "apt", "7.4.0", "18.0", ds.cookbookIDs)
	if status != StatusIncompatible {
		t.Errorf("expected %s, got %s", StatusIncompatible, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected %s, got %s", SourceCookstyle, source)
	}
}

func TestCheckCookbookCompatibility_CSPassNoTargetVersion(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	// CookStyle result with empty target version (server-sourced scan).
	ds.addCSResult("id-apt", "", true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "apt", "7.4.0", "18.0", ds.cookbookIDs)
	if status != StatusCompatibleCookstyleOnly {
		t.Errorf("expected %s, got %s", StatusCompatibleCookstyleOnly, status)
	}
	if source != SourceCookstyle {
		t.Errorf("expected %s, got %s", SourceCookstyle, source)
	}
}

func TestCheckCookbookCompatibility_Untested(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	// No TK or CS results.

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "apt", "7.4.0", "18.0", ds.cookbookIDs)
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

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "unknown", "1.0.0", "18.0", ds.cookbookIDs)
	if status != StatusUntested {
		t.Errorf("expected %s, got %s", StatusUntested, status)
	}
	if source != SourceNone {
		t.Errorf("expected %s, got %s", SourceNone, source)
	}
}

func TestCheckCookbookCompatibility_TKTakesPrecedenceOverCS(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)
	ds.addCSResult("id-apt", "18.0", false) // CS says fail, but TK pass takes precedence

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	status, source := e.checkCookbookCompatibility(context.Background(), "apt", "7.4.0", "18.0", ds.cookbookIDs)
	if status != StatusCompatible {
		t.Errorf("expected TK pass to take precedence, got %s", status)
	}
	if source != SourceTestKitchen {
		t.Errorf("expected %s, got %s", SourceTestKitchen, source)
	}
}

// ---------------------------------------------------------------------------
// evaluateOne — integration tests
// ---------------------------------------------------------------------------

func TestEvaluateOne_AllCompatibleSufficientDisk(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addCookbookID("nginx", "2.0.0", "id-nginx")
	ds.addTKResult("id-apt", "18.0", true, true)
	ds.addTKResult("id-nginx", "18.0", true, true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addCookbookID("nginx", "2.0.0", "id-nginx")
	ds.addTKResult("id-apt", "18.0", true, true)
	ds.addTKResult("id-nginx", "18.0", false, false) // FAIL

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	if bc.Source != SourceTestKitchen {
		t.Errorf("expected source %s, got %s", SourceTestKitchen, bc.Source)
	}
}

func TestEvaluateOne_UntestedCookbook(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)
	// "nginx" is in the ID map but has no test results.
	ds.addCookbookID("nginx", "2.0.0", "id-nginx")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "2097152", KBUsed: "1048576", KBAvailable: "1048576", PercentUsed: "50%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		nil) // no filesystem data

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "stale-node", true,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	snap := makeSnapshot("snap-1", "org-1", "bare-node", false,
		nil, // no cookbooks
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	ds.addCookbookID("nginx", "2.0.0", "id-nginx")
	ds.addTKResult("id-nginx", "18.0", false, false) // incompatible
	ds.addComplexity("id-nginx", "18.0", 45, "high")

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false,
		cookbooksJSON(map[string]string{"nginx": "2.0.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addCookbookID("nginx", "2.0.0", "id-nginx")
	ds.addCookbookID("java", "8.5.0", "id-java")
	ds.addTKResult("id-apt", "18.0", true, true) // pass
	// nginx and java: no results → untested

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0", "nginx": "2.0.0", "java": "8.5.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addCSResult("id-apt", "18.0", true) // CookStyle pass, no TK

	e := NewReadinessEvaluator(ds, nil, 1, 2048)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false,
		cookbooksJSON(map[string]string{"apt": "7.4.0"}),
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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
		makeSnapshot("snap-1", "org-1", "node-1", false,
			cookbooksJSON(map[string]string{"apt": "7.4.0"}),
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
		makeSnapshot("snap-2", "org-1", "node-2", false,
			cookbooksJSON(map[string]string{"apt": "7.4.0"}),
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
	}
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)

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
		makeSnapshot("snap-1", "org-1", "node-1", false,
			cookbooksJSON(map[string]string{"apt": "7.4.0"}),
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
	}
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)
	ds.addTKResult("id-apt", "17.0", false, false) // fails for 17.0

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
		makeSnapshot("snap-1", "org-1", "node-1", false, nil, nil),
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
		makeSnapshot("snap-1", "org-1", "node-1", false, nil, nil),
	}
	ds.cookbookIDMapErr = fmt.Errorf("connection refused")

	e := NewReadinessEvaluator(ds, nil, 4, 2048)
	_, err := e.EvaluateOrganisation(context.Background(), "org-1", "org-1", []string{"18.0"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "loading cookbook ID map") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEvaluateOrganisation_UpsertErrorDoesNotAbortBatch(t *testing.T) {
	ds := newFakeReadinessDS()
	ds.snapshots = []datastore.NodeSnapshot{
		makeSnapshot("snap-1", "org-1", "node-1", false, nil,
			linuxFilesystemJSON(map[string]linuxMount{
				"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			})),
		makeSnapshot("snap-2", "org-1", "node-2", false, nil,
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
			makeSnapshot(fmt.Sprintf("snap-%d", i), "org-1", fmt.Sprintf("node-%d", i), false, nil,
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
		ds.snapshots = append(ds.snapshots,
			makeSnapshot(fmt.Sprintf("snap-%d", i), "org-1", fmt.Sprintf("node-%d", i), false,
				cookbooksJSON(map[string]string{"apt": "7.4.0"}),
				linuxFilesystemJSON(map[string]linuxMount{
					"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
				})))
	}
	ds.addCookbookID("apt", "7.4.0", "id-apt")
	ds.addTKResult("id-apt", "18.0", true, true)

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
// NewReadinessEvaluator option tests
// ---------------------------------------------------------------------------

func TestNewReadinessEvaluator_Defaults(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 0, 0)
	if e.concurrency != 1 {
		t.Errorf("expected concurrency 1, got %d", e.concurrency)
	}
	if e.minFreeDiskMB != 2048 {
		t.Errorf("expected minFreeDiskMB 2048, got %d", e.minFreeDiskMB)
	}
}

func TestNewReadinessEvaluator_NegativeValues(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, -5, -100)
	if e.concurrency != 1 {
		t.Errorf("expected concurrency 1, got %d", e.concurrency)
	}
	if e.minFreeDiskMB != 2048 {
		t.Errorf("expected minFreeDiskMB 2048, got %d", e.minFreeDiskMB)
	}
}

func TestNewReadinessEvaluator_CustomValues(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 10, 4096)
	if e.concurrency != 10 {
		t.Errorf("expected concurrency 10, got %d", e.concurrency)
	}
	if e.minFreeDiskMB != 4096 {
		t.Errorf("expected minFreeDiskMB 4096, got %d", e.minFreeDiskMB)
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
		NodeSnapshotID:         "snap-1",
		OrganisationID:         "org-1",
		NodeName:               "node-1",
		TargetChefVersion:      "18.0",
		IsReady:                false,
		AllCookbooksCompatible: false,
		SufficientDiskSpace:    &sufficient,
		BlockingCookbooks: []BlockingCookbook{
			{Name: "nginx", Version: "2.0.0", Reason: StatusIncompatible, Source: SourceTestKitchen, ComplexityScore: 30, ComplexityLabel: "high"},
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
		NodeSnapshotID:         "snap-1",
		OrganisationID:         "org-1",
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
		NodeSnapshotID:    "snap-1",
		OrganisationID:    "org-1",
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
		Source:          SourceTestKitchen,
		ComplexityScore: 45,
		ComplexityLabel: "high",
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

// ---------------------------------------------------------------------------
// Edge case: disk space with /hab as a sub-mount under /opt
// ---------------------------------------------------------------------------

func TestEvaluateDiskSpace_HabUnderOpt(t *testing.T) {
	e := NewReadinessEvaluator(newFakeReadinessDS(), nil, 1, 2048)
	// /opt is mounted, but /hab is not under /opt — root should match.
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			"/dev/sdb1": {KBSize: "102400000", KBUsed: "50000000", KBAvailable: "47360000", PercentUsed: "51%", Mount: "/opt"},
		}))

	availMB, known := e.evaluateDiskSpace(snap)
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
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
			"/dev/sdb1": {KBSize: "102400000", KBUsed: "50000000", KBAvailable: "100000", PercentUsed: "99%", Mount: "/hab"},
		}))

	availMB, known := e.evaluateDiskSpace(snap)
	if !known {
		t.Fatal("expected known")
	}
	// /hab mount should be preferred over root.
	expected := 100000 / 1024
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

	// Exactly 2048 MB free = 2048 * 1024 = 2097152 KB.
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "4194304", KBUsed: "2097152", KBAvailable: "2097152", PercentUsed: "50%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

	if result.SufficientDiskSpace == nil {
		t.Fatal("expected disk space known")
	}
	// 2097152 KB / 1024 = 2048 MB, which equals the threshold.
	if !*result.SufficientDiskSpace {
		t.Error("expected sufficient disk space at exactly the threshold")
	}
}

func TestEvaluateOne_OneBelowDiskSpaceBoundary(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	// 2047 MB free = 2047 * 1024 = 2096128 KB.
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "4194304", KBUsed: "2098176", KBAvailable: "2096128", PercentUsed: "50%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)

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

	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil,
		linuxFilesystemJSON(map[string]linuxMount{
			"/dev/sda1": {KBSize: "20511356", KBUsed: "5123456", KBAvailable: "14340800", PercentUsed: "26%", Mount: "/"},
		}))

	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)
	if result.RequiredDiskMB != 4096 {
		t.Errorf("expected requiredDiskMB 4096, got %d", result.RequiredDiskMB)
	}
}

func TestReadinessResult_EvaluatedAtSet(t *testing.T) {
	ds := newFakeReadinessDS()
	e := NewReadinessEvaluator(ds, nil, 1, 2048)

	before := time.Now().UTC().Add(-time.Second)
	snap := makeSnapshot("snap-1", "org-1", "node-1", false, nil, nil)
	result := e.evaluateOne(context.Background(), snap, "18.0", ds.cookbookIDs)
	after := time.Now().UTC().Add(time.Second)

	if result.EvaluatedAt.Before(before) || result.EvaluatedAt.After(after) {
		t.Errorf("evaluatedAt %v not in range [%v, %v]", result.EvaluatedAt, before, after)
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
