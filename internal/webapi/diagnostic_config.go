// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// DiagnosticConfigSummary returns a safe subset of cfg suitable for
// diagnostic endpoints. It uses an explicit allowlist — only the listed
// fields are included. Secrets (DB URLs, key paths, credentials, SMTP
// settings, TLS cert paths, ACME config, etc.) are never returned.
func DiagnosticConfigSummary(cfg config.Config) map[string]any {
	return map[string]any{
		"organisation_count":   len(cfg.Organisations),
		"target_chef_versions": cfg.TargetChefVersions,
		"git_base_urls":        cfg.GitBaseURLs,
		"collection":           diagnosticCollection(cfg),
		"concurrency":          diagnosticConcurrency(cfg),
		"analysis_tools":       diagnosticAnalysisTools(cfg),
		"readiness":            diagnosticReadiness(cfg),
		"exports":              diagnosticExports(cfg),
		"logging":              diagnosticLogging(cfg),
		"server":               diagnosticServer(cfg),
		"system_health":        diagnosticSystemHealth(cfg),
		"performance":          diagnosticPerformance(cfg),
		"ownership":            diagnosticOwnership(cfg),
		"auth":                 diagnosticAuth(cfg),
	}
}

func diagnosticCollection(cfg config.Config) map[string]any {
	return map[string]any{
		"schedule":                           cfg.Collection.Schedule,
		"stale_node_threshold_days":          cfg.Collection.StaleNodeThresholdDays,
		"stale_node_warning_hours":           cfg.Collection.StaleNodeWarningHours,
		"stale_node_critical_days":           cfg.Collection.StaleNodeCriticalDays,
		"stale_cookbook_threshold_days":      cfg.Collection.StaleCookbookThresholdDays,
		"skip_server_cookbook_download":      cfg.Collection.SkipServerCookbookDownload,
		"delete_server_cookbooks_after_scan": cfg.Collection.DeleteServerCookbooksAfterScanEnabled(),
	}
}

func diagnosticConcurrency(cfg config.Config) map[string]any {
	return map[string]any{
		"organisation_collection": cfg.Concurrency.OrganisationCollection,
		"node_page_fetching":      cfg.Concurrency.NodePageFetching,
		"git_pull":                cfg.Concurrency.GitPull,
		"cookbook_download":       cfg.Concurrency.CookbookDownload,
		"cookstyle_scan":          cfg.Concurrency.CookstyleScan,
		"readiness_evaluation":    cfg.Concurrency.ReadinessEvaluation,
	}
}

func diagnosticAnalysisTools(cfg config.Config) map[string]any {
	tk := cfg.AnalysisTools.TestKitchen
	return map[string]any{
		"embedded_bin_dir":          cfg.AnalysisTools.EmbeddedBinDir,
		"cookstyle_enabled":         cfg.AnalysisTools.IsCookstyleEnabled(),
		"cookstyle_timeout_minutes": cfg.AnalysisTools.CookstyleTimeoutMinutes,
		"test_kitchen": map[string]any{
			"enabled":                       tk.IsEnabled(),
			"timeout_minutes":               tk.TimeoutMinutes,
			"driver":                        tk.Driver,
			"driver_settings_keys":          sortedMapKeys(tk.DriverSettings),
			"driver_secrets_keys":           sortedStringMapKeys(tk.DriverSecrets),
			"hypervisor_type":               tk.HypervisorType,
			"vm_ttl_hours":                  tk.VMTTLHours,
			"vm_name_prefix":                tk.VMNamePrefix,
			"max_concurrent_vms":            tk.MaxConcurrentVMs,
			"orphan_sweep_interval_minutes": tk.OrphanSweepIntervalMinutes,
		},
	}
}

func diagnosticReadiness(cfg config.Config) map[string]any {
	return map[string]any{
		"min_free_disk_mb": cfg.Readiness.MinFreeDiskMB,
	}
}

func diagnosticExports(cfg config.Config) map[string]any {
	return map[string]any{
		"max_rows":        cfg.Exports.MaxRows,
		"async_threshold": cfg.Exports.AsyncThreshold,
		"retention_hours": cfg.Exports.RetentionHours,
	}
}

func diagnosticLogging(cfg config.Config) map[string]any {
	return map[string]any{
		"level":          cfg.Logging.Level,
		"retention_days": cfg.Logging.RetentionDays,
	}
}

func diagnosticServer(cfg config.Config) map[string]any {
	return map[string]any{
		"listen_address":            cfg.Server.ListenAddress,
		"port":                      cfg.Server.Port,
		"tls_mode":                  cfg.Server.TLS.Mode,
		"graceful_shutdown_seconds": cfg.Server.GracefulShutdownSeconds,
		"trusted_proxy":             cfg.Server.TrustedProxy,
	}
}

func diagnosticSystemHealth(cfg config.Config) map[string]any {
	return map[string]any{
		"disk_paths":                   cfg.SystemHealth.DiskPaths,
		"disk_used_warning_percent":    cfg.SystemHealth.DiskUsedWarningPercent,
		"disk_used_critical_percent":   cfg.SystemHealth.DiskUsedCriticalPercent,
		"cpu_load_warning_per_cpu":     cfg.SystemHealth.CPULoadWarningPerCPU,
		"cpu_load_critical_per_cpu":    cfg.SystemHealth.CPULoadCriticalPerCPU,
		"mem_used_warning_percent":     cfg.SystemHealth.MemUsedWarningPercent,
		"mem_used_critical_percent":    cfg.SystemHealth.MemUsedCriticalPercent,
		"pause_collection_on_critical": cfg.SystemHealth.IsPauseCollectionOnCritical(),
	}
}

func diagnosticPerformance(cfg config.Config) map[string]any {
	return map[string]any{
		"window_seconds": cfg.Performance.WindowSeconds,
		"enabled":        cfg.Performance.IsEnabled(),
	}
}

func diagnosticOwnership(cfg config.Config) map[string]any {
	return map[string]any{
		"enabled":                  cfg.Ownership.Enabled,
		"audit_log_retention_days": cfg.Ownership.AuditLog.RetentionDays,
	}
}

func diagnosticAuth(cfg config.Config) map[string]any {
	types := make([]string, 0, len(cfg.Auth.Providers))
	for _, p := range cfg.Auth.Providers {
		types = append(types, p.Type)
	}
	return map[string]any{
		"session_expiry":      cfg.Auth.SessionExpiry,
		"min_password_length": cfg.Auth.MinPasswordLength,
		"lockout_attempts":    cfg.Auth.LockoutAttempts,
		"provider_types":      types,
	}
}

// sortedMapKeys returns the sorted keys of a map[string]any.
func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedStringMapKeys returns the sorted keys of a map[string]string.
func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
