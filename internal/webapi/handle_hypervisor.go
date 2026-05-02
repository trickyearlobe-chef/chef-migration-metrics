// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/hypervisor"
)

// handleHypervisorTemplates returns the list of VM templates available on
// the configured hypervisor.
//
//	GET /api/v1/hypervisor/templates
func (r *Router) handleHypervisorTemplates(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	if r.hypervisor == nil {
		WriteJSON(w, http.StatusOK, []any{})
		return
	}

	ctx := req.Context()
	templates, err := r.hypervisor.ListTemplates(ctx)
	if err != nil {
		r.logf("ERROR", "failed to list hypervisor templates: %v", err)
		WriteInternalError(w, "Failed to retrieve hypervisor templates.")
		return
	}
	if templates == nil {
		templates = []hypervisor.Template{}
	}
	WriteJSON(w, http.StatusOK, templates)
}

// handleHypervisorVMs returns tracked VMs, optionally filtered by status.
//
//	GET /api/v1/hypervisor/vms
//	GET /api/v1/hypervisor/vms?status=orphaned
func (r *Router) handleHypervisorVMs(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()
	status := req.URL.Query().Get("status")

	var vms []datastore.TrackedVM
	var err error
	if status != "" {
		vms, err = r.db.ListTrackedVMsFiltered(ctx, status)
	} else {
		vms, err = r.db.ListTrackedVMs(ctx)
	}
	if err != nil {
		r.logf("ERROR", "failed to list tracked VMs: %v", err)
		WriteInternalError(w, "Failed to retrieve tracked VMs.")
		return
	}
	if vms == nil {
		vms = []datastore.TrackedVM{}
	}
	WriteJSON(w, http.StatusOK, vms)
}

// handleHypervisorDestroyVM destroys a single VM by its tracked ID.
//
//	POST /api/v1/hypervisor/vms/:id/destroy
func (r *Router) handleHypervisorDestroyVM(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	segments := pathSegments(req.URL.Path, "/api/v1/hypervisor/vms/")
	if len(segments) < 2 || segments[len(segments)-1] != "destroy" {
		WriteNotFound(w, "Expected path: /api/v1/hypervisor/vms/:id/destroy")
		return
	}
	vmID := segments[0]

	ctx := req.Context()
	vm, err := r.db.GetTrackedVM(ctx, vmID)
	if err != nil {
		r.logf("ERROR", "failed to get tracked VM %s: %v", vmID, err)
		WriteInternalError(w, "Failed to retrieve VM details.")
		return
	}
	if vm == nil {
		WriteNotFound(w, "VM not found.")
		return
	}

	if r.hypervisor != nil && vm.HypervisorID != "" {
		if err := r.hypervisor.DestroyVM(ctx, vm.HypervisorID); err != nil {
			r.logf("ERROR", "failed to destroy VM %s on hypervisor: %v", vm.HypervisorID, err)
			WriteInternalError(w, "Failed to destroy VM on hypervisor.")
			return
		}
	}

	if err := r.db.MarkVMDestroyed(ctx, vmID); err != nil {
		r.logf("ERROR", "failed to mark VM %s as destroyed: %v", vmID, err)
		WriteInternalError(w, "VM destroyed on hypervisor but failed to update database.")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "destroyed", "vm_id": vmID})
}

// handleHypervisorCleanup detects and destroys all orphaned VMs.
//
//	POST /api/v1/hypervisor/cleanup
func (r *Router) handleHypervisorCleanup(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	ctx := req.Context()
	prefix := r.liveConfig().AnalysisTools.TestKitchen.EffectiveVMNamePrefix()

	result, err := hypervisor.CleanupOrphans(ctx, r.db, r.hypervisor, prefix)
	if err != nil {
		r.logf("ERROR", "orphan cleanup failed: %v", err)
		WriteInternalError(w, "Orphan cleanup failed.")
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// handleHypervisorTestConnection tests connectivity to the configured
// hypervisor by calling ListTemplates. Returns status, type, and the
// discovered templates on success.
//
//	POST /api/v1/admin/hypervisor/test-connection
func (r *Router) handleHypervisorTestConnection(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	if r.hypervisor == nil {
		WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "not_configured",
			"message": "No hypervisor is configured. Set the driver type and credentials first.",
		})
		return
	}

	ctx := req.Context()
	templates, err := r.hypervisor.ListTemplates(ctx)
	if err != nil {
		r.logf("ERROR", "hypervisor test-connection failed: %v", err)
		WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}
	if templates == nil {
		templates = []hypervisor.Template{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"hypervisor_type": r.hypervisor.Type(),
		"template_count":  len(templates),
		"templates":       templates,
	})
}

// handleOrphanSweep performs a hypervisor-side orphan sweep by listing VMs
// directly from the hypervisor and destroying those exceeding the age
// threshold based on the timestamp embedded in their names.
//
//	POST /api/v1/kitchen/orphan-sweep?dry_run=true
func (r *Router) handleOrphanSweep(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	if r.hypervisor == nil {
		WriteJSON(w, http.StatusOK, &hypervisor.SweepResult{
			Details: []hypervisor.SweepDetail{},
		})
		return
	}

	// dry_run defaults to true for safety.
	dryRun := true
	if req.URL.Query().Get("dry_run") == "false" {
		dryRun = false
	}

	cfg := r.liveConfig().AnalysisTools.TestKitchen
	prefix := cfg.EffectiveVMNamePrefix()
	ageThreshold := cfg.EffectiveOrphanSweepAge()

	ctx := req.Context()
	result, err := hypervisor.SweepOrphanVMs(ctx, r.hypervisor, prefix, ageThreshold, dryRun)
	if err != nil {
		r.logf("ERROR", "orphan sweep failed: %v", err)
		WriteInternalError(w, "Orphan sweep failed.")
		return
	}

	r.hub.Broadcast(Event{
		Type:      "orphan_sweep_complete",
		Timestamp: time.Now().UTC(),
		Data:      result,
	})

	WriteJSON(w, http.StatusOK, result)
}
