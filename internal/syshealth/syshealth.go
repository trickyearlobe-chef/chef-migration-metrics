// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package syshealth provides lightweight host-level health metrics
// (disk, CPU load, memory) using only Go stdlib and syscall. No external
// dependencies like gopsutil are required.
package syshealth

import (
	"fmt"
	"runtime"
	"time"
)

// startTime records when the process started, used to compute uptime.
var startTime = time.Now()

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Stats holds a point-in-time snapshot of host-level and Go runtime metrics.
type Stats struct {
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`

	// Disk
	DiskPath        string  `json:"disk_path"`
	DiskTotalBytes  uint64  `json:"disk_total_bytes"`
	DiskFreeBytes   uint64  `json:"disk_free_bytes"`
	DiskUsedPercent float64 `json:"disk_used_percent"`

	// CPU / Load
	CPUCount   int     `json:"cpu_count"`
	LoadAvg1   float64 `json:"load_avg_1"`
	LoadPerCPU float64 `json:"load_per_cpu"`

	// OS Memory
	MemTotalBytes  uint64  `json:"mem_total_bytes"`
	MemAvailBytes  uint64  `json:"mem_avail_bytes"`
	MemUsedPercent float64 `json:"mem_used_percent"`

	// Go runtime
	GoHeapBytes  uint64 `json:"go_heap_bytes"`
	GoGoroutines int    `json:"go_goroutines"`

	// Alerts evaluated against thresholds.
	Alerts []Alert `json:"alerts"`
}

// Alert represents a threshold breach for a single metric.
type Alert struct {
	Level   string `json:"level"`  // "warning" or "critical"
	Metric  string `json:"metric"` // "disk", "cpu", "memory"
	Message string `json:"message"`
}

// Thresholds defines warning and critical levels for each monitored metric.
type Thresholds struct {
	DiskUsedWarningPercent  float64
	DiskUsedCriticalPercent float64
	CPULoadWarningPerCPU    float64
	CPULoadCriticalPerCPU   float64
	MemUsedWarningPercent   float64
	MemUsedCriticalPercent  float64
}

// DefaultThresholds returns sensible default thresholds.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DiskUsedWarningPercent:  80,
		DiskUsedCriticalPercent: 90,
		CPULoadWarningPerCPU:    2.0,
		CPULoadCriticalPerCPU:   4.0,
		MemUsedWarningPercent:   80,
		MemUsedCriticalPercent:  90,
	}
}

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------

// Snapshot collects all host-level and Go runtime metrics, evaluates them
// against the provided thresholds, and returns a Stats value. The function
// is stateless and safe to call concurrently.
//
// diskPath is the filesystem path whose mount point is checked for free
// space (e.g. the data directory). If empty, "/" is used.
func Snapshot(diskPath string, th Thresholds) Stats {
	if diskPath == "" {
		diskPath = "/"
	}

	now := time.Now()

	s := Stats{
		Timestamp: now,
		Uptime:    formatDuration(now.Sub(startTime)),
		DiskPath:  diskPath,
		CPUCount:  runtime.NumCPU(),
	}

	// Disk -----------------------------------------------------------
	total, free, err := diskUsage(diskPath)
	if err == nil {
		s.DiskTotalBytes = total
		s.DiskFreeBytes = free
		if total > 0 {
			s.DiskUsedPercent = float64(total-free) / float64(total) * 100
		}
	}

	// CPU load -------------------------------------------------------
	load, err := loadAvg1()
	if err == nil {
		s.LoadAvg1 = load
		if s.CPUCount > 0 {
			s.LoadPerCPU = load / float64(s.CPUCount)
		}
	}

	// OS Memory ------------------------------------------------------
	memTotal, memAvail, err := memoryUsage()
	if err == nil {
		s.MemTotalBytes = memTotal
		s.MemAvailBytes = memAvail
		if memTotal > 0 {
			s.MemUsedPercent = float64(memTotal-memAvail) / float64(memTotal) * 100
		}
	}

	// Go runtime -----------------------------------------------------
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.GoHeapBytes = m.HeapInuse
	s.GoGoroutines = runtime.NumGoroutine()

	// Evaluate alerts ------------------------------------------------
	s.Alerts = evaluateAlerts(s, th)

	return s
}

// ShouldPauseCollection returns true if any alert is at the "critical" level.
func ShouldPauseCollection(s Stats) bool {
	for _, a := range s.Alerts {
		if a.Level == "critical" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Alert evaluation
// ---------------------------------------------------------------------------

func evaluateAlerts(s Stats, th Thresholds) []Alert {
	var alerts []Alert

	// Disk alerts (check critical first so both can fire).
	if s.DiskTotalBytes > 0 {
		freeGB := float64(s.DiskFreeBytes) / (1 << 30)
		totalGB := float64(s.DiskTotalBytes) / (1 << 30)
		if th.DiskUsedCriticalPercent > 0 && s.DiskUsedPercent >= th.DiskUsedCriticalPercent {
			alerts = append(alerts, Alert{
				Level:  "critical",
				Metric: "disk",
				Message: fmt.Sprintf("Disk usage at %.1f%% on %s (%.1f GB free of %.1f GB)",
					s.DiskUsedPercent, s.DiskPath, freeGB, totalGB),
			})
		} else if th.DiskUsedWarningPercent > 0 && s.DiskUsedPercent >= th.DiskUsedWarningPercent {
			alerts = append(alerts, Alert{
				Level:  "warning",
				Metric: "disk",
				Message: fmt.Sprintf("Disk usage at %.1f%% on %s (%.1f GB free of %.1f GB)",
					s.DiskUsedPercent, s.DiskPath, freeGB, totalGB),
			})
		}
	}

	// CPU load alerts.
	if s.CPUCount > 0 && s.LoadAvg1 > 0 {
		if th.CPULoadCriticalPerCPU > 0 && s.LoadPerCPU >= th.CPULoadCriticalPerCPU {
			alerts = append(alerts, Alert{
				Level:  "critical",
				Metric: "cpu",
				Message: fmt.Sprintf("CPU load average %.2f (%.2f per CPU, %d CPUs)",
					s.LoadAvg1, s.LoadPerCPU, s.CPUCount),
			})
		} else if th.CPULoadWarningPerCPU > 0 && s.LoadPerCPU >= th.CPULoadWarningPerCPU {
			alerts = append(alerts, Alert{
				Level:  "warning",
				Metric: "cpu",
				Message: fmt.Sprintf("CPU load average %.2f (%.2f per CPU, %d CPUs)",
					s.LoadAvg1, s.LoadPerCPU, s.CPUCount),
			})
		}
	}

	// Memory alerts.
	if s.MemTotalBytes > 0 {
		availGB := float64(s.MemAvailBytes) / (1 << 30)
		totalGB := float64(s.MemTotalBytes) / (1 << 30)
		if th.MemUsedCriticalPercent > 0 && s.MemUsedPercent >= th.MemUsedCriticalPercent {
			alerts = append(alerts, Alert{
				Level:  "critical",
				Metric: "memory",
				Message: fmt.Sprintf("Memory usage at %.1f%% (%.1f GB available of %.1f GB)",
					s.MemUsedPercent, availGB, totalGB),
			})
		} else if th.MemUsedWarningPercent > 0 && s.MemUsedPercent >= th.MemUsedWarningPercent {
			alerts = append(alerts, Alert{
				Level:  "warning",
				Metric: "memory",
				Message: fmt.Sprintf("Memory usage at %.1f%% (%.1f GB available of %.1f GB)",
					s.MemUsedPercent, availGB, totalGB),
			})
		}
	}

	return alerts
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// formatDuration formats a duration as "Xd Yh Zm" for human readability.
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
