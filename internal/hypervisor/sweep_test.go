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

func TestSweepOrphanVMs_UptimeFallback_KitchenVMs(t *testing.T) {
	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{
			// Old kitchen VM — uptime exceeds threshold (orphan).
			{HypervisorID: "146", Name: "kitchen-config-amazonlinux-2-efebd23e", Uptime: 3 * time.Hour},
			// Young kitchen VM — uptime below threshold (active run).
			{HypervisorID: "113", Name: "kitchen-cron-resource-fedora-latest-c4fd114d", Uptime: 5 * time.Minute},
			// Non-kitchen, non-CMM VM — should be skipped as unparsed.
			{HypervisorID: "108", Name: "homeassistant", Uptime: 24 * time.Hour},
		},
	}

	result, err := SweepOrphanVMs(context.Background(), hyp, "cmm", 1*time.Hour, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3", result.Scanned)
	}
	if result.SkippedTooYoung != 1 {
		t.Errorf("SkippedTooYoung = %d, want 1 (young kitchen VM)", result.SkippedTooYoung)
	}
	if result.SkippedUnparsed != 1 {
		t.Errorf("SkippedUnparsed = %d, want 1 (homeassistant)", result.SkippedUnparsed)
	}

	// Dry run — nothing destroyed, but old kitchen VM flagged.
	var wouldDestroy int
	for _, d := range result.Details {
		if d.Action == "would_destroy" {
			wouldDestroy++
			if d.VMName != "kitchen-config-amazonlinux-2-efebd23e" {
				t.Errorf("expected would_destroy for kitchen-config-amazonlinux-2-efebd23e, got %s", d.VMName)
			}
		}
	}
	if wouldDestroy != 1 {
		t.Errorf("would_destroy count = %d, want 1", wouldDestroy)
	}
}

func TestSweepOrphanVMs_UptimeFallback_PoweredOff(t *testing.T) {
	// Powered off kitchen VM has zero uptime — should be swept (stopped = orphaned).
	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{
			{HypervisorID: "146", Name: "kitchen-config-amazonlinux-2-efebd23e", Uptime: 0, PowerState: "poweredOff"},
		},
	}

	result, err := SweepOrphanVMs(context.Background(), hyp, "cmm", 1*time.Hour, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Scanned != 1 {
		t.Errorf("Scanned = %d, want 1", result.Scanned)
	}
	if len(result.Details) != 1 || result.Details[0].Action != "would_destroy" {
		t.Errorf("expected would_destroy for powered-off kitchen VM, got %+v", result.Details)
	}
}

func TestSweepOrphanVMs_KitchenVM_ZeroUptime_Running(t *testing.T) {
	// A running kitchen VM with zero uptime (just booted) — age unknown, skip.
	hyp := &mockHypervisor{
		managedVMs: []ManagedVM{
			{HypervisorID: "113", Name: "kitchen-cron-fedora-c4fd114d", Uptime: 0, PowerState: "poweredOn"},
		},
	}

	result, err := SweepOrphanVMs(context.Background(), hyp, "cmm", 1*time.Hour, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SkippedUnparsed != 1 {
		t.Errorf("SkippedUnparsed = %d, want 1 (running + zero uptime = unknown)", result.SkippedUnparsed)
	}
}

func TestVmAge_CMM_Named(t *testing.T) {
	now := time.Now()
	ts := now.Add(-90 * time.Minute).Unix()
	vm := ManagedVM{Name: fmt.Sprintf("cmm-cookbook-suite-ubuntu-%d", ts)}

	age, ok := vmAge(vm, "cmm", now.Unix())
	if !ok {
		t.Fatal("expected age to be known for CMM-named VM")
	}
	// Allow 1s tolerance for test execution time.
	if age < 89*time.Minute || age > 91*time.Minute {
		t.Errorf("age = %v, want ~90m", age)
	}
}

func TestVmAge_KitchenFallback(t *testing.T) {
	vm := ManagedVM{Name: "kitchen-test-ubuntu-abcd1234", Uptime: 2 * time.Hour}

	age, ok := vmAge(vm, "cmm", time.Now().Unix())
	if !ok {
		t.Fatal("expected age to be known for kitchen-* VM with uptime")
	}
	if age != 2*time.Hour {
		t.Errorf("age = %v, want 2h", age)
	}
}

func TestVmAge_Unknown(t *testing.T) {
	vm := ManagedVM{Name: "nexus", Uptime: 24 * time.Hour}

	_, ok := vmAge(vm, "cmm", time.Now().Unix())
	if ok {
		t.Error("expected age to be unknown for non-kitchen, non-CMM VM")
	}
}
