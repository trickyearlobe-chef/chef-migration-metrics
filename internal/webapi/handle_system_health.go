// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/syshealth"
)

// handleAdminSystemHealth handles GET /api/v1/admin/system-health.
// Returns a snapshot of host-level resource metrics (disk, CPU, memory)
// and Go runtime stats, along with any alerts evaluated against the
// configured thresholds.
func (r *Router) handleAdminSystemHealth(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	sh := r.liveConfig().SystemHealth

	th := syshealth.Thresholds{
		DiskUsedWarningPercent:  sh.DiskUsedWarningPercent,
		DiskUsedCriticalPercent: sh.DiskUsedCriticalPercent,
		CPULoadWarningPerCPU:    sh.CPULoadWarningPerCPU,
		CPULoadCriticalPerCPU:   sh.CPULoadCriticalPerCPU,
		MemUsedWarningPercent:   sh.MemUsedWarningPercent,
		MemUsedCriticalPercent:  sh.MemUsedCriticalPercent,
	}

	stats := syshealth.Snapshot(sh.DiskPaths, th)

	// Query database size and per-table breakdown (best-effort — don't
	// fail the whole endpoint if the DB is unreachable).
	var dbSizeBytes int64
	var tableSizes []datastore.TableSize
	if r.db != nil {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		size, err := r.db.DatabaseSize(ctx)
		if err == nil {
			dbSizeBytes = size
		}
		ts, err := r.db.DatabaseTableSizes(ctx)
		if err == nil {
			tableSizes = ts
		}
	}
	if tableSizes == nil {
		tableSizes = []datastore.TableSize{}
	}

	// Determine whether the collection circuit breaker would trip.
	collectionPaused := sh.IsPauseCollectionOnCritical() && syshealth.ShouldPauseCollection(stats)

	type thresholdsResponse struct {
		DiskUsedWarningPercent  float64 `json:"disk_used_warning_percent"`
		DiskUsedCriticalPercent float64 `json:"disk_used_critical_percent"`
		CPULoadWarningPerCPU    float64 `json:"cpu_load_warning_per_cpu"`
		CPULoadCriticalPerCPU   float64 `json:"cpu_load_critical_per_cpu"`
		MemUsedWarningPercent   float64 `json:"mem_used_warning_percent"`
		MemUsedCriticalPercent  float64 `json:"mem_used_critical_percent"`
	}

	// Ensure alerts and disks are never null in JSON.
	alerts := stats.Alerts
	if alerts == nil {
		alerts = []syshealth.Alert{}
	}
	disks := stats.Disks
	if disks == nil {
		disks = []syshealth.DiskStats{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"timestamp": stats.Timestamp,
		"uptime":    stats.Uptime,

		"disks": disks,

		"cpu_count":    stats.CPUCount,
		"load_avg_1":   stats.LoadAvg1,
		"load_per_cpu": stats.LoadPerCPU,

		"mem_total_bytes":  stats.MemTotalBytes,
		"mem_avail_bytes":  stats.MemAvailBytes,
		"mem_used_percent": stats.MemUsedPercent,

		"go_heap_bytes": stats.GoHeapBytes,
		"go_goroutines": stats.GoGoroutines,

		"database_size_bytes": dbSizeBytes,
		"table_sizes":         tableSizes,

		"alerts":            alerts,
		"collection_paused": collectionPaused,

		"thresholds": thresholdsResponse{
			DiskUsedWarningPercent:  sh.DiskUsedWarningPercent,
			DiskUsedCriticalPercent: sh.DiskUsedCriticalPercent,
			CPULoadWarningPerCPU:    sh.CPULoadWarningPerCPU,
			CPULoadCriticalPerCPU:   sh.CPULoadCriticalPerCPU,
			MemUsedWarningPercent:   sh.MemUsedWarningPercent,
			MemUsedCriticalPercent:  sh.MemUsedCriticalPercent,
		},
	})
}
