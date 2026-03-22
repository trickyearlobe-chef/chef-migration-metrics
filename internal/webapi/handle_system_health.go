// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"

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

	sh := r.cfg.SystemHealth

	th := syshealth.Thresholds{
		DiskUsedWarningPercent:  sh.DiskUsedWarningPercent,
		DiskUsedCriticalPercent: sh.DiskUsedCriticalPercent,
		CPULoadWarningPerCPU:    sh.CPULoadWarningPerCPU,
		CPULoadCriticalPerCPU:   sh.CPULoadCriticalPerCPU,
		MemUsedWarningPercent:   sh.MemUsedWarningPercent,
		MemUsedCriticalPercent:  sh.MemUsedCriticalPercent,
	}

	stats := syshealth.Snapshot(sh.DiskPath, th)

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

	// Ensure alerts is never null in JSON.
	alerts := stats.Alerts
	if alerts == nil {
		alerts = []syshealth.Alert{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"timestamp": stats.Timestamp,
		"uptime":    stats.Uptime,

		"disk_path":         stats.DiskPath,
		"disk_total_bytes":  stats.DiskTotalBytes,
		"disk_free_bytes":   stats.DiskFreeBytes,
		"disk_used_percent": stats.DiskUsedPercent,

		"cpu_count":    stats.CPUCount,
		"load_avg_1":   stats.LoadAvg1,
		"load_per_cpu": stats.LoadPerCPU,

		"mem_total_bytes":  stats.MemTotalBytes,
		"mem_avail_bytes":  stats.MemAvailBytes,
		"mem_used_percent": stats.MemUsedPercent,

		"go_heap_bytes": stats.GoHeapBytes,
		"go_goroutines": stats.GoGoroutines,

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
