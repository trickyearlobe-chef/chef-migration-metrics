// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package syshealth

import (
	"runtime"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Snapshot — basic sanity checks
// ---------------------------------------------------------------------------

func TestSnapshot_ReturnsValidStats(t *testing.T) {
	s := Snapshot("/", DefaultThresholds())

	if s.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if s.Uptime == "" {
		t.Error("Uptime should not be empty")
	}
	if s.DiskPath != "/" {
		t.Errorf("DiskPath = %q, want /", s.DiskPath)
	}
	if s.CPUCount != runtime.NumCPU() {
		t.Errorf("CPUCount = %d, want %d", s.CPUCount, runtime.NumCPU())
	}
	if s.GoGoroutines <= 0 {
		t.Errorf("GoGoroutines = %d, want > 0", s.GoGoroutines)
	}
	if s.GoHeapBytes == 0 {
		t.Error("GoHeapBytes should not be zero")
	}
}

func TestSnapshot_DiskMetrics_Populated(t *testing.T) {
	s := Snapshot("/", DefaultThresholds())

	// On any real system the root filesystem should have some capacity.
	if s.DiskTotalBytes == 0 {
		t.Skip("DiskTotalBytes is zero — disk metrics may not be supported on this platform")
	}
	if s.DiskFreeBytes == 0 {
		t.Error("DiskFreeBytes should not be zero on a healthy system")
	}
	if s.DiskUsedPercent <= 0 || s.DiskUsedPercent > 100 {
		t.Errorf("DiskUsedPercent = %.2f, want between 0 and 100", s.DiskUsedPercent)
	}
}

func TestSnapshot_CPUMetrics_Populated(t *testing.T) {
	s := Snapshot("/", DefaultThresholds())

	if s.CPUCount <= 0 {
		t.Errorf("CPUCount = %d, want > 0", s.CPUCount)
	}
	// LoadAvg1 may be zero on some CI environments; just check it's not negative.
	if s.LoadAvg1 < 0 {
		t.Errorf("LoadAvg1 = %.2f, want >= 0", s.LoadAvg1)
	}
	if s.LoadPerCPU < 0 {
		t.Errorf("LoadPerCPU = %.2f, want >= 0", s.LoadPerCPU)
	}
}

func TestSnapshot_MemoryMetrics_Populated(t *testing.T) {
	s := Snapshot("/", DefaultThresholds())

	if s.MemTotalBytes == 0 {
		t.Skip("MemTotalBytes is zero — memory metrics may not be supported on this platform")
	}
	if s.MemAvailBytes == 0 {
		t.Error("MemAvailBytes should not be zero on a healthy system")
	}
	if s.MemUsedPercent <= 0 || s.MemUsedPercent > 100 {
		t.Errorf("MemUsedPercent = %.2f, want between 0 and 100", s.MemUsedPercent)
	}
}

func TestSnapshot_DefaultDiskPath(t *testing.T) {
	s := Snapshot("", DefaultThresholds())
	if s.DiskPath != "/" {
		t.Errorf("DiskPath = %q, want / when empty string is passed", s.DiskPath)
	}
}

// ---------------------------------------------------------------------------
// Alert evaluation
// ---------------------------------------------------------------------------

func TestEvaluateAlerts_DiskWarning(t *testing.T) {
	s := Stats{
		DiskTotalBytes:  100,
		DiskFreeBytes:   15,
		DiskUsedPercent: 85.0,
		DiskPath:        "/data",
	}
	th := Thresholds{
		DiskUsedWarningPercent:  80,
		DiskUsedCriticalPercent: 90,
	}

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	if alerts[0].Level != "warning" {
		t.Errorf("alert level = %q, want warning", alerts[0].Level)
	}
	if alerts[0].Metric != "disk" {
		t.Errorf("alert metric = %q, want disk", alerts[0].Metric)
	}
}

func TestEvaluateAlerts_DiskCritical(t *testing.T) {
	s := Stats{
		DiskTotalBytes:  100,
		DiskFreeBytes:   5,
		DiskUsedPercent: 95.0,
		DiskPath:        "/data",
	}
	th := Thresholds{
		DiskUsedWarningPercent:  80,
		DiskUsedCriticalPercent: 90,
	}

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("alert level = %q, want critical", alerts[0].Level)
	}
	if alerts[0].Metric != "disk" {
		t.Errorf("alert metric = %q, want disk", alerts[0].Metric)
	}
}

func TestEvaluateAlerts_CPUWarning(t *testing.T) {
	s := Stats{
		CPUCount:   4,
		LoadAvg1:   10.0,
		LoadPerCPU: 2.5,
	}
	th := Thresholds{
		CPULoadWarningPerCPU:  2.0,
		CPULoadCriticalPerCPU: 4.0,
	}

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	if alerts[0].Level != "warning" {
		t.Errorf("alert level = %q, want warning", alerts[0].Level)
	}
	if alerts[0].Metric != "cpu" {
		t.Errorf("alert metric = %q, want cpu", alerts[0].Metric)
	}
}

