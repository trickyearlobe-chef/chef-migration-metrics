// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func intPtr(i int) *int { return &i }

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ---------------------------------------------------------------------------
// blockingEntry helpers for building test data
// ---------------------------------------------------------------------------

type testVerdict struct {
	Source string `json:"source"`
	Status string `json:"status"`
}

type testBlocking struct {
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Reason   string        `json:"reason"`
	Source   string        `json:"source"`
	Verdicts []testVerdict `json:"verdicts"`
}

// ---------------------------------------------------------------------------
// Disk status tests
// ---------------------------------------------------------------------------

func TestDeriveDiskStatus_Sufficient(t *testing.T) {
	nr := datastore.NodeReadiness{SufficientDiskSpace: boolPtr(true)}
	if got := deriveDiskStatus(nr); got != DiskStatusSufficient {
		t.Errorf("deriveDiskStatus = %q, want %q", got, DiskStatusSufficient)
	}
}

func TestDeriveDiskStatus_Insufficient(t *testing.T) {
	nr := datastore.NodeReadiness{SufficientDiskSpace: boolPtr(false)}
	if got := deriveDiskStatus(nr); got != DiskStatusInsufficient {
		t.Errorf("deriveDiskStatus = %q, want %q", got, DiskStatusInsufficient)
	}
}

func TestDeriveDiskStatus_Unknown_Nil(t *testing.T) {
	nr := datastore.NodeReadiness{SufficientDiskSpace: nil}
	if got := deriveDiskStatus(nr); got != DiskStatusUnknown {
		t.Errorf("deriveDiskStatus = %q, want %q", got, DiskStatusUnknown)
	}
}

// ---------------------------------------------------------------------------
// CookStyle status tests
// ---------------------------------------------------------------------------

func TestDeriveCookstyleStatus_AllPassed(t *testing.T) {
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: true,
		BlockingCookbooks:      mustJSON([]testBlocking{}),
	}
	if got := deriveCookstyleStatus(nr); got != CookstyleStatusPassed {
		t.Errorf("deriveCookstyleStatus = %q, want %q", got, CookstyleStatusPassed)
	}
}

