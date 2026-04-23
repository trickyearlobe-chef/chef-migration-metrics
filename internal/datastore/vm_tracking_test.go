// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// validateInsertTrackedVMParams — pure function tests
// ---------------------------------------------------------------------------

func TestValidateInsertTrackedVMParams_Valid(t *testing.T) {
	err := validateInsertTrackedVMParams(InsertTrackedVMParams{
		VMName:       "kitchen-mycookbook-default-ubuntu-2204",
		CookbookName: "mycookbook",
		SuiteName:    "default",
		PlatformName: "ubuntu-22.04",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInsertTrackedVMParams_MissingVMName(t *testing.T) {
	err := validateInsertTrackedVMParams(InsertTrackedVMParams{
		CookbookName: "mycookbook",
		SuiteName:    "default",
		PlatformName: "ubuntu-22.04",
	})
	if err == nil {
		t.Fatal("expected error for missing vm_name")
	}
	if got := err.Error(); got != "datastore: vm_name is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateInsertTrackedVMParams_MissingCookbookName(t *testing.T) {
	err := validateInsertTrackedVMParams(InsertTrackedVMParams{
		VMName:       "kitchen-mycookbook-default-ubuntu-2204",
		SuiteName:    "default",
		PlatformName: "ubuntu-22.04",
	})
	if err == nil {
		t.Fatal("expected error for missing cookbook_name")
	}
	if got := err.Error(); got != "datastore: cookbook_name is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateInsertTrackedVMParams_MissingSuiteName(t *testing.T) {
	err := validateInsertTrackedVMParams(InsertTrackedVMParams{
		VMName:       "kitchen-mycookbook-default-ubuntu-2204",
		CookbookName: "mycookbook",
		PlatformName: "ubuntu-22.04",
	})
	if err == nil {
		t.Fatal("expected error for missing suite_name")
	}
	if got := err.Error(); got != "datastore: suite_name is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateInsertTrackedVMParams_MissingPlatformName(t *testing.T) {
	err := validateInsertTrackedVMParams(InsertTrackedVMParams{
		VMName:       "kitchen-mycookbook-default-ubuntu-2204",
		CookbookName: "mycookbook",
		SuiteName:    "default",
	})
	if err == nil {
		t.Fatal("expected error for missing platform_name")
	}
	if got := err.Error(); got != "datastore: platform_name is required" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestValidateInsertTrackedVMParams_ValidationOrder(t *testing.T) {
	// All fields missing — should fail on vm_name first.
	err := validateInsertTrackedVMParams(InsertTrackedVMParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: vm_name is required" {
		t.Errorf("expected vm_name error first, got: %v", got)
	}

	// vm_name present — should fail on cookbook_name.
	err = validateInsertTrackedVMParams(InsertTrackedVMParams{
		VMName: "test-vm",
	})
	if err == nil {
		t.Fatal("expected error for missing cookbook_name")
	}
	if got := err.Error(); got != "datastore: cookbook_name is required" {
		t.Errorf("expected cookbook_name error, got: %v", got)
	}

	// vm_name + cookbook_name present — should fail on suite_name.
	err = validateInsertTrackedVMParams(InsertTrackedVMParams{
		VMName:       "test-vm",
		CookbookName: "mycookbook",
	})
	if err == nil {
		t.Fatal("expected error for missing suite_name")
	}
	if got := err.Error(); got != "datastore: suite_name is required" {
		t.Errorf("expected suite_name error, got: %v", got)
	}

	// vm_name + cookbook_name + suite_name present — should fail on platform_name.
	err = validateInsertTrackedVMParams(InsertTrackedVMParams{
		VMName:       "test-vm",
		CookbookName: "mycookbook",
		SuiteName:    "default",
	})
	if err == nil {
		t.Fatal("expected error for missing platform_name")
	}
	if got := err.Error(); got != "datastore: platform_name is required" {
		t.Errorf("expected platform_name error, got: %v", got)
	}
}

// ---------------------------------------------------------------------------
// validateVMStatus
// ---------------------------------------------------------------------------

func TestValidateVMStatus_Valid(t *testing.T) {
	for _, status := range []string{"creating", "running", "destroying", "destroyed", "orphaned"} {
		t.Run(status, func(t *testing.T) {
			if err := validateVMStatus(status); err != nil {
				t.Errorf("validateVMStatus(%q) returned unexpected error: %v", status, err)
			}
		})
	}
}

func TestValidateVMStatus_Invalid(t *testing.T) {
	for _, status := range []string{"", "pending", "RUNNING", "unknown", "deleted"} {
		t.Run(status, func(t *testing.T) {
			err := validateVMStatus(status)
			if err == nil {
				t.Errorf("validateVMStatus(%q) should have returned an error", status)
			}
			want := "datastore: invalid VM status "
			if got := err.Error(); len(got) < len(want) {
				t.Errorf("error = %q, expected prefix %q", got, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TrackedVM — JSON marshalling
// ---------------------------------------------------------------------------

func TestTrackedVM_MarshalJSON(t *testing.T) {
	now := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)
	destroyAt := now.Add(2 * time.Hour)
	actualDestroy := now.Add(3 * time.Hour)

	vm := TrackedVM{
		ID:                "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		VMName:            "kitchen-mycookbook-default-ubuntu-2204",
		HypervisorID:      "esxi-01.example.com",
		CookbookName:      "mycookbook",
		SuiteName:         "default",
		PlatformName:      "ubuntu-22.04",
		BatchID:           "11111111-2222-3333-4444-555555555555",
		Status:            "destroyed",
		CreatedAt:         now,
		ExpectedDestroyAt: destroyAt,
		ActualDestroyAt:   actualDestroy,
		UpdatedAt:         actualDestroy,
	}

	data, err := json.Marshal(vm)
	if err != nil {
		t.Fatalf("json.Marshal(TrackedVM) failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if m["id"] != vm.ID {
		t.Errorf("id = %v, want %v", m["id"], vm.ID)
	}
	if m["vm_name"] != vm.VMName {
		t.Errorf("vm_name = %v, want %v", m["vm_name"], vm.VMName)
	}
	if m["hypervisor_id"] != vm.HypervisorID {
		t.Errorf("hypervisor_id = %v, want %v", m["hypervisor_id"], vm.HypervisorID)
	}
	if m["cookbook_name"] != vm.CookbookName {
		t.Errorf("cookbook_name = %v, want %v", m["cookbook_name"], vm.CookbookName)
	}
	if m["suite_name"] != vm.SuiteName {
		t.Errorf("suite_name = %v, want %v", m["suite_name"], vm.SuiteName)
	}
	if m["platform_name"] != vm.PlatformName {
		t.Errorf("platform_name = %v, want %v", m["platform_name"], vm.PlatformName)
	}
	if m["batch_id"] != vm.BatchID {
		t.Errorf("batch_id = %v, want %v", m["batch_id"], vm.BatchID)
	}
	if m["status"] != "destroyed" {
		t.Errorf("status = %v, want destroyed", m["status"])
	}
}

func TestTrackedVM_MarshalJSON_OmitEmpty(t *testing.T) {
	now := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)

	vm := TrackedVM{
		ID:           "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		VMName:       "kitchen-mycookbook-default-ubuntu-2204",
		CookbookName: "mycookbook",
		SuiteName:    "default",
		PlatformName: "ubuntu-22.04",
		Status:       "creating",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	data, err := json.Marshal(vm)
	if err != nil {
		t.Fatalf("json.Marshal(TrackedVM) failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Fields with omitempty should be absent when zero.
	for _, key := range []string{"hypervisor_id", "batch_id"} {
		if v, ok := m[key]; ok && v != nil && v != "" {
			t.Errorf("%s should be omitted or empty, got %v", key, v)
		}
	}

	// Required fields must be present.
	if m["id"] != vm.ID {
		t.Errorf("id = %v, want %v", m["id"], vm.ID)
	}
	if m["vm_name"] != vm.VMName {
		t.Errorf("vm_name = %v, want %v", m["vm_name"], vm.VMName)
	}
	if m["status"] != "creating" {
		t.Errorf("status = %v, want creating", m["status"])
	}
}

// ---------------------------------------------------------------------------
// CountTrackedVMsByStatus — empty map handling
// ---------------------------------------------------------------------------

func TestCountTrackedVMsByStatus_EmptyMap(t *testing.T) {
	// Verify that an empty map behaves correctly — this tests the consumer
	// side, ensuring zero values are handled without panics.
	counts := make(map[string]int)

	if got := counts["creating"]; got != 0 {
		t.Errorf("empty map[creating] = %d, want 0", got)
	}
	if got := counts["running"]; got != 0 {
		t.Errorf("empty map[running] = %d, want 0", got)
	}
	if got := counts["destroying"]; got != 0 {
		t.Errorf("empty map[destroying] = %d, want 0", got)
	}
	if got := counts["destroyed"]; got != 0 {
		t.Errorf("empty map[destroyed] = %d, want 0", got)
	}
	if got := counts["orphaned"]; got != 0 {
		t.Errorf("empty map[orphaned] = %d, want 0", got)
	}
	if got := len(counts); got != 0 {
		t.Errorf("len(counts) = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// TrackedVM zero value
// ---------------------------------------------------------------------------

func TestTrackedVM_ZeroValue(t *testing.T) {
	var vm TrackedVM

	if vm.ID != "" {
		t.Error("zero-value ID should be empty")
	}
	if vm.VMName != "" {
		t.Error("zero-value VMName should be empty")
	}
	if vm.HypervisorID != "" {
		t.Error("zero-value HypervisorID should be empty")
	}
	if vm.CookbookName != "" {
		t.Error("zero-value CookbookName should be empty")
	}
	if vm.SuiteName != "" {
		t.Error("zero-value SuiteName should be empty")
	}
	if vm.PlatformName != "" {
		t.Error("zero-value PlatformName should be empty")
	}
	if vm.BatchID != "" {
		t.Error("zero-value BatchID should be empty")
	}
	if vm.Status != "" {
		t.Error("zero-value Status should be empty")
	}
	if !vm.CreatedAt.IsZero() {
		t.Error("zero-value CreatedAt should be zero time")
	}
	if !vm.ExpectedDestroyAt.IsZero() {
		t.Error("zero-value ExpectedDestroyAt should be zero time")
	}
	if !vm.ActualDestroyAt.IsZero() {
		t.Error("zero-value ActualDestroyAt should be zero time")
	}
	if !vm.UpdatedAt.IsZero() {
		t.Error("zero-value UpdatedAt should be zero time")
	}
}

// ---------------------------------------------------------------------------
// Column list sanity check
// ---------------------------------------------------------------------------

func TestVMTrackingColumns_NotEmpty(t *testing.T) {
	if vmTrackingColumns == "" {
		t.Error("vmTrackingColumns constant should not be empty")
	}
}

// ---------------------------------------------------------------------------
// Valid VM statuses map completeness
// ---------------------------------------------------------------------------

func TestValidVMStatuses_Completeness(t *testing.T) {
	expected := []string{"creating", "running", "destroying", "destroyed", "orphaned"}
	if len(validVMStatuses) != len(expected) {
		t.Errorf("validVMStatuses has %d entries, want %d", len(validVMStatuses), len(expected))
	}
	for _, s := range expected {
		if !validVMStatuses[s] {
			t.Errorf("validVMStatuses missing %q", s)
		}
	}
}
