// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"fmt"
	"time"
)

// SweepResult holds the outcome of a hypervisor-side orphan sweep.
type SweepResult struct {
	Scanned         int           `json:"scanned"`
	Destroyed       int           `json:"destroyed"`
	SkippedTooYoung int           `json:"skipped_too_young"`
	SkippedUnparsed int           `json:"skipped_unparsed"`
	Errors          int           `json:"errors"`
	DryRun          bool          `json:"dry_run"`
	Details         []SweepDetail `json:"details"`
}

// SweepDetail describes the action taken (or skipped) for a single VM.
type SweepDetail struct {
	VMName       string        `json:"vm_name"`
	HypervisorID string        `json:"hypervisor_id"`
	Age          time.Duration `json:"age_seconds"`
	Action       string        `json:"action"`
	Error        string        `json:"error,omitempty"`
}

// SweepOrphanVMs lists all VMs matching prefix on the hypervisor and
// destroys those whose embedded timestamp indicates an age exceeding
// ageThreshold. VMs with unparseable names are skipped. When dryRun is
// true, no VMs are destroyed but the result reports what would happen.
func SweepOrphanVMs(ctx context.Context, hyp Hypervisor, prefix string, ageThreshold time.Duration, dryRun bool) (*SweepResult, error) {
	vms, err := hyp.ListManagedVMs(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("hypervisor sweep: listing VMs: %w", err)
	}

	result := &SweepResult{
		Scanned: len(vms),
		DryRun:  dryRun,
	}

	now := time.Now().Unix()

	for _, vm := range vms {
		comp, ok := ParseVMName(vm.Name, prefix)
		if !ok {
			result.SkippedUnparsed++
			result.Details = append(result.Details, SweepDetail{
				VMName:       vm.Name,
				HypervisorID: vm.HypervisorID,
				Action:       "skipped_unparsed",
			})
			continue
		}

		age := time.Duration(now-comp.Timestamp) * time.Second
		if age < ageThreshold {
			result.SkippedTooYoung++
			result.Details = append(result.Details, SweepDetail{
				VMName:       vm.Name,
				HypervisorID: vm.HypervisorID,
				Age:          age,
				Action:       "skipped_too_young",
			})
			continue
		}

		if dryRun {
			result.Details = append(result.Details, SweepDetail{
				VMName:       vm.Name,
				HypervisorID: vm.HypervisorID,
				Age:          age,
				Action:       "would_destroy",
			})
			continue
		}

		if err := hyp.DestroyVM(ctx, vm.HypervisorID); err != nil {
			result.Errors++
			result.Details = append(result.Details, SweepDetail{
				VMName:       vm.Name,
				HypervisorID: vm.HypervisorID,
				Age:          age,
				Action:       "error",
				Error:        err.Error(),
			})
			continue
		}

		result.Destroyed++
		result.Details = append(result.Details, SweepDetail{
			VMName:       vm.Name,
			HypervisorID: vm.HypervisorID,
			Age:          age,
			Action:       "destroyed",
		})
	}

	return result, nil
}
