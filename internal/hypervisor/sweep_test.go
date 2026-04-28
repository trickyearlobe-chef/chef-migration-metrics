// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// SweepOrphanVMs tests
// ---------------------------------------------------------------------------

func TestSweepOrphanVMs_MixedVMs(t *testing.T) {
	now := time.Now()
	oldTS := now.Add(-2 * time.Hour).Unix()
	youngTS := now.Add(-5 * time.Minute).Unix()

	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{
			{HypervisorID: "vm-old-1", Name: fmt.Sprintf("cmm-cookbook-suite-ubuntu-%d", oldTS)},
			{HypervisorID: "vm-old-2", Name: fmt.Sprintf("cmm-cookbook-suite-centos-%d", oldTS)},
			{HypervisorID: "vm-young", Name: fmt.Sprintf("cmm-cookbook-suite-rhel-%d", youngTS)},
			{HypervisorID: "vm-bad", Name: "not-a-valid-vm-name"},
		},
	}

	result, err := SweepOrphanVMs(context.Background(), hyp, "cmm", 1*time.Hour, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Scanned != 4 {
		t.Errorf("Scanned = %d, want 4", result.Scanned)
	}
	if result.Destroyed != 2 {
		t.Errorf("Destroyed = %d, want 2", result.Destroyed)
	}
	if result.SkippedTooYoung != 1 {
		t.Errorf("SkippedTooYoung = %d, want 1", result.SkippedTooYoung)
	}
	if result.SkippedUnparsed != 1 {
		t.Errorf("SkippedUnparsed = %d, want 1", result.SkippedUnparsed)
	}
	if result.Errors != 0 {
		t.Errorf("Errors = %d, want 0", result.Errors)
	}
	if result.DryRun {
		t.Error("DryRun = true, want false")
	}

	if len(hyp.destroyed) != 2 {
		t.Fatalf("hypervisor destroyed count = %d, want 2", len(hyp.destroyed))
	}
	if hyp.destroyed[0] != "vm-old-1" || hyp.destroyed[1] != "vm-old-2" {
		t.Errorf("destroyed = %v, want [vm-old-1 vm-old-2]", hyp.destroyed)
	}
}

func TestSweepOrphanVMs_DryRun(t *testing.T) {
	now := time.Now()
	oldTS := now.Add(-2 * time.Hour).Unix()

	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{
			{HypervisorID: "vm-old", Name: fmt.Sprintf("cmm-cookbook-suite-ubuntu-%d", oldTS)},
		},
	}

	result, err := SweepOrphanVMs(context.Background(), hyp, "cmm", 1*time.Hour, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.DryRun {
		t.Error("DryRun = false, want true")
	}
	if result.Destroyed != 0 {
		t.Errorf("Destroyed = %d, want 0 in dry run", result.Destroyed)
	}
	if len(hyp.destroyed) != 0 {
		t.Errorf("hypervisor.destroyed count = %d, want 0 in dry run", len(hyp.destroyed))
	}

	// Should have a detail entry with action "would_destroy".
	if len(result.Details) != 1 {
		t.Fatalf("Details count = %d, want 1", len(result.Details))
	}
	if result.Details[0].Action != "would_destroy" {
		t.Errorf("Details[0].Action = %q, want %q", result.Details[0].Action, "would_destroy")
	}
}

func TestSweepOrphanVMs_DestroyErrors(t *testing.T) {
	now := time.Now()
	oldTS := now.Add(-2 * time.Hour).Unix()

	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{
			{HypervisorID: "vm-1", Name: fmt.Sprintf("cmm-cookbook-suite-ubuntu-%d", oldTS)},
			{HypervisorID: "vm-2", Name: fmt.Sprintf("cmm-cookbook-suite-centos-%d", oldTS)},
		},
		destroyErrIDs: map[string]error{
			"vm-2": fmt.Errorf("permission denied"),
		},
	}

	result, err := SweepOrphanVMs(context.Background(), hyp, "cmm", 1*time.Hour, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Destroyed != 1 {
		t.Errorf("Destroyed = %d, want 1", result.Destroyed)
	}
	if result.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Errors)
	}

	// Check error detail.
	var foundErr bool
	for _, d := range result.Details {
		if d.Action == "error" && d.HypervisorID == "vm-2" {
			foundErr = true
			if d.Error == "" {
				t.Error("expected non-empty Error for failed destroy")
			}
		}
	}
	if !foundErr {
		t.Error("expected error detail for vm-2")
	}
}

func TestSweepOrphanVMs_EmptyList(t *testing.T) {
	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{},
	}

	result, err := SweepOrphanVMs(context.Background(), hyp, "cmm", 1*time.Hour, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Scanned != 0 {
		t.Errorf("Scanned = %d, want 0", result.Scanned)
	}
	if result.Destroyed != 0 {
		t.Errorf("Destroyed = %d, want 0", result.Destroyed)
	}
	if len(result.Details) != 0 {
		t.Errorf("Details count = %d, want 0", len(result.Details))
	}
}

func TestSweepOrphanVMs_ListError(t *testing.T) {
	hyp := &mockHypervisor{
		managedVMs: nil,
	}
	// Override ListManagedVMs to return error — use a custom mock.
	errHyp := &errListHypervisor{listErr: fmt.Errorf("connection refused")}

	_, err := SweepOrphanVMs(context.Background(), errHyp, "cmm", 1*time.Hour, false)
	if err == nil {
		t.Fatal("expected error from ListManagedVMs, got nil")
	}
	_ = hyp
}

func TestSweepOrphanVMs_AllYoung(t *testing.T) {
	now := time.Now()
	youngTS := now.Add(-2 * time.Minute).Unix()

	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{
			{HypervisorID: "vm-1", Name: fmt.Sprintf("cmm-cookbook-suite-ubuntu-%d", youngTS)},
			{HypervisorID: "vm-2", Name: fmt.Sprintf("cmm-cookbook-suite-centos-%d", youngTS)},
		},
	}

	result, err := SweepOrphanVMs(context.Background(), hyp, "cmm", 1*time.Hour, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SkippedTooYoung != 2 {
		t.Errorf("SkippedTooYoung = %d, want 2", result.SkippedTooYoung)
	}
	if result.Destroyed != 0 {
		t.Errorf("Destroyed = %d, want 0", result.Destroyed)
	}
}

// errListHypervisor is a mock that returns an error on ListManagedVMs.
type errListHypervisor struct {
	listErr error
}

func (e *errListHypervisor) ListTemplates(_ context.Context) ([]Template, error) {
	return nil, nil
}

func (e *errListHypervisor) ListManagedVMs(_ context.Context, _ string) ([]ManagedVM, error) {
	return nil, e.listErr
}

func (e *errListHypervisor) DestroyVM(_ context.Context, _ string) error {
	return nil
}

func (e *errListHypervisor) Type() string { return "mock-err" }