func TestEvaluateAlerts_CPUCritical(t *testing.T) {
	s := Stats{
		CPUCount:   4,
		LoadAvg1:   20.0,
		LoadPerCPU: 5.0,
	}
	th := Thresholds{
		CPULoadWarningPerCPU:  2.0,
		CPULoadCriticalPerCPU: 4.0,
	}

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("alert level = %q, want critical", alerts[0].Level)
	}
}

func TestEvaluateAlerts_MemoryWarning(t *testing.T) {
	s := Stats{
		MemTotalBytes:  1000,
		MemAvailBytes:  150,
		MemUsedPercent: 85.0,
	}
	th := Thresholds{
		MemUsedWarningPercent:  80,
		MemUsedCriticalPercent: 90,
	}

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	if alerts[0].Level != "warning" {
		t.Errorf("alert level = %q, want warning", alerts[0].Level)
	}
	if alerts[0].Metric != "memory" {
		t.Errorf("alert metric = %q, want memory", alerts[0].Metric)
	}
}

func TestEvaluateAlerts_MemoryCritical(t *testing.T) {
	s := Stats{
		MemTotalBytes:  1000,
		MemAvailBytes:  50,
		MemUsedPercent: 95.0,
	}
	th := Thresholds{
		MemUsedWarningPercent:  80,
		MemUsedCriticalPercent: 90,
	}

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("alert level = %q, want critical", alerts[0].Level)
	}
}

func TestEvaluateAlerts_NoAlerts_BelowThresholds(t *testing.T) {
	s := Stats{
		DiskTotalBytes:  100,
		DiskFreeBytes:   50,
		DiskUsedPercent: 50.0,
		DiskPath:        "/",
		CPUCount:        4,
		LoadAvg1:        2.0,
		LoadPerCPU:      0.5,
		MemTotalBytes:   1000,
		MemAvailBytes:   500,
		MemUsedPercent:  50.0,
	}
	th := DefaultThresholds()

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 0 {
		t.Errorf("len(alerts) = %d, want 0; alerts = %+v", len(alerts), alerts)
	}
}

func TestEvaluateAlerts_NoAlerts_ZeroValues(t *testing.T) {
	// When metrics are zero (e.g. unsupported platform), no alerts should fire.
	s := Stats{}
	th := DefaultThresholds()

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 0 {
		t.Errorf("len(alerts) = %d, want 0 for zero-valued stats", len(alerts))
	}
}

func TestEvaluateAlerts_MultipleAlerts(t *testing.T) {
	s := Stats{
		DiskTotalBytes:  100,
		DiskFreeBytes:   2,
		DiskUsedPercent: 98.0,
		DiskPath:        "/data",
		CPUCount:        2,
		LoadAvg1:        10.0,
		LoadPerCPU:      5.0,
		MemTotalBytes:   1000,
		MemAvailBytes:   50,
		MemUsedPercent:  95.0,
	}
	th := DefaultThresholds()

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 3 {
		t.Fatalf("len(alerts) = %d, want 3; alerts = %+v", len(alerts), alerts)
	}

	// Verify we got one alert per metric.
	metrics := make(map[string]bool)
	for _, a := range alerts {
		metrics[a.Metric] = true
	}
	for _, m := range []string{"disk", "cpu", "memory"} {
		if !metrics[m] {
			t.Errorf("expected alert for metric %q", m)
		}
	}
}

func TestEvaluateAlerts_ExactlyAtWarningThreshold(t *testing.T) {
	s := Stats{
		DiskTotalBytes:  100,
		DiskFreeBytes:   20,
		DiskUsedPercent: 80.0,
		DiskPath:        "/",
	}
	th := Thresholds{
		DiskUsedWarningPercent:  80,
		DiskUsedCriticalPercent: 90,
	}

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1 at exact threshold", len(alerts))
	}
	if alerts[0].Level != "warning" {
		t.Errorf("alert level = %q, want warning at exact threshold", alerts[0].Level)
	}
}

func TestEvaluateAlerts_ExactlyAtCriticalThreshold(t *testing.T) {
	s := Stats{
		DiskTotalBytes:  100,
		DiskFreeBytes:   10,
		DiskUsedPercent: 90.0,
		DiskPath:        "/",
	}
	th := Thresholds{
		DiskUsedWarningPercent:  80,
		DiskUsedCriticalPercent: 90,
	}

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1 at exact critical threshold", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("alert level = %q, want critical at exact critical threshold", alerts[0].Level)
	}
}

