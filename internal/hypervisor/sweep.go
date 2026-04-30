// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"fmt"
	"strings"
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
// destroys those whose age exceeds ageThreshold. Age is determined by:
// 1. Embedded timestamp in CMM-named VMs (primary)
// 2. Hypervisor-reported uptime for legacy "kitchen-" VMs (fallback)
// VMs with unparseable names that don't match "kitchen-" are skipped.
// When dryRun is true, no VMs are destroyed but the result reports what
// would happen.
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
		age, ageKnown := vmAge(vm, prefix, now)
		if !ageKnown {
			result.SkippedUnparsed++
			result.Details = append(result.Details, SweepDetail{
				VMName:       vm.Name,
				HypervisorID: vm.HypervisorID,
				Action:       "skipped_unparsed",
			})
			continue
		}

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

// vmAge determines the age of a VM using the best available signal:
// 1. If the name parses as a CMM-named VM, use the embedded timestamp.
// 2. If the name starts with "kitchen-" and has a non-zero uptime, use uptime.
// 3. Otherwise, age is unknown.
func vmAge(vm ManagedVM, prefix string, nowUnix int64) (time.Duration, bool) {
	comp, ok := ParseVMName(vm.Name, prefix)
	if ok {
		return time.Duration(nowUnix-comp.Timestamp) * time.Second, true
	}
	if strings.HasPrefix(vm.Name, "kitchen-") && vm.Uptime > 0 {
		return vm.Uptime, true
	}
	return 0, false
}
