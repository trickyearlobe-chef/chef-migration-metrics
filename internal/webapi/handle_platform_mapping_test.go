// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/hypervisor"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func tkConfigJSON(t *testing.T, cfg config.TestKitchenConfig) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("tkConfigJSON: %v", err)
	}
	return b
}

func decodePlatformMappingResponse(t *testing.T, rec *httptest.ResponseRecorder) PlatformMappingStatusResponse {
	t.Helper()
	var resp PlatformMappingStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return resp
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHandlePlatformMappingStatus_AllMapped(t *testing.T) {
	tkCfg := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ubuntu-tmpl"},
			{KitchenName: "centos-7", Image: "centos-tmpl"},
			{KitchenName: "windows-2022", Image: "win-tmpl"},
		},
	}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{
				{PlatformName: "ubuntu-22.04", NormalisedName: "ubuntu-22.04", OSFamily: "debian", CookbookCount: 50},
				{PlatformName: "centos-7", NormalisedName: "centos-7", OSFamily: "rhel", CookbookCount: 30},
				{PlatformName: "windows-2022", NormalisedName: "windows-2022", OSFamily: "windows", CookbookCount: 10},
			}, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)
	if len(resp.DiscoveredPlatforms) != 3 {
		t.Fatalf("discovered_platforms count = %d, want 3", len(resp.DiscoveredPlatforms))
	}
	for i, dp := range resp.DiscoveredPlatforms {
		if dp.MappingStatus != "mapped" {
			t.Errorf("discovered_platforms[%d].mapping_status = %q, want %q", i, dp.MappingStatus, "mapped")
		}
	}
	if resp.MappedCount != 3 {
		t.Errorf("mapped_count = %d, want 3", resp.MappedCount)
	}
	if resp.UnmappedCount != 0 {
		t.Errorf("unmapped_count = %d, want 0", resp.UnmappedCount)
	}
	if resp.SkippedCount != 0 {
		t.Errorf("skipped_count = %d, want 0", resp.SkippedCount)
	}
}

func TestHandlePlatformMappingStatus_SomeUnmapped(t *testing.T) {
	tkCfg := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ubuntu-tmpl"},
		},
	}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{
				{PlatformName: "ubuntu-22.04", NormalisedName: "ubuntu-22.04", OSFamily: "debian", CookbookCount: 50},
				{PlatformName: "centos-7", NormalisedName: "centos-7", OSFamily: "rhel", CookbookCount: 30},
				{PlatformName: "windows-2022", NormalisedName: "windows-2022", OSFamily: "windows", CookbookCount: 10},
			}, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)
	if resp.MappedCount != 1 {
		t.Errorf("mapped_count = %d, want 1", resp.MappedCount)
	}
	if resp.UnmappedCount != 2 {
		t.Errorf("unmapped_count = %d, want 2", resp.UnmappedCount)
	}

	// Verify specific statuses.
	for _, dp := range resp.DiscoveredPlatforms {
		switch dp.PlatformName {
		case "ubuntu-22.04":
			if dp.MappingStatus != "mapped" {
				t.Errorf("ubuntu-22.04 status = %q, want %q", dp.MappingStatus, "mapped")
			}
			if dp.MatchedImage != "ubuntu-tmpl" {
				t.Errorf("ubuntu-22.04 matched_image = %q, want %q", dp.MatchedImage, "ubuntu-tmpl")
			}
		case "centos-7", "windows-2022":
			if dp.MappingStatus != "unmapped" {
				t.Errorf("%s status = %q, want %q", dp.PlatformName, dp.MappingStatus, "unmapped")
			}
			if dp.MatchedEntryIndex != -1 {
				t.Errorf("%s matched_entry_index = %d, want -1", dp.PlatformName, dp.MatchedEntryIndex)
			}
			if dp.MatchedImage != "" {
				t.Errorf("%s matched_image = %q, want empty", dp.PlatformName, dp.MatchedImage)
			}
		}
	}
}

