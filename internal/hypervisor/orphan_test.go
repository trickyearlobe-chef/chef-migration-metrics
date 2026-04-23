// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Mock store
// ---------------------------------------------------------------------------

type mockOrphanStore struct {
	orphanedVMs     []datastore.TrackedVM
	markedOrphaned  []string
	markedDestroyed []string
	listErr         error
	markOrphanErr   error
	markDestroyErr  error
}

func (m *mockOrphanStore) ListOrphanedVMs(_ context.Context) ([]datastore.TrackedVM, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.orphanedVMs, nil
}

func (m *mockOrphanStore) MarkVMOrphaned(_ context.Context, id string) error {
	if m.markOrphanErr != nil {
		return m.markOrphanErr
	}
	m.markedOrphaned = append(m.markedOrphaned, id)
	return nil
}

func (m *mockOrphanStore) MarkVMDestroyed(_ context.Context, id string) error {
	if m.markDestroyErr != nil {
		return m.markDestroyErr
	}
	m.markedDestroyed = append(m.markedDestroyed, id)
	return nil
}

// ---------------------------------------------------------------------------
// Mock hypervisor
// ---------------------------------------------------------------------------

type mockHypervisor struct {
	templates  []Template
	managedVMs []ManagedVM
	destroyErr error
	destroyed  []string
	// destroyErrIDs allows per-ID error simulation.
	destroyErrIDs map[string]error
}

func (m *mockHypervisor) ListTemplates(_ context.Context) ([]Template, error) {
	return m.templates, nil
}

func (m *mockHypervisor) ListManagedVMs(_ context.Context, _ string) ([]ManagedVM, error) {
	return m.managedVMs, nil
}

func (m *mockHypervisor) DestroyVM(_ context.Context, id string) error {
	if m.destroyErrIDs != nil {
		if err, ok := m.destroyErrIDs[id]; ok {
			return err
		}
	}
	if m.destroyErr != nil {
		return m.destroyErr
	}
	m.destroyed = append(m.destroyed, id)
	return nil
}

func (m *mockHypervisor) Type() string { return "mock" }

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func expiredVM(id, name, hypID, status string) datastore.TrackedVM {
	return datastore.TrackedVM{
		ID:                id,
		VMName:            name,
		HypervisorID:      hypID,
		Status:            status,
		CreatedAt:         time.Now().Add(-2 * time.Hour),
		ExpectedDestroyAt: time.Now().Add(-1 * time.Hour),
	}
}

// ---------------------------------------------------------------------------
// DetectOrphans tests
// ---------------------------------------------------------------------------

func TestDetectOrphans_FindsExpired(t *testing.T) {
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "hyp-1", "running"),
			expiredVM("vm-2", "cmm-cb2-suite-centos-2222222222", "hyp-2", "creating"),
		},
	}
	hyp := &mockHypervisor{}

	result, err := DetectOrphans(context.Background(), store, hyp, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected != 2 {
		t.Errorf("Detected = %d, want 2", result.Detected)
	}
	if len(store.markedOrphaned) != 2 {
		t.Errorf("markedOrphaned count = %d, want 2", len(store.markedOrphaned))
	}
	if store.markedOrphaned[0] != "vm-1" || store.markedOrphaned[1] != "vm-2" {
		t.Errorf("markedOrphaned = %v, want [vm-1 vm-2]", store.markedOrphaned)
	}
}

func TestDetectOrphans_NoOrphans(t *testing.T) {
	store := &mockOrphanStore{}
	hyp := &mockHypervisor{}

	result, err := DetectOrphans(context.Background(), store, hyp, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected != 0 {
		t.Errorf("Detected = %d, want 0", result.Detected)
	}
	if len(result.Details) != 0 {
		t.Errorf("Details = %v, want empty", result.Details)
	}
}

func TestDetectOrphans_NilHypervisor(t *testing.T) {
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "hyp-1", "running"),
		},
	}

	result, err := DetectOrphans(context.Background(), store, nil, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected != 1 {
		t.Errorf("Detected = %d, want 1", result.Detected)
	}
	if len(store.markedOrphaned) != 1 {
		t.Errorf("markedOrphaned count = %d, want 1", len(store.markedOrphaned))
	}
}

