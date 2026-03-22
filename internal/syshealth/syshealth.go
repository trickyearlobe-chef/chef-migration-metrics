// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package syshealth provides lightweight host-level health metrics
// (disk, CPU load, memory) using only Go stdlib and syscall. No external
// dependencies like gopsutil are required.
package syshealth

import (
	"fmt"
	"runtime"
	"sort"
	"time"
)

// startTime records when the process started, used to compute uptime.
var startTime = time.Now()

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// DiskStats holds usage information for a single filesystem / mount point.
type DiskStats struct {
	Path        string  `json:"path"`
	Device      uint64  `json:"-"` // device ID for de-duplication (not serialised)
	TotalBytes  uint64  `json:"total_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

// Stats holds a point-in-time snapshot of host-level and Go runtime metrics.
type Stats struct {
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`

	// Disk — one entry per unique filesystem detected from the
	// configured paths. Multiple paths on the same device are
	// de-duplicated; the first path encountered is kept as the label.
	Disks []DiskStats `json:"disks"`

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
// diskPaths lists filesystem paths whose mount points are checked for free
// space (e.g. the data directory, export directory, database volume).
// Multiple paths that resolve to the same underlying device are
// de-duplicated — only the first path per device is kept. If no paths
// are provided, "/" is used as a fallback.
func Snapshot(diskPaths []string, th Thresholds) Stats {
	if len(diskPaths) == 0 {
		diskPaths = []string{"/"}
	}

	now := time.Now()

	s := Stats{
		Timestamp: now,
		Uptime:    formatDuration(now.Sub(startTime)),
		CPUCount:  runtime.NumCPU(),
	}

	// Disk -----------------------------------------------------------
	s.Disks = collectDisks(diskPaths)

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

// collectDisks probes each path for disk usage and de-duplicates by
// underlying device ID. Paths that fail to stat are silently skipped.
// The returned slice preserves the order of first-seen devices.
func collectDisks(paths []string) []DiskStats {
	seen := make(map[uint64]bool)
	var disks []DiskStats

	for _, p := range paths {
		if p == "" {
			continue
		}
		total, free, devID, err := diskUsageWithDevice(p)
		if err != nil {
			continue
		}
		if seen[devID] {
			continue
		}
		seen[devID] = true

		usedPct := 0.0
		if total > 0 {
			usedPct = float64(total-free) / float64(total) * 100
		}
		disks = append(disks, DiskStats{
			Path:        p,
			Device:      devID,
			TotalBytes:  total,
			FreeBytes:   free,
			UsedPercent: usedPct,
		})
	}

	return disks
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

	// Disk alerts — evaluated per filesystem.
	for _, d := range s.Disks {
		if d.TotalBytes == 0 {
			continue
		}
		freeGB := float64(d.FreeBytes) / (1 << 30)
		totalGB := float64(d.TotalBytes) / (1 << 30)
		if th.DiskUsedCriticalPercent > 0 && d.UsedPercent >= th.DiskUsedCriticalPercent {
			alerts = append(alerts, Alert{
				Level:  "critical",
				Metric: "disk",
				Message: fmt.Sprintf("Disk usage at %.1f%% on %s (%.1f GB free of %.1f GB)",
					d.UsedPercent, d.Path, freeGB, totalGB),
			})
		} else if th.DiskUsedWarningPercent > 0 && d.UsedPercent >= th.DiskUsedWarningPercent {
			alerts = append(alerts, Alert{
				Level:  "warning",
				Metric: "disk",
				Message: fmt.Sprintf("Disk usage at %.1f%% on %s (%.1f GB free of %.1f GB)",
					d.UsedPercent, d.Path, freeGB, totalGB),
			})
		}
	}

	// Sort disk alerts so critical comes before warning for deterministic output.
	sort.SliceStable(alerts, func(i, j int) bool {
		if alerts[i].Level == alerts[j].Level {
			return false
		}
		return alerts[i].Level == "critical"
	})

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