func TestHandlePlatformMappingStatus_WithSkip(t *testing.T) {
	tkCfg := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ubuntu-tmpl"},
			{KitchenName: "windows-2022", Skip: true},
		},
	}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{
				{PlatformName: "ubuntu-22.04", NormalisedName: "ubuntu-22.04", OSFamily: "debian", CookbookCount: 50},
				{PlatformName: "windows-2022", NormalisedName: "windows-2022", OSFamily: "windows", CookbookCount: 10},
				{PlatformName: "centos-7", NormalisedName: "centos-7", OSFamily: "rhel", CookbookCount: 20},
			}, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)
	if resp.SkippedCount != 1 {
		t.Errorf("skipped_count = %d, want 1", resp.SkippedCount)
	}
	if resp.MappedCount != 1 {
		t.Errorf("mapped_count = %d, want 1", resp.MappedCount)
	}
	if resp.UnmappedCount != 1 {
		t.Errorf("unmapped_count = %d, want 1", resp.UnmappedCount)
	}

	for _, dp := range resp.DiscoveredPlatforms {
		if dp.PlatformName == "windows-2022" {
			if dp.MappingStatus != "skipped" {
				t.Errorf("windows-2022 status = %q, want %q", dp.MappingStatus, "skipped")
			}
			if dp.MatchedImage != "" {
				t.Errorf("windows-2022 matched_image = %q, want empty", dp.MatchedImage)
			}
		}
	}
}

func TestHandlePlatformMappingStatus_PatternMatch(t *testing.T) {
	tkCfg := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "rhel*", Image: "rhel-tmpl", IsPattern: true},
		},
	}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{
				{PlatformName: "rhel7-chef16", NormalisedName: "rhel7-chef16", OSFamily: "rhel", CookbookCount: 25},
				{PlatformName: "rhel8", NormalisedName: "rhel8", OSFamily: "rhel", CookbookCount: 40},
				{PlatformName: "ubuntu-22.04", NormalisedName: "ubuntu-22.04", OSFamily: "debian", CookbookCount: 10},
			}, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)
	if resp.MappedCount != 2 {
		t.Errorf("mapped_count = %d, want 2", resp.MappedCount)
	}
	if resp.UnmappedCount != 1 {
		t.Errorf("unmapped_count = %d, want 1", resp.UnmappedCount)
	}

	for _, dp := range resp.DiscoveredPlatforms {
		switch dp.PlatformName {
		case "rhel7-chef16", "rhel8":
			if dp.MappingStatus != "mapped" {
				t.Errorf("%s status = %q, want %q", dp.PlatformName, dp.MappingStatus, "mapped")
			}
			if dp.MatchedImage != "rhel-tmpl" {
				t.Errorf("%s matched_image = %q, want %q", dp.PlatformName, dp.MatchedImage, "rhel-tmpl")
			}
			if dp.MatchedEntryIndex != 0 {
				t.Errorf("%s matched_entry_index = %d, want 0", dp.PlatformName, dp.MatchedEntryIndex)
			}
		case "ubuntu-22.04":
			if dp.MappingStatus != "unmapped" {
				t.Errorf("ubuntu-22.04 status = %q, want %q", dp.MappingStatus, "unmapped")
			}
		}
	}
}

func TestHandlePlatformMappingStatus_NoDiscoveredPlatforms(t *testing.T) {
	tkCfg := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ubuntu-tmpl"},
		},
	}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{}, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Capture raw bytes before decoding consumes the body.
	raw := make([]byte, rec.Body.Len())
	copy(raw, rec.Body.Bytes())

	resp := decodePlatformMappingResponse(t, rec)
	if resp.DiscoveredPlatforms == nil {
		t.Fatal("discovered_platforms should not be nil")
	}
	if len(resp.DiscoveredPlatforms) != 0 {
		t.Errorf("discovered_platforms count = %d, want 0", len(resp.DiscoveredPlatforms))
	}
	if resp.MappedCount != 0 {
		t.Errorf("mapped_count = %d, want 0", resp.MappedCount)
	}
	if resp.UnmappedCount != 0 {
		t.Errorf("unmapped_count = %d, want 0", resp.UnmappedCount)
	}
	if resp.SkippedCount != 0 {
		t.Errorf("skipped_count = %d, want 0", resp.SkippedCount)
	}

	// Verify JSON has [] not null.
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if string(rawMap["discovered_platforms"]) == "null" {
		t.Error("discovered_platforms serialised as null, want []")
	}
}

