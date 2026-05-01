// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

func TestDiagnosticConfigSummary(t *testing.T) {
	trueVal := true
	falseVal := false

	cfg := config.Config{
		Organisations: []config.Organisation{
			{
				Name:                "example-corp",
				ChefServerURL:       "https://chef.example.com",
				OrgName:             "example-corp",
				ClientName:          "migration-bot",
				ClientKeyPath:       "/etc/chef/client.pem",
				ClientKeyCredential: "chef-client-key-cred",
			},
			{
				Name:                "acme",
				ChefServerURL:       "https://chef2.example.com",
				OrgName:             "acme",
				ClientName:          "migration-bot",
				ClientKeyCredential: "chef-client-key-cred-2",
			},
		},
		TargetChefVersions: []string{"17.10.0", "18.0.0"},
		GitBaseURLs:        []string{"https://git.example.com"},
		Collection: config.CollectionConfig{
			Schedule:                       "0 * * * *",
			StaleNodeThresholdDays:         7,
			StaleNodeWarningHours:          72,
			StaleNodeCriticalDays:          7,
			StaleCookbookThresholdDays:     365,
			SkipServerCookbookDownload:     false,
			DeleteServerCookbooksAfterScan: &falseVal,
		},
		Concurrency: config.ConcurrencyConfig{
			OrganisationCollection: 5,
			NodePageFetching:       10,
			GitPull:                10,
			CookbookDownload:       4,
			CookstyleScan:          8,
			TestKitchenRun:         4,
			ReadinessEvaluation:    20,
		},
		AnalysisTools: config.AnalysisToolsConfig{
			EmbeddedBinDir:          "/opt/chef-migration-metrics/embedded/bin",
			CookstyleEnabled:        &trueVal,
			CookstyleTimeoutMinutes: 10,
			TestKitchen: config.TestKitchenConfig{
				Enabled:        &trueVal,
				TimeoutMinutes: 30,
				Driver:         "vcenter",
				DriverSettings: map[string]any{
					"vcenter_host": "vcenter.internal.example.com",
					"datacenter":   "DC1",
				},
				DriverSecrets: map[string]string{
					"vcenter_password": "my-credential-name",
					"vcenter_user":     "vcenter-user-cred",
				},
				HypervisorType:             "vcenter",
				VMTTLHours:                 4,
				VMNamePrefix:               "cmm",
				MaxConcurrentVMs:           10,
				OrphanSweepIntervalMinutes: 30,
			},
		},
		Readiness: config.ReadinessConfig{
			MinFreeDiskMB: 2048,
		},
		Exports: config.ExportsConfig{
			MaxRows:         100000,
			AsyncThreshold:  10000,
			OutputDirectory: "/var/lib/chef-migration-metrics/exports",
			RetentionHours:  24,
		},
		Logging: config.LoggingConfig{
			Level:         "INFO",
			RetentionDays: 90,
		},
		Server: config.ServerConfig{
			ListenAddress:           "0.0.0.0",
			Port:                    8080,
			GracefulShutdownSeconds: 30,
			TrustedProxy:            false,
			TLS: config.TLSConfig{
				Mode:     "tls",
				CertPath: "/etc/ssl/cert.pem",
				KeyPath:  "/etc/ssl/key.pem",
				CAPath:   "/etc/ssl/ca.pem",
				ACME: config.ACMEConfig{
					Email:   "admin@example.com",
					CAURL:   "https://acme-v02.api.letsencrypt.org/directory",
					Domains: []string{"example.com"},
				},
			},
		},
		SystemHealth: config.SystemHealthConfig{
			DiskPaths:                 []string{"/var/lib"},
			DiskUsedWarningPercent:    80,
			DiskUsedCriticalPercent:   90,
			CPULoadWarningPerCPU:      1.5,
			CPULoadCriticalPerCPU:     3.0,
			MemUsedWarningPercent:     80,
			MemUsedCriticalPercent:    90,
			PauseCollectionOnCritical: &trueVal,
		},
		Performance: config.PerformanceConfig{
			Enabled:       &trueVal,
			WindowSeconds: 300,
		},
		Ownership: config.OwnershipConfig{
			Enabled: true,
			AuditLog: config.OwnershipAuditLog{
				RetentionDays: 365,
			},
		},
		Auth: config.AuthConfig{
			SessionExpiry:     "8h",
			MinPasswordLength: 12,
			LockoutAttempts:   5,
			Providers: []config.AuthProvider{
				{
					Type: "local",
				},
				{
					Type:                   "ldap",
					Host:                   "ldap.internal.example.com",
					Port:                   389,
					BaseDN:                 "dc=example,dc=com",
					BindDN:                 "cn=admin,dc=example,dc=com",
					BindPasswordEnv:        "LDAP_BIND_PASS",
					BindPasswordCredential: "ldap-pass-cred",
				},
			},
		},
		SMTP: config.SMTPConfig{
			Host:        "smtp.example.com",
			Port:        587,
			UsernameEnv: "SMTP_USER",
			PasswordEnv: "SMTP_PASS",
			FromAddress: "no-reply@example.com",
			TLS:         true,
		},
		Datastore: config.DatastoreConfig{
			URL:          "postgres://user:secret@db/chef",
			MaxOpenConns: 25,
			MaxIdleConns: 5,
		},
	}

	result := DiagnosticConfigSummary(cfg)

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	got := string(raw)

	// --- Safe fields are present with correct values ---

	t.Run("organisation_count", func(t *testing.T) {
		v, ok := result["organisation_count"]
		if !ok {
			t.Fatal("organisation_count missing from result")
		}
		if v != 2 {
			t.Fatalf("expected organisation_count=2, got %v", v)
		}
	})

	t.Run("target_chef_versions", func(t *testing.T) {
		v, ok := result["target_chef_versions"]
		if !ok {
			t.Fatal("target_chef_versions missing from result")
		}
		versions, ok := v.([]string)
		if !ok {
			t.Fatalf("expected []string, got %T", v)
		}
		if len(versions) != 2 || versions[0] != "17.10.0" || versions[1] != "18.0.0" {
			t.Fatalf("unexpected target_chef_versions: %v", versions)
		}
	})

	t.Run("collection_fields", func(t *testing.T) {
		c, ok := result["collection"].(map[string]any)
		if !ok {
			t.Fatal("collection missing or wrong type")
		}
		if c["schedule"] != "0 * * * *" {
			t.Errorf("schedule: expected %q, got %v", "0 * * * *", c["schedule"])
		}
		if c["stale_node_threshold_days"] != 7 {
			t.Errorf("stale_node_threshold_days: expected 7, got %v", c["stale_node_threshold_days"])
		}
		if c["delete_server_cookbooks_after_scan"] != false {
			t.Errorf("delete_server_cookbooks_after_scan: expected false, got %v", c["delete_server_cookbooks_after_scan"])
		}
	})

	t.Run("concurrency_fields", func(t *testing.T) {
		c, ok := result["concurrency"].(map[string]any)
		if !ok {
			t.Fatal("concurrency missing or wrong type")
		}
		if c["organisation_collection"] != 5 {
			t.Errorf("organisation_collection: expected 5, got %v", c["organisation_collection"])
		}
		if c["test_kitchen_run"] != 4 {
			t.Errorf("test_kitchen_run: expected 4, got %v", c["test_kitchen_run"])
		}
		if c["readiness_evaluation"] != 20 {
			t.Errorf("readiness_evaluation: expected 20, got %v", c["readiness_evaluation"])
		}
	})

	t.Run("analysis_tools_fields", func(t *testing.T) {
		at, ok := result["analysis_tools"].(map[string]any)
		if !ok {
			t.Fatal("analysis_tools missing or wrong type")
		}
		if at["cookstyle_enabled"] != true {
			t.Errorf("cookstyle_enabled: expected true, got %v", at["cookstyle_enabled"])
		}
		if at["embedded_bin_dir"] != "/opt/chef-migration-metrics/embedded/bin" {
			t.Errorf("embedded_bin_dir: unexpected value %v", at["embedded_bin_dir"])
		}
	})

	t.Run("test_kitchen_enabled_nil_defaults_true", func(t *testing.T) {
		cfgNilEnabled := config.Config{
			AnalysisTools: config.AnalysisToolsConfig{
				TestKitchen: config.TestKitchenConfig{Enabled: nil},
			},
		}
		r := DiagnosticConfigSummary(cfgNilEnabled)
		at := r["analysis_tools"].(map[string]any)
		tk := at["test_kitchen"].(map[string]any)
		if tk["enabled"] != true {
			t.Errorf("nil Enabled should default to true, got %v", tk["enabled"])
		}
	})

	t.Run("driver_secrets_keys_present_not_values", func(t *testing.T) {
		at, ok := result["analysis_tools"].(map[string]any)
		if !ok {
			t.Fatal("analysis_tools missing")
		}
		tk, ok := at["test_kitchen"].(map[string]any)
		if !ok {
			t.Fatal("test_kitchen missing")
		}
		keys, ok := tk["driver_secrets_keys"].([]string)
		if !ok {
			t.Fatalf("driver_secrets_keys wrong type: %T", tk["driver_secrets_keys"])
		}
		if len(keys) != 2 {
			t.Fatalf("expected 2 driver_secrets_keys, got %d: %v", len(keys), keys)
		}
		// Keys must be sorted.
		if keys[0] != "vcenter_password" || keys[1] != "vcenter_user" {
			t.Errorf("unexpected driver_secrets_keys: %v", keys)
		}
	})

	t.Run("driver_settings_keys_sorted", func(t *testing.T) {
		at := result["analysis_tools"].(map[string]any)
		tk := at["test_kitchen"].(map[string]any)
		keys, ok := tk["driver_settings_keys"].([]string)
		if !ok {
			t.Fatalf("driver_settings_keys wrong type: %T", tk["driver_settings_keys"])
		}
		if len(keys) != 2 || keys[0] != "datacenter" || keys[1] != "vcenter_host" {
			t.Errorf("unexpected driver_settings_keys: %v", keys)
		}
	})

	t.Run("server_fields", func(t *testing.T) {
		s, ok := result["server"].(map[string]any)
		if !ok {
			t.Fatal("server missing or wrong type")
		}
		if s["listen_address"] != "0.0.0.0" {
			t.Errorf("listen_address: expected 0.0.0.0, got %v", s["listen_address"])
		}
		if s["port"] != 8080 {
			t.Errorf("port: expected 8080, got %v", s["port"])
		}
		if s["tls_mode"] != "tls" {
			t.Errorf("tls_mode: expected tls, got %v", s["tls_mode"])
		}
	})

	t.Run("exports_no_output_directory", func(t *testing.T) {
		e, ok := result["exports"].(map[string]any)
		if !ok {
			t.Fatal("exports missing or wrong type")
		}
		if _, hasDir := e["output_directory"]; hasDir {
			t.Error("exports must not contain output_directory")
		}
		if e["max_rows"] != 100000 {
			t.Errorf("max_rows: expected 100000, got %v", e["max_rows"])
		}
	})

	t.Run("auth_provider_types_present_no_host_dn", func(t *testing.T) {
		auth, ok := result["auth"].(map[string]any)
		if !ok {
			t.Fatal("auth missing or wrong type")
		}
		types, ok := auth["provider_types"].([]string)
		if !ok {
			t.Fatalf("provider_types wrong type: %T", auth["provider_types"])
		}
		if len(types) != 2 || types[0] != "local" || types[1] != "ldap" {
			t.Errorf("unexpected provider_types: %v", types)
		}
		// Host, BaseDN, BindDN must not appear.
		for _, forbidden := range []string{"host", "base_dn", "bind_dn", "bind_password"} {
			if strings.Contains(got, `"`+forbidden+`"`) {
				t.Errorf("auth field %q must not appear in output", forbidden)
			}
		}
	})

	t.Run("performance_fields", func(t *testing.T) {
		p, ok := result["performance"].(map[string]any)
		if !ok {
			t.Fatal("performance missing or wrong type")
		}
		if p["window_seconds"] != 300 {
			t.Errorf("window_seconds: expected 300, got %v", p["window_seconds"])
		}
		if p["enabled"] != true {
			t.Errorf("enabled: expected true, got %v", p["enabled"])
		}
	})

	t.Run("ownership_fields", func(t *testing.T) {
		o, ok := result["ownership"].(map[string]any)
		if !ok {
			t.Fatal("ownership missing or wrong type")
		}
		if o["enabled"] != true {
			t.Errorf("enabled: expected true, got %v", o["enabled"])
		}
		if o["audit_log_retention_days"] != 365 {
			t.Errorf("audit_log_retention_days: expected 365, got %v", o["audit_log_retention_days"])
		}
	})

	// --- Secret fields must NOT appear anywhere in the JSON output ---

	t.Run("no_secrets_in_output", func(t *testing.T) {
		secrets := []struct {
			name  string
			value string
		}{
			{"org name", "example-corp"},
			{"chef server url", "https://chef.example.com"},
			{"db url", "postgres://user:secret@db/chef"},
			{"client key path", "/etc/chef/client.pem"},
			{"driver secret value", "my-credential-name"},
			{"smtp password env", "SMTP_PASS"},
			{"bind password credential", "ldap-pass-cred"},
			{"tls key path", "/etc/ssl/key.pem"},
			{"acme email", "admin@example.com"},
		}
		for _, s := range secrets {
			if strings.Contains(got, s.value) {
				t.Errorf("secret %s (%q) found in output JSON", s.name, s.value)
			}
		}
	})
}