func TestEvaluateAlerts_ZeroThresholdsDisableAlerts(t *testing.T) {
	s := Stats{
		DiskTotalBytes:  100,
		DiskFreeBytes:   1,
		DiskUsedPercent: 99.0,
		DiskPath:        "/",
		CPUCount:        1,
		LoadAvg1:        100.0,
		LoadPerCPU:      100.0,
		MemTotalBytes:   100,
		MemAvailBytes:   1,
		MemUsedPercent:  99.0,
	}
	th := Thresholds{} // all zeros

	alerts := evaluateAlerts(s, th)

	if len(alerts) != 0 {
		t.Errorf("len(alerts) = %d, want 0 when thresholds are zero (disabled)", len(alerts))
	}
}

// ---------------------------------------------------------------------------
// ShouldPauseCollection
// ---------------------------------------------------------------------------

func TestShouldPauseCollection_Critical(t *testing.T) {
	s := Stats{
		Alerts: []Alert{
			{Level: "critical", Metric: "disk", Message: "too full"},
		},
	}
	if !ShouldPauseCollection(s) {
		t.Error("ShouldPauseCollection = false, want true with critical alert")
	}
}

func TestShouldPauseCollection_WarningOnly(t *testing.T) {
	s := Stats{
		Alerts: []Alert{
			{Level: "warning", Metric: "disk", Message: "getting full"},
		},
	}
	if ShouldPauseCollection(s) {
		t.Error("ShouldPauseCollection = true, want false with only warning alerts")
	}
}

func TestShouldPauseCollection_NoAlerts(t *testing.T) {
	s := Stats{}
	if ShouldPauseCollection(s) {
		t.Error("ShouldPauseCollection = true, want false with no alerts")
	}
}

func TestShouldPauseCollection_MixedAlerts(t *testing.T) {
	s := Stats{
		Alerts: []Alert{
			{Level: "warning", Metric: "cpu", Message: "high load"},
			{Level: "critical", Metric: "disk", Message: "almost full"},
		},
	}
	if !ShouldPauseCollection(s) {
		t.Error("ShouldPauseCollection = false, want true when any alert is critical")
	}
}

// ---------------------------------------------------------------------------
// formatDuration
// ---------------------------------------------------------------------------

func TestFormatDuration_Minutes(t *testing.T) {
	got := formatDuration(5 * time.Minute)
	if got != "5m" {
		t.Errorf("formatDuration(5m) = %q, want 5m", got)
	}
}

func TestFormatDuration_Hours(t *testing.T) {
	got := formatDuration(2*time.Hour + 30*time.Minute)
	if got != "2h 30m" {
		t.Errorf("formatDuration(2h30m) = %q, want 2h 30m", got)
	}
}

func TestFormatDuration_Days(t *testing.T) {
	got := formatDuration(3*24*time.Hour + 14*time.Hour + 22*time.Minute)
	if got != "3d 14h 22m" {
		t.Errorf("formatDuration(3d14h22m) = %q, want 3d 14h 22m", got)
	}
}

func TestFormatDuration_Zero(t *testing.T) {
	got := formatDuration(0)
	if got != "0m" {
		t.Errorf("formatDuration(0) = %q, want 0m", got)
	}
}

func TestFormatDuration_LessThanMinute(t *testing.T) {
	got := formatDuration(30 * time.Second)
	if got != "0m" {
		t.Errorf("formatDuration(30s) = %q, want 0m", got)
	}
}

// ---------------------------------------------------------------------------
// DefaultThresholds
// ---------------------------------------------------------------------------

func TestDefaultThresholds(t *testing.T) {
	th := DefaultThresholds()
	if th.DiskUsedWarningPercent != 80 {
		t.Errorf("DiskUsedWarningPercent = %f, want 80", th.DiskUsedWarningPercent)
	}
	if th.DiskUsedCriticalPercent != 90 {
		t.Errorf("DiskUsedCriticalPercent = %f, want 90", th.DiskUsedCriticalPercent)
	}
	if th.CPULoadWarningPerCPU != 2.0 {
		t.Errorf("CPULoadWarningPerCPU = %f, want 2.0", th.CPULoadWarningPerCPU)
	}
	if th.CPULoadCriticalPerCPU != 4.0 {
		t.Errorf("CPULoadCriticalPerCPU = %f, want 4.0", th.CPULoadCriticalPerCPU)
	}
	if th.MemUsedWarningPercent != 80 {
		t.Errorf("MemUsedWarningPercent = %f, want 80", th.MemUsedWarningPercent)
	}
	if th.MemUsedCriticalPercent != 90 {
		t.Errorf("MemUsedCriticalPercent = %f, want 90", th.MemUsedCriticalPercent)
	}
}