func TestHandlePlatformMappingStatus_NoConfig(t *testing.T) {
	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return nil, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{
				{PlatformName: "ubuntu-22.04", NormalisedName: "ubuntu-22.04", OSFamily: "debian", CookbookCount: 50},
				{PlatformName: "centos-7", NormalisedName: "centos-7", OSFamily: "rhel", CookbookCount: 30},
			}, nil
		},
	}
	// File config has empty platform_map by default via testConfig().
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)
	if resp.UnmappedCount != 2 {
		t.Errorf("unmapped_count = %d, want 2", resp.UnmappedCount)
	}
	if resp.MappedCount != 0 {
		t.Errorf("mapped_count = %d, want 0", resp.MappedCount)
	}
	for i, dp := range resp.DiscoveredPlatforms {
		if dp.MappingStatus != "unmapped" {
			t.Errorf("discovered_platforms[%d].mapping_status = %q, want %q", i, dp.MappingStatus, "unmapped")
		}
	}
}

func TestHandlePlatformMappingStatus_WithTemplates(t *testing.T) {
	now := time.Now().UTC()
	tkCfg := config.TestKitchenConfig{}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return nil, nil
		},
	}
	hyp := &mockHypervisorClient{
		templates: []hypervisor.Template{
			{ID: "tmpl-1", Name: "ubuntu-22.04-template", GuestOS: "ubuntu64Guest", LastModified: now},
			{ID: "tmpl-2", Name: "rhel-9-template", GuestOS: "rhel9_64Guest", LastModified: now},
		},
	}
	r := newTestRouterWithHypervisor(store, hyp)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)
	if len(resp.Templates) != 2 {
		t.Fatalf("templates count = %d, want 2", len(resp.Templates))
	}
	if resp.Templates[0].Name != "ubuntu-22.04-template" {
		t.Errorf("templates[0].name = %q, want %q", resp.Templates[0].Name, "ubuntu-22.04-template")
	}
	if resp.Templates[1].Name != "rhel-9-template" {
		t.Errorf("templates[1].name = %q, want %q", resp.Templates[1].Name, "rhel-9-template")
	}
}

func TestHandlePlatformMappingStatus_NoHypervisor(t *testing.T) {
	tkCfg := config.TestKitchenConfig{}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Capture raw bytes before decoding consumes the body.
	raw := make([]byte, rec.Body.Len())
	copy(raw, rec.Body.Bytes())

	resp := decodePlatformMappingResponse(t, rec)
	if resp.Templates == nil {
		t.Fatal("templates should not be nil")
	}
	if len(resp.Templates) != 0 {
		t.Errorf("templates count = %d, want 0", len(resp.Templates))
	}

	// Verify JSON has [] not null.
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if string(rawMap["templates"]) == "null" {
		t.Error("templates serialised as null, want []")
	}
}

func TestHandlePlatformMappingStatus_MethodNotAllowed(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlePlatformMappingStatus_DBError(t *testing.T) {
	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return nil, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeInternalError {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeInternalError)
	}
}

func TestHandlePlatformMappingStatus_NodePlatforms(t *testing.T) {
	tkCfg := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ubuntu-tmpl"},
		},
	}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{
				{PlatformName: "ubuntu-22.04", NormalisedName: "ubuntu-22.04", OSFamily: "debian", CookbookCount: 50},
			}, nil
		},
		CountNodePlatformDistributionFn: func(_ context.Context, _ datastore.NodeSnapshotFilter) (map[string]int, int, error) {
			return map[string]int{
				"almalinux 10.1": 15,
				"windows 2022":   8,
			}, 23, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)

	// 1 kitchen + 2 node-only = 3 total
	if len(resp.DiscoveredPlatforms) != 3 {
		t.Fatalf("discovered_platforms count = %d, want 3", len(resp.DiscoveredPlatforms))
	}

	byName := make(map[string]DiscoveredPlatformStatus)
	for _, dp := range resp.DiscoveredPlatforms {
		byName[dp.PlatformName] = dp
	}

	// Kitchen-only platform.
	ub := byName["ubuntu-22.04"]
	if ub.Source != "kitchen" {
		t.Errorf("ubuntu-22.04 source = %q, want %q", ub.Source, "kitchen")
	}
	if ub.MappingStatus != "mapped" {
		t.Errorf("ubuntu-22.04 status = %q, want %q", ub.MappingStatus, "mapped")
	}

	// Node-only platforms.
	alma := byName["almalinux 10.1"]
	if alma.Source != "nodes" {
		t.Errorf("almalinux 10.1 source = %q, want %q", alma.Source, "nodes")
	}
	if alma.NodeCount != 15 {
		t.Errorf("almalinux 10.1 node_count = %d, want 15", alma.NodeCount)
	}
	if alma.MappingStatus != "unmapped" {
		t.Errorf("almalinux 10.1 status = %q, want %q", alma.MappingStatus, "unmapped")
	}

	win := byName["windows 2022"]
	if win.Source != "nodes" {
		t.Errorf("windows 2022 source = %q, want %q", win.Source, "nodes")
	}
	if win.NodeCount != 8 {
		t.Errorf("windows 2022 node_count = %d, want 8", win.NodeCount)
	}
}

