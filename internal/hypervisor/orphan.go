// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// OrphanResult holds the outcome of an orphan detection/cleanup operation.
type OrphanResult struct {
	Detected  int      `json:"detected"`
	Destroyed int      `json:"destroyed"`
	Errors    int      `json:"errors"`
	Details   []string `json:"details"`
}

// OrphanStore is the minimal DB interface needed for orphan operations.
type OrphanStore interface {
	ListOrphanedVMs(ctx context.Context) ([]datastore.TrackedVM, error)
	MarkVMOrphaned(ctx context.Context, id string) error
	MarkVMDestroyed(ctx context.Context, id string) error
}

// DetectOrphans queries the database for VMs that have exceeded their TTL
// and marks them as orphaned. When hyp is non-nil and not a NullHypervisor,
// it cross-references with the hypervisor to note whether the VMs are still
// running.
func DetectOrphans(ctx context.Context, store OrphanStore, hyp Hypervisor, prefix string) (*OrphanResult, error) {
	result := &OrphanResult{}

	expired, err := store.ListOrphanedVMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("hypervisor: listing orphaned VMs: %w", err)
	}

	for _, vm := range expired {
		if err := store.MarkVMOrphaned(ctx, vm.ID); err != nil {
			result.Errors++
			result.Details = append(result.Details, fmt.Sprintf("failed to mark VM %s (%s) as orphaned: %v", vm.ID, vm.VMName, err))
			continue
		}
		result.Detected++
		result.Details = append(result.Details, fmt.Sprintf("marked VM %s (%s) as orphaned", vm.ID, vm.VMName))
	}

	if hyp == nil || isNullHypervisor(hyp) {
		return result, nil
	}

	managed, err := hyp.ListManagedVMs(ctx, prefix)
	if err != nil {
		result.Details = append(result.Details, fmt.Sprintf("failed to list hypervisor VMs: %v", err))
		return result, nil
	}

	liveSet := make(map[string]ManagedVM, len(managed))
	for _, m := range managed {
		liveSet[m.Name] = m
	}

	for _, vm := range expired {
		if live, ok := liveSet[vm.VMName]; ok {
			result.Details = append(result.Details, fmt.Sprintf("VM %s (%s) still running on hypervisor (power_state=%s)", vm.ID, vm.VMName, live.PowerState))
		}
	}

	return result, nil
}

// CleanupOrphans detects new orphans, then attempts to destroy all orphaned
// VMs on the hypervisor and updates the database accordingly.
func CleanupOrphans(ctx context.Context, store OrphanStore, hyp Hypervisor, prefix string) (*OrphanResult, error) {
	// First pass: flag any newly expired VMs as orphaned.
	detectResult, err := DetectOrphans(ctx, store, hyp, prefix)
	if err != nil {
		return nil, fmt.Errorf("hypervisor: detect phase of cleanup: %w", err)
	}

	result := &OrphanResult{
		Detected: detectResult.Detected,
		Details:  detectResult.Details,
		Errors:   detectResult.Errors,
	}

	if hyp == nil || isNullHypervisor(hyp) {
		result.Details = append(result.Details, "no hypervisor configured — skipping VM destruction")
		return result, nil
	}

	// Second pass: re-fetch orphaned VMs (now includes newly flagged ones).
	orphaned, err := store.ListOrphanedVMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("hypervisor: listing orphaned VMs for cleanup: %w", err)
	}

	for _, vm := range orphaned {
		if vm.Status != "orphaned" {
			continue
		}
		if vm.HypervisorID == "" {
			result.Details = append(result.Details, fmt.Sprintf("VM %s (%s) has no hypervisor ID — marking destroyed", vm.ID, vm.VMName))
			if err := store.MarkVMDestroyed(ctx, vm.ID); err != nil {
				result.Errors++
				result.Details = append(result.Details, fmt.Sprintf("failed to mark VM %s as destroyed: %v", vm.ID, err))
				continue
			}
			result.Destroyed++
			continue
		}

		if err := hyp.DestroyVM(ctx, vm.HypervisorID); err != nil {
			result.Errors++
			result.Details = append(result.Details, fmt.Sprintf("failed to destroy VM %s (%s) on hypervisor: %v", vm.ID, vm.VMName, err))
			continue
		}

		if err := store.MarkVMDestroyed(ctx, vm.ID); err != nil {
			result.Errors++
			result.Details = append(result.Details, fmt.Sprintf("destroyed VM %s on hypervisor but failed to update DB: %v", vm.ID, err))
			continue
		}

		result.Destroyed++
		result.Details = append(result.Details, fmt.Sprintf("destroyed VM %s (%s)", vm.ID, vm.VMName))
	}

	return result, nil
}

// isNullHypervisor reports whether hyp is a NullHypervisor.
func isNullHypervisor(hyp Hypervisor) bool {
	_, ok := hyp.(NullHypervisor)
	return ok
}