func TestDeriveCookstyleStatus_Failed(t *testing.T) {
	blocking := []testBlocking{{
		Name:    "apt",
		Version: "1.0.0",
		Reason:  "incompatible",
		Source:  "cookstyle",
		Verdicts: []testVerdict{
			{Source: "server_cookstyle", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
	}
	if got := deriveCookstyleStatus(nr); got != CookstyleStatusFailed {
		t.Errorf("deriveCookstyleStatus = %q, want %q", got, CookstyleStatusFailed)
	}
}

func TestDeriveCookstyleStatus_FailedMultipleCookbooks(t *testing.T) {
	blocking := []testBlocking{
		{
			Name:   "apt",
			Reason: "incompatible",
			Verdicts: []testVerdict{
				{Source: "server_cookstyle", Status: "incompatible"},
			},
		},
		{
			Name:   "yum",
			Reason: "incompatible",
			Verdicts: []testVerdict{
				{Source: "git_cookstyle", Status: "incompatible"},
			},
		},
	}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
	}
	if got := deriveCookstyleStatus(nr); got != CookstyleStatusFailed {
		t.Errorf("deriveCookstyleStatus = %q, want %q", got, CookstyleStatusFailed)
	}
}

func TestDeriveCookstyleStatus_Unknown_Stale(t *testing.T) {
	nr := datastore.NodeReadiness{
		StaleData:              true,
		AllCookbooksCompatible: true,
	}
	if got := deriveCookstyleStatus(nr); got != CookstyleStatusUnknown {
		t.Errorf("deriveCookstyleStatus = %q, want %q", got, CookstyleStatusUnknown)
	}
}

func TestDeriveCookstyleStatus_Unknown_Untested(t *testing.T) {
	blocking := []testBlocking{{
		Name:     "mystery",
		Version:  "1.0.0",
		Reason:   "untested",
		Source:   "none",
		Verdicts: []testVerdict{},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
	}
	if got := deriveCookstyleStatus(nr); got != CookstyleStatusUnknown {
		t.Errorf("deriveCookstyleStatus = %q, want %q", got, CookstyleStatusUnknown)
	}
}

func TestDeriveCookstyleStatus_PassedWhenOnlyTKFails(t *testing.T) {
	blocking := []testBlocking{{
		Name:   "web-app",
		Reason: "incompatible",
		Source: "test_kitchen",
		Verdicts: []testVerdict{
			{Source: "server_cookstyle", Status: "compatible"},
			{Source: "git_test_kitchen", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
	}
	if got := deriveCookstyleStatus(nr); got != CookstyleStatusPassed {
		t.Errorf("deriveCookstyleStatus = %q, want %q", got, CookstyleStatusPassed)
	}
}

func TestDeriveCookstyleStatus_PassedWhenOnlyTKFails_NoCSVerdicts(t *testing.T) {
	// Blocking entries with only TK failures and no cookstyle verdicts at all.
	blocking := []testBlocking{{
		Name:   "web-app",
		Reason: "incompatible",
		Source: "test_kitchen",
		Verdicts: []testVerdict{
			{Source: "git_test_kitchen", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
	}
	if got := deriveCookstyleStatus(nr); got != CookstyleStatusPassed {
		t.Errorf("deriveCookstyleStatus = %q, want %q", got, CookstyleStatusPassed)
	}
}

// ---------------------------------------------------------------------------
// Kitchen status tests
// ---------------------------------------------------------------------------

func TestDeriveKitchenStatus_AllPassed(t *testing.T) {
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: true,
		BlockingCookbooks:      mustJSON([]testBlocking{}),
	}
	if got := deriveKitchenStatus(nr); got != KitchenStatusPassed {
		t.Errorf("deriveKitchenStatus = %q, want %q", got, KitchenStatusPassed)
	}
}

func TestDeriveKitchenStatus_Failed(t *testing.T) {
	blocking := []testBlocking{{
		Name:   "web-app",
		Reason: "incompatible",
		Source: "test_kitchen",
		Verdicts: []testVerdict{
			{Source: "git_test_kitchen", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
	}
	if got := deriveKitchenStatus(nr); got != KitchenStatusFailed {
		t.Errorf("deriveKitchenStatus = %q, want %q", got, KitchenStatusFailed)
	}
}

func TestDeriveKitchenStatus_Partial(t *testing.T) {
	// One cookbook tested with TK (passed), another has no TK verdict.
	blocking := []testBlocking{
		{
			Name:   "web-app",
			Reason: "incompatible",
			Source: "cookstyle",
			Verdicts: []testVerdict{
				{Source: "server_cookstyle", Status: "incompatible"},
				{Source: "git_test_kitchen", Status: "compatible"},
			},
		},
		{
			Name:   "legacy",
			Reason: "untested",
			Source: "none",
			Verdicts: []testVerdict{
				{Source: "server_cookstyle", Status: "incompatible"},
			},
		},
	}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
	}
	if got := deriveKitchenStatus(nr); got != KitchenStatusPartial {
		t.Errorf("deriveKitchenStatus = %q, want %q", got, KitchenStatusPartial)
	}
}

func TestDeriveKitchenStatus_Unknown_Stale(t *testing.T) {
	nr := datastore.NodeReadiness{
		StaleData:              true,
		AllCookbooksCompatible: true,
	}
	if got := deriveKitchenStatus(nr); got != KitchenStatusUnknown {
		t.Errorf("deriveKitchenStatus = %q, want %q", got, KitchenStatusUnknown)
	}
}

func TestDeriveKitchenStatus_Unknown_NoResults(t *testing.T) {
	blocking := []testBlocking{{
		Name:   "apt",
		Reason: "incompatible",
		Source: "cookstyle",
		Verdicts: []testVerdict{
			{Source: "server_cookstyle", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{
		AllCookbooksCompatible: false,
		BlockingCookbooks:      mustJSON(blocking),
	}
	if got := deriveKitchenStatus(nr); got != KitchenStatusUnknown {
		t.Errorf("deriveKitchenStatus = %q, want %q", got, KitchenStatusUnknown)
	}
}

// ---------------------------------------------------------------------------
// Integration: deriveCheckStatus
// ---------------------------------------------------------------------------

func TestDeriveCheckStatus_NilBlockingCookbooks(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace:    boolPtr(true),
		AllCookbooksCompatible: true,
		BlockingCookbooks:      nil,
		AvailableDiskMB:        intPtr(4096),
	}
	got := deriveCheckStatus(nr)
	if got.DiskStatus != DiskStatusSufficient {
		t.Errorf("DiskStatus = %q, want %q", got.DiskStatus, DiskStatusSufficient)
	}
	if got.CookstyleStatus != CookstyleStatusPassed {
		t.Errorf("CookstyleStatus = %q, want %q", got.CookstyleStatus, CookstyleStatusPassed)
	}
	if got.KitchenStatus != KitchenStatusPassed {
		t.Errorf("KitchenStatus = %q, want %q", got.KitchenStatus, KitchenStatusPassed)
	}
}

func TestDeriveCheckStatus_EmptyBlockingCookbooks(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace:    boolPtr(false),
		AllCookbooksCompatible: true,
		BlockingCookbooks:      json.RawMessage(`[]`),
		AvailableDiskMB:        intPtr(512),
		RequiredDiskMB:         intPtr(2048),
	}
	got := deriveCheckStatus(nr)
	if got.DiskStatus != DiskStatusInsufficient {
		t.Errorf("DiskStatus = %q, want %q", got.DiskStatus, DiskStatusInsufficient)
	}
	if got.CookstyleStatus != CookstyleStatusPassed {
		t.Errorf("CookstyleStatus = %q, want %q", got.CookstyleStatus, CookstyleStatusPassed)
	}
	if got.KitchenStatus != KitchenStatusPassed {
		t.Errorf("KitchenStatus = %q, want %q", got.KitchenStatus, KitchenStatusPassed)
	}
}

func TestDeriveCheckStatus_InvalidJSON(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace:    nil,
		AllCookbooksCompatible: false,
		BlockingCookbooks:      json.RawMessage(`{not valid`),
	}
	got := deriveCheckStatus(nr)
	// Invalid JSON should not panic; returns unknown.
	if got.CookstyleStatus != CookstyleStatusUnknown {
		t.Errorf("CookstyleStatus = %q, want %q", got.CookstyleStatus, CookstyleStatusUnknown)
	}
	if got.KitchenStatus != KitchenStatusUnknown {
		t.Errorf("KitchenStatus = %q, want %q", got.KitchenStatus, KitchenStatusUnknown)
	}
}

func TestDeriveCheckStatus_NullJSON(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace:    boolPtr(true),
		AllCookbooksCompatible: true,
		BlockingCookbooks:      json.RawMessage(`null`),
	}
	got := deriveCheckStatus(nr)
	if got.CookstyleStatus != CookstyleStatusPassed {
		t.Errorf("CookstyleStatus = %q, want %q", got.CookstyleStatus, CookstyleStatusPassed)
	}
}

// ---------------------------------------------------------------------------
// Disk detail tests
// ---------------------------------------------------------------------------

func TestDiskDetail_WithAvailableAndRequired(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace: boolPtr(false),
		AvailableDiskMB:     intPtr(512),
		RequiredDiskMB:      intPtr(2048),
	}
	got := diskDetail(nr)
	if got == nil {
		t.Fatal("diskDetail returned nil")
	}
	want := "Disk: insufficient (0.5 GB free, need 2.0 GB)"
	if *got != want {
		t.Errorf("diskDetail = %q, want %q", *got, want)
	}
}

func TestDiskDetail_Sufficient_WithMB(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace: boolPtr(true),
		AvailableDiskMB:     intPtr(4096),
	}
	got := diskDetail(nr)
	if got == nil {
		t.Fatal("diskDetail returned nil")
	}
	want := "Disk: sufficient (4.0 GB free)"
	if *got != want {
		t.Errorf("diskDetail = %q, want %q", *got, want)
	}
}

func TestDiskDetail_Sufficient_NoMB(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace: boolPtr(true),
	}
	got := diskDetail(nr)
	if got == nil {
		t.Fatal("diskDetail returned nil")
	}
	if *got != "Disk: sufficient" {
		t.Errorf("diskDetail = %q, want %q", *got, "Disk: sufficient")
	}
}

func TestDiskDetail_Insufficient_NoMB(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace: boolPtr(false),
	}
	got := diskDetail(nr)
	if got == nil {
		t.Fatal("diskDetail returned nil")
	}
	if *got != "Disk: insufficient" {
		t.Errorf("diskDetail = %q, want %q", *got, "Disk: insufficient")
	}
}

func TestDiskDetail_Insufficient_AvailableOnly(t *testing.T) {
	nr := datastore.NodeReadiness{
		SufficientDiskSpace: boolPtr(false),
		AvailableDiskMB:     intPtr(256),
	}
	got := diskDetail(nr)
	if got == nil {
		t.Fatal("diskDetail returned nil")
	}
	want := "Disk: insufficient (0.2 GB free)"
	if *got != want {
		t.Errorf("diskDetail = %q, want %q", *got, want)
	}
}

func TestDiskDetail_Unknown(t *testing.T) {
	nr := datastore.NodeReadiness{SufficientDiskSpace: nil}
	got := diskDetail(nr)
	if got == nil {
		t.Fatal("diskDetail returned nil")
	}
	if *got != "Disk: unknown" {
		t.Errorf("diskDetail = %q, want %q", *got, "Disk: unknown")
	}
}

// ---------------------------------------------------------------------------
// CookStyle detail tests
// ---------------------------------------------------------------------------

func TestCookstyleDetail_Passed(t *testing.T) {
	nr := datastore.NodeReadiness{}
	got := cookstyleDetail(nr, CookstyleStatusPassed)
	if got == nil {
		t.Fatal("cookstyleDetail returned nil")
	}
	if *got != "CookStyle: all cookbooks passed" {
		t.Errorf("cookstyleDetail = %q, want %q", *got, "CookStyle: all cookbooks passed")
	}
}

func TestCookstyleDetail_Failed(t *testing.T) {
	blocking := []testBlocking{
		{
			Name: "apt",
			Verdicts: []testVerdict{
				{Source: "server_cookstyle", Status: "incompatible"},
			},
		},
		{
			Name: "yum",
			Verdicts: []testVerdict{
				{Source: "git_cookstyle", Status: "incompatible"},
			},
		},
	}
	nr := datastore.NodeReadiness{BlockingCookbooks: mustJSON(blocking)}
	got := cookstyleDetail(nr, CookstyleStatusFailed)
	if got == nil {
		t.Fatal("cookstyleDetail returned nil")
	}
	want := "CookStyle: 2 cookbooks incompatible"
	if *got != want {
		t.Errorf("cookstyleDetail = %q, want %q", *got, want)
	}
}

func TestCookstyleDetail_Failed_Single(t *testing.T) {
	blocking := []testBlocking{{
		Name: "apt",
		Verdicts: []testVerdict{
			{Source: "server_cookstyle", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{BlockingCookbooks: mustJSON(blocking)}
	got := cookstyleDetail(nr, CookstyleStatusFailed)
	if got == nil {
		t.Fatal("cookstyleDetail returned nil")
	}
	want := "CookStyle: 1 cookbook incompatible"
	if *got != want {
		t.Errorf("cookstyleDetail = %q, want %q", *got, want)
	}
}

func TestCookstyleDetail_Unknown(t *testing.T) {
	nr := datastore.NodeReadiness{}
	got := cookstyleDetail(nr, CookstyleStatusUnknown)
	if got == nil {
		t.Fatal("cookstyleDetail returned nil")
	}
	if *got != "CookStyle: not scanned" {
		t.Errorf("cookstyleDetail = %q, want %q", *got, "CookStyle: not scanned")
	}
}

// ---------------------------------------------------------------------------
// Kitchen detail tests
// ---------------------------------------------------------------------------

func TestKitchenDetail_Passed(t *testing.T) {
	nr := datastore.NodeReadiness{}
	got := kitchenDetail(nr, KitchenStatusPassed)
	if got == nil {
		t.Fatal("kitchenDetail returned nil")
	}
	if *got != "Test Kitchen: all passed" {
		t.Errorf("kitchenDetail = %q, want %q", *got, "Test Kitchen: all passed")
	}
}

func TestKitchenDetail_Failed(t *testing.T) {
	blocking := []testBlocking{
		{
			Name: "web-app",
			Verdicts: []testVerdict{
				{Source: "git_test_kitchen", Status: "incompatible"},
			},
		},
		{
			Name: "api",
			Verdicts: []testVerdict{
				{Source: "git_test_kitchen", Status: "incompatible"},
			},
		},
	}
	nr := datastore.NodeReadiness{BlockingCookbooks: mustJSON(blocking)}
	got := kitchenDetail(nr, KitchenStatusFailed)
	if got == nil {
		t.Fatal("kitchenDetail returned nil")
	}
	want := "Test Kitchen: 2 cookbooks failed"
	if *got != want {
		t.Errorf("kitchenDetail = %q, want %q", *got, want)
	}
}

func TestKitchenDetail_Failed_Single(t *testing.T) {
	blocking := []testBlocking{{
		Name: "web-app",
		Verdicts: []testVerdict{
			{Source: "git_test_kitchen", Status: "incompatible"},
		},
	}}
	nr := datastore.NodeReadiness{BlockingCookbooks: mustJSON(blocking)}
	got := kitchenDetail(nr, KitchenStatusFailed)
	if got == nil {
		t.Fatal("kitchenDetail returned nil")
	}
	want := "Test Kitchen: 1 cookbook failed"
	if *got != want {
		t.Errorf("kitchenDetail = %q, want %q", *got, want)
	}
}

func TestKitchenDetail_Partial(t *testing.T) {
	nr := datastore.NodeReadiness{}
	got := kitchenDetail(nr, KitchenStatusPartial)
	if got == nil {
		t.Fatal("kitchenDetail returned nil")
	}
	if *got != "Test Kitchen: partially tested" {
		t.Errorf("kitchenDetail = %q, want %q", *got, "Test Kitchen: partially tested")
	}
}

func TestKitchenDetail_Unknown(t *testing.T) {
	nr := datastore.NodeReadiness{}
	got := kitchenDetail(nr, KitchenStatusUnknown)
	if got == nil {
		t.Fatal("kitchenDetail returned nil")
	}
	if *got != "Test Kitchen: not tested" {
		t.Errorf("kitchenDetail = %q, want %q", *got, "Test Kitchen: not tested")
	}
}

// ---------------------------------------------------------------------------
// parseBlockingCookbooks edge cases
// ---------------------------------------------------------------------------

func TestParseBlockingCookbooks_Empty(t *testing.T) {
	if got := parseBlockingCookbooks(nil); got != nil {
		t.Errorf("parseBlockingCookbooks(nil) = %v, want nil", got)
	}
	if got := parseBlockingCookbooks(json.RawMessage(`null`)); got != nil {
		t.Errorf("parseBlockingCookbooks(null) = %v, want nil", got)
	}
	if got := parseBlockingCookbooks(json.RawMessage(``)); got != nil {
		t.Errorf("parseBlockingCookbooks(empty) = %v, want nil", got)
	}
}

func TestParseBlockingCookbooks_InvalidJSON(t *testing.T) {
	if got := parseBlockingCookbooks(json.RawMessage(`{broken`)); got != nil {
		t.Errorf("parseBlockingCookbooks(invalid) = %v, want nil", got)
	}
}

func TestParseBlockingCookbooks_ValidArray(t *testing.T) {
	data := mustJSON([]testBlocking{
		{Name: "apt", Version: "1.0.0", Reason: "incompatible"},
		{Name: "yum", Version: "2.0.0", Reason: "untested"},
	})
	got := parseBlockingCookbooks(data)
	if len(got) != 2 {
		t.Fatalf("parseBlockingCookbooks len = %d, want 2", len(got))
	}
	if got[0].Name != "apt" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "apt")
	}
	if got[1].Reason != "untested" {
		t.Errorf("got[1].Reason = %q, want %q", got[1].Reason, "untested")
	}
}

// ---------------------------------------------------------------------------
// isCookstyleSource tests
// ---------------------------------------------------------------------------

func TestIsCookstyleSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"server_cookstyle", true},
		{"git_cookstyle", true},
		{"git_test_kitchen", false},
		{"none", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isCookstyleSource(tc.source); got != tc.want {
			t.Errorf("isCookstyleSource(%q) = %v, want %v", tc.source, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// isOnlyTKFailure tests
// ---------------------------------------------------------------------------

func TestIsOnlyTKFailure_True(t *testing.T) {
	b := blockingEntry{
		Verdicts: []struct {
			Source string `json:"source"`
			Status string `json:"status"`
		}{
			{Source: "git_test_kitchen", Status: "incompatible"},
		},
	}
	if !isOnlyTKFailure(b) {
		t.Error("isOnlyTKFailure = false, want true")
	}
}

func TestIsOnlyTKFailure_FalseWithCSFail(t *testing.T) {
	b := blockingEntry{
		Verdicts: []struct {
			Source string `json:"source"`
			Status string `json:"status"`
		}{
			{Source: "server_cookstyle", Status: "incompatible"},
			{Source: "git_test_kitchen", Status: "incompatible"},
		},
	}
	if isOnlyTKFailure(b) {
		t.Error("isOnlyTKFailure = true, want false")
	}
}

func TestIsOnlyTKFailure_FalseNoTK(t *testing.T) {
	b := blockingEntry{
		Verdicts: []struct {
			Source string `json:"source"`
			Status string `json:"status"`
		}{
			{Source: "server_cookstyle", Status: "compatible"},
		},
	}
	if isOnlyTKFailure(b) {
		t.Error("isOnlyTKFailure = true, want false (no TK fail)")
	}
}

// ---------------------------------------------------------------------------
// countCookstyleFailures / countKitchenFailures
// ---------------------------------------------------------------------------

func TestCountCookstyleFailures(t *testing.T) {
	blocking := []testBlocking{
		{Name: "a", Verdicts: []testVerdict{{Source: "server_cookstyle", Status: "incompatible"}}},
		{Name: "b", Verdicts: []testVerdict{{Source: "git_cookstyle", Status: "compatible"}}},
		{Name: "c", Verdicts: []testVerdict{{Source: "git_test_kitchen", Status: "incompatible"}}},
		{Name: "d", Verdicts: []testVerdict{{Source: "git_cookstyle", Status: "incompatible"}}},
	}
	if got := countCookstyleFailures(mustJSON(blocking)); got != 2 {
		t.Errorf("countCookstyleFailures = %d, want 2", got)
	}
}

func TestCountCookstyleFailures_None(t *testing.T) {
	if got := countCookstyleFailures(nil); got != 0 {
		t.Errorf("countCookstyleFailures(nil) = %d, want 0", got)
	}
}

func TestCountKitchenFailures(t *testing.T) {
	blocking := []testBlocking{
		{Name: "a", Verdicts: []testVerdict{{Source: "git_test_kitchen", Status: "incompatible"}}},
		{Name: "b", Verdicts: []testVerdict{{Source: "git_test_kitchen", Status: "compatible"}}},
		{Name: "c", Verdicts: []testVerdict{{Source: "server_cookstyle", Status: "incompatible"}}},
	}
	if got := countKitchenFailures(mustJSON(blocking)); got != 1 {
		t.Errorf("countKitchenFailures = %d, want 1", got)
	}
}

func TestCountKitchenFailures_None(t *testing.T) {
	if got := countKitchenFailures(nil); got != 0 {
		t.Errorf("countKitchenFailures(nil) = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// strPtr
// ---------------------------------------------------------------------------

func TestStrPtr(t *testing.T) {
	got := strPtr("hello")
	if got == nil || *got != "hello" {
		t.Errorf("strPtr(\"hello\") = %v, want pointer to \"hello\"", got)
	}
}