func TestDetectOrphans_NullHypervisor(t *testing.T) {
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "hyp-1", "running"),
		},
	}

	result, err := DetectOrphans(context.Background(), store, NullHypervisor{}, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected != 1 {
		t.Errorf("Detected = %d, want 1", result.Detected)
	}
}

func TestDetectOrphans_CrossReferencesLiveVMs(t *testing.T) {
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "hyp-1", "running"),
		},
	}
	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{
			{HypervisorID: "hyp-1", Name: "cmm-cb1-suite-ubuntu-1111111111", PowerState: "poweredOn"},
		},
	}

	result, err := DetectOrphans(context.Background(), store, hyp, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Detected != 1 {
		t.Errorf("Detected = %d, want 1", result.Detected)
	}
	// Should contain a detail noting the VM is still running.
	found := false
	for _, d := range result.Details {
		if len(d) > 0 && contains(d, "still running") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected detail about VM still running, got: %v", result.Details)
	}
}

func TestDetectOrphans_StoreError(t *testing.T) {
	store := &mockOrphanStore{
		listErr: fmt.Errorf("db gone"),
	}

	_, err := DetectOrphans(context.Background(), store, nil, "cmm")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// CleanupOrphans tests
// ---------------------------------------------------------------------------

func TestCleanupOrphans_Success(t *testing.T) {
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "hyp-1", "orphaned"),
			expiredVM("vm-2", "cmm-cb2-suite-centos-2222222222", "hyp-2", "orphaned"),
		},
	}
	hyp := &mockHypervisor{}

	result, err := CleanupOrphans(context.Background(), store, hyp, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Destroyed != 2 {
		t.Errorf("Destroyed = %d, want 2", result.Destroyed)
	}
	if result.Errors != 0 {
		t.Errorf("Errors = %d, want 0", result.Errors)
	}
	if len(hyp.destroyed) != 2 {
		t.Errorf("hypervisor destroyed count = %d, want 2", len(hyp.destroyed))
	}
}

func TestCleanupOrphans_PartialFailure(t *testing.T) {
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "hyp-1", "orphaned"),
			expiredVM("vm-2", "cmm-cb2-suite-centos-2222222222", "hyp-2", "orphaned"),
		},
	}
	hyp := &mockHypervisor{
		destroyErrIDs: map[string]error{
			"hyp-2": fmt.Errorf("permission denied"),
		},
	}

	result, err := CleanupOrphans(context.Background(), store, hyp, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Destroyed != 1 {
		t.Errorf("Destroyed = %d, want 1", result.Destroyed)
	}
	if result.Errors < 1 {
		t.Errorf("Errors = %d, want >= 1", result.Errors)
	}
}

func TestCleanupOrphans_NoHypervisor(t *testing.T) {
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "hyp-1", "running"),
		},
	}

	result, err := CleanupOrphans(context.Background(), store, NullHypervisor{}, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Detection should still happen.
	if result.Detected != 1 {
		t.Errorf("Detected = %d, want 1", result.Detected)
	}
	// No destruction without a real hypervisor.
	if result.Destroyed != 0 {
		t.Errorf("Destroyed = %d, want 0", result.Destroyed)
	}
	// Should note that no hypervisor is configured.
	found := false
	for _, d := range result.Details {
		if contains(d, "no hypervisor configured") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected detail about no hypervisor, got: %v", result.Details)
	}
}

func TestCleanupOrphans_NilHypervisor(t *testing.T) {
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "", "running"),
		},
	}

	result, err := CleanupOrphans(context.Background(), store, nil, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Destroyed != 0 {
		t.Errorf("Destroyed = %d, want 0", result.Destroyed)
	}
}

func TestCleanupOrphans_NoHypervisorID(t *testing.T) {
	// VMs with no hypervisor_id should be marked destroyed without calling
	// DestroyVM on the hypervisor.
	store := &mockOrphanStore{
		orphanedVMs: []datastore.TrackedVM{
			expiredVM("vm-1", "cmm-cb1-suite-ubuntu-1111111111", "", "orphaned"),
		},
	}
	hyp := &mockHypervisor{}

	result, err := CleanupOrphans(context.Background(), store, hyp, "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Destroyed != 1 {
		t.Errorf("Destroyed = %d, want 1", result.Destroyed)
	}
	if len(hyp.destroyed) != 0 {
		t.Errorf("hypervisor destroyed count = %d, want 0 (no hypervisor ID)", len(hyp.destroyed))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