func TestHandlePlatformMappingStatus_NodeAndKitchenOverlap(t *testing.T) {
	tkCfg := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ubuntu-tmpl"},
		},
	}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return []datastore.KitchenDiscoveredPlatform{
				{PlatformName: "ubuntu-22.04", NormalisedName: "ubuntu-22.04", OSFamily: "debian", CookbookCount: 50},
			}, nil
		},
		CountNodePlatformDistributionFn: func(_ context.Context, _ datastore.NodeSnapshotFilter) (map[string]int, int, error) {
			// Same name as a kitchen platform — should merge, not duplicate.
			return map[string]int{
				"ubuntu-22.04": 20,
				"centos 7":     5,
			}, 25, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)

	// 1 merged (ubuntu-22.04) + 1 node-only (centos 7) = 2
	if len(resp.DiscoveredPlatforms) != 2 {
		t.Fatalf("discovered_platforms count = %d, want 2", len(resp.DiscoveredPlatforms))
	}

	byName := make(map[string]DiscoveredPlatformStatus)
	for _, dp := range resp.DiscoveredPlatforms {
		byName[dp.PlatformName] = dp
	}

	ub := byName["ubuntu-22.04"]
	if ub.Source != "both" {
		t.Errorf("ubuntu-22.04 source = %q, want %q", ub.Source, "both")
	}
	if ub.NodeCount != 20 {
		t.Errorf("ubuntu-22.04 node_count = %d, want 20", ub.NodeCount)
	}
	if ub.CookbookCount != 50 {
		t.Errorf("ubuntu-22.04 cookbook_count = %d, want 50", ub.CookbookCount)
	}
	if ub.MappingStatus != "mapped" {
		t.Errorf("ubuntu-22.04 status = %q, want %q", ub.MappingStatus, "mapped")
	}

	centos := byName["centos 7"]
	if centos.Source != "nodes" {
		t.Errorf("centos 7 source = %q, want %q", centos.Source, "nodes")
	}
	if centos.NodeCount != 5 {
		t.Errorf("centos 7 node_count = %d, want 5", centos.NodeCount)
	}
}

func TestHandlePlatformMappingStatus_NodePlatformsMapped(t *testing.T) {
	tkCfg := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "almalinux 10.1", Image: "alma-tmpl"},
			{KitchenName: "windows 2022", Skip: true},
		},
	}
	cfgBytes := tkConfigJSON(t, tkCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			return &datastore.RuntimeSetting{Key: key, Value: cfgBytes}, nil
		},
		ListDiscoveredPlatformsFn: func(_ context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
			return nil, nil
		},
		CountNodePlatformDistributionFn: func(_ context.Context, _ datastore.NodeSnapshotFilter) (map[string]int, int, error) {
			return map[string]int{
				"almalinux 10.1": 15,
				"windows 2022":   8,
			}, 23, nil
		},
	}
	r := newTestRouterWithHypervisor(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-mapping/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodePlatformMappingResponse(t, rec)
	if resp.MappedCount != 1 {
		t.Errorf("mapped_count = %d, want 1", resp.MappedCount)
	}
	if resp.SkippedCount != 1 {
		t.Errorf("skipped_count = %d, want 1", resp.SkippedCount)
	}
	if resp.UnmappedCount != 0 {
		t.Errorf("unmapped_count = %d, want 0", resp.UnmappedCount)
	}

	byName := make(map[string]DiscoveredPlatformStatus)
	for _, dp := range resp.DiscoveredPlatforms {
		byName[dp.PlatformName] = dp
	}

	alma := byName["almalinux 10.1"]
	if alma.MappingStatus != "mapped" {
		t.Errorf("almalinux 10.1 status = %q, want %q", alma.MappingStatus, "mapped")
	}
	if alma.MatchedImage != "alma-tmpl" {
		t.Errorf("almalinux 10.1 matched_image = %q, want %q", alma.MatchedImage, "alma-tmpl")
	}

	win := byName["windows 2022"]
	if win.MappingStatus != "skipped" {
		t.Errorf("windows 2022 status = %q, want %q", win.MappingStatus, "skipped")
	}
}
