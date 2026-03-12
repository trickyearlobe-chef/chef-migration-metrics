package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper: minimal valid YAML that passes all validation
// ---------------------------------------------------------------------------

func minimalValidYAML() string {
	return `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test-client
    client_key_credential: test-key

target_chef_versions:
  - "18.5.0"

datastore:
  url: postgres://localhost:5432/test
`
}

// mustParse is a test helper that parses YAML and fails the test on error.
func mustParse(t *testing.T, yamlStr string) *Config {
	t.Helper()
	cfg, _, err := Parse([]byte(yamlStr))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	return cfg
}

// expectParseError parses YAML and asserts that a validation error containing
// substr is returned.
func expectParseError(t *testing.T, yamlStr string, substr string) {
	t.Helper()
	_, _, err := Parse([]byte(yamlStr))
	if err == nil {
		t.Fatalf("expected validation error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}

// ---------------------------------------------------------------------------
// Parse / Load basics
// ---------------------------------------------------------------------------

func TestParse_MinimalValid(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if len(cfg.Organisations) != 1 {
		t.Fatalf("expected 1 organisation, got %d", len(cfg.Organisations))
	}
	if cfg.Organisations[0].Name != "test-org" {
		t.Errorf("expected org name 'test-org', got %q", cfg.Organisations[0].Name)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, _, err := Parse([]byte("{{{{not yaml"))
	if err == nil {
		t.Fatal("expected YAML parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parsing configuration YAML") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_MissingPath(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_CONFIG", "")
	_, _, err := Load("")
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, _, err := Load("/tmp/nonexistent-config-file-abc123.yml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestLoad_FromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(minimalValidYAML()), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHEF_MIGRATION_METRICS_CONFIG", path)
	cfg, _, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Organisations[0].Name != "test-org" {
		t.Error("config not loaded from env path")
	}
}

func TestLoad_FromExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(minimalValidYAML()), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Organisations) != 1 {
		t.Error("config not loaded from explicit path")
	}
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

func TestDefaults_CollectionSchedule(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Collection.Schedule != "0 * * * *" {
		t.Errorf("expected default schedule '0 * * * *', got %q", cfg.Collection.Schedule)
	}
}

func TestDefaults_StaleNodeThresholdDays(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Collection.StaleNodeThresholdDays != 7 {
		t.Errorf("expected default stale_node_threshold_days 7, got %d", cfg.Collection.StaleNodeThresholdDays)
	}
}

func TestDefaults_StaleCookbookThresholdDays(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Collection.StaleCookbookThresholdDays != 365 {
		t.Errorf("expected default stale_cookbook_threshold_days 365, got %d", cfg.Collection.StaleCookbookThresholdDays)
	}
}

func TestDefaults_Concurrency(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	checks := []struct {
		name string
		got  int
		want int
	}{
		{"organisation_collection", cfg.Concurrency.OrganisationCollection, 5},
		{"node_page_fetching", cfg.Concurrency.NodePageFetching, 10},
		{"git_pull", cfg.Concurrency.GitPull, 10},
		{"cookstyle_scan", cfg.Concurrency.CookstyleScan, 8},
		{"test_kitchen_run", cfg.Concurrency.TestKitchenRun, 4},
		{"readiness_evaluation", cfg.Concurrency.ReadinessEvaluation, 20},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("concurrency.%s: expected %d, got %d", c.name, c.want, c.got)
		}
	}
}

func TestDefaults_AnalysisTools(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.AnalysisTools.EmbeddedBinDir != "/opt/chef-migration-metrics/embedded/bin" {
		t.Errorf("unexpected embedded_bin_dir: %q", cfg.AnalysisTools.EmbeddedBinDir)
	}
	if cfg.AnalysisTools.CookstyleTimeoutMinutes != 10 {
		t.Errorf("expected cookstyle_timeout_minutes 10, got %d", cfg.AnalysisTools.CookstyleTimeoutMinutes)
	}
	if cfg.AnalysisTools.TestKitchenTimeoutMinutes != 30 {
		t.Errorf("expected test_kitchen_timeout_minutes 30, got %d", cfg.AnalysisTools.TestKitchenTimeoutMinutes)
	}
}

func TestDefaults_CookstyleEnabled(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if !cfg.AnalysisTools.IsCookstyleEnabled() {
		t.Error("expected cookstyle_enabled to default to true")
	}
	if cfg.AnalysisTools.CookstyleEnabled == nil {
		t.Error("expected CookstyleEnabled pointer to be set after defaults are applied")
	}
}

func TestCookstyleEnabled_ExplicitFalse(t *testing.T) {
	yaml := minimalValidYAML() + `
analysis_tools:
  cookstyle_enabled: false
`
	cfg := mustParse(t, yaml)
	if cfg.AnalysisTools.IsCookstyleEnabled() {
		t.Error("expected cookstyle_enabled to be false when explicitly set")
	}
}

func TestCookstyleEnabled_ExplicitTrue(t *testing.T) {
	yaml := minimalValidYAML() + `
analysis_tools:
  cookstyle_enabled: true
`
	cfg := mustParse(t, yaml)
	if !cfg.AnalysisTools.IsCookstyleEnabled() {
		t.Error("expected cookstyle_enabled to be true when explicitly set")
	}
}

func TestCookstyleIsEnabled_NilPointer(t *testing.T) {
	// IsCookstyleEnabled should return true even when the pointer is nil
	// (before defaults are applied) to match the documented default behaviour.
	a := AnalysisToolsConfig{}
	if !a.IsCookstyleEnabled() {
		t.Error("expected IsCookstyleEnabled() to return true when CookstyleEnabled is nil")
	}
}

func TestDefaults_TestKitchenEnabled(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if !cfg.AnalysisTools.TestKitchen.IsEnabled() {
		t.Error("expected test_kitchen.enabled to default to true")
	}
	if cfg.AnalysisTools.TestKitchen.Enabled == nil {
		t.Error("expected Enabled pointer to be set after defaults are applied")
	}
}

func TestTestKitchenEnabled_ExplicitFalse(t *testing.T) {
	yaml := minimalValidYAML() + `
analysis_tools:
  test_kitchen:
    enabled: false
`
	cfg := mustParse(t, yaml)
	if cfg.AnalysisTools.TestKitchen.IsEnabled() {
		t.Error("expected test_kitchen.enabled to be false when explicitly set")
	}
}

func TestTestKitchenEnabled_ExplicitTrue(t *testing.T) {
	yaml := minimalValidYAML() + `
analysis_tools:
  test_kitchen:
    enabled: true
`
	cfg := mustParse(t, yaml)
	if !cfg.AnalysisTools.TestKitchen.IsEnabled() {
		t.Error("expected test_kitchen.enabled to be true when explicitly set")
	}
}

func TestTestKitchenIsEnabled_NilPointer(t *testing.T) {
	// IsEnabled should return true even when the pointer is nil (before
	// defaults are applied) to match the documented default behaviour.
	tk := TestKitchenConfig{}
	if !tk.IsEnabled() {
		t.Error("expected IsEnabled() to return true when Enabled is nil")
	}
}

func TestDefaults_Readiness(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Readiness.MinFreeDiskMB != 2048 {
		t.Errorf("expected min_free_disk_mb 2048, got %d", cfg.Readiness.MinFreeDiskMB)
	}
}

func TestDefaults_Notifications(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	expected := []int{50, 75, 90, 100}
	if len(cfg.Notifications.ReadinessMilestones) != len(expected) {
		t.Fatalf("expected %d milestones, got %d", len(expected), len(cfg.Notifications.ReadinessMilestones))
	}
	for i, v := range expected {
		if cfg.Notifications.ReadinessMilestones[i] != v {
			t.Errorf("milestone[%d]: expected %d, got %d", i, v, cfg.Notifications.ReadinessMilestones[i])
		}
	}
	if cfg.Notifications.StaleNodeAlertCount != 50 {
		t.Errorf("expected stale_node_alert_count 50, got %d", cfg.Notifications.StaleNodeAlertCount)
	}
}

func TestDefaults_SMTP(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.SMTP.Port != 587 {
		t.Errorf("expected SMTP port 587, got %d", cfg.SMTP.Port)
	}
	if !cfg.SMTP.TLS {
		t.Error("expected SMTP TLS default true")
	}
}

func TestDefaults_Exports(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Exports.MaxRows != 100000 {
		t.Errorf("expected max_rows 100000, got %d", cfg.Exports.MaxRows)
	}
	if cfg.Exports.AsyncThreshold != 10000 {
		t.Errorf("expected async_threshold 10000, got %d", cfg.Exports.AsyncThreshold)
	}
	if cfg.Exports.OutputDirectory != "/var/lib/chef-migration-metrics/exports" {
		t.Errorf("unexpected output_directory: %q", cfg.Exports.OutputDirectory)
	}
	if cfg.Exports.RetentionHours != 24 {
		t.Errorf("expected retention_hours 24, got %d", cfg.Exports.RetentionHours)
	}
}

func TestDefaults_Elasticsearch(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Elasticsearch.OutputDirectory != "/var/lib/chef-migration-metrics/elasticsearch" {
		t.Errorf("unexpected elasticsearch output_directory: %q", cfg.Elasticsearch.OutputDirectory)
	}
	if cfg.Elasticsearch.RetentionHours != 48 {
		t.Errorf("expected elasticsearch retention_hours 48, got %d", cfg.Elasticsearch.RetentionHours)
	}
}

func TestDefaults_Server(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.ListenAddress != "0.0.0.0" {
		t.Errorf("expected listen_address '0.0.0.0', got %q", cfg.Server.ListenAddress)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.GracefulShutdownSeconds != 30 {
		t.Errorf("expected graceful_shutdown_seconds 30, got %d", cfg.Server.GracefulShutdownSeconds)
	}
}

func TestDefaults_TLS(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.Mode != "off" {
		t.Errorf("expected tls.mode 'off', got %q", cfg.Server.TLS.Mode)
	}
	if cfg.Server.TLS.MinVersion != "1.2" {
		t.Errorf("expected tls.min_version '1.2', got %q", cfg.Server.TLS.MinVersion)
	}
}

func TestDefaults_ACME(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.ACME.CAURL != "https://acme-v02.api.letsencrypt.org/directory" {
		t.Errorf("unexpected acme.ca_url: %q", cfg.Server.TLS.ACME.CAURL)
	}
	if cfg.Server.TLS.ACME.Challenge != "http-01" {
		t.Errorf("expected acme.challenge 'http-01', got %q", cfg.Server.TLS.ACME.Challenge)
	}
	if cfg.Server.TLS.ACME.StoragePath != "/var/lib/chef-migration-metrics/acme" {
		t.Errorf("unexpected acme.storage_path: %q", cfg.Server.TLS.ACME.StoragePath)
	}
	if cfg.Server.TLS.ACME.RenewBeforeDays != 30 {
		t.Errorf("expected acme.renew_before_days 30, got %d", cfg.Server.TLS.ACME.RenewBeforeDays)
	}
}

func TestDefaults_Frontend(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Frontend.BasePath != "/" {
		t.Errorf("expected base_path '/', got %q", cfg.Frontend.BasePath)
	}
}

func TestDefaults_Logging(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Logging.Level != "INFO" {
		t.Errorf("expected logging.level 'INFO', got %q", cfg.Logging.Level)
	}
	if cfg.Logging.RetentionDays != 90 {
		t.Errorf("expected retention_days 90, got %d", cfg.Logging.RetentionDays)
	}
}

func TestDefaults_CredentialEncryptionKeyEnv(t *testing.T) {
	cfg := mustParse(t, minimalValidYAML())
	if cfg.CredentialEncryptionKeyEnv != "CMM_CREDENTIAL_ENCRYPTION_KEY" {
		t.Errorf("unexpected credential_encryption_key_env: %q", cfg.CredentialEncryptionKeyEnv)
	}
}

func TestDefaults_Datastore(t *testing.T) {
	// Config with no datastore section should get the default URL
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test-client
    client_key_credential: test-key
`
	cfg := mustParse(t, yaml)
	if cfg.Datastore.URL != "postgres://localhost:5432/chef_migration_metrics" {
		t.Errorf("unexpected default datastore URL: %q", cfg.Datastore.URL)
	}
}

// ---------------------------------------------------------------------------
// Default overrides (non-zero values in YAML override defaults)
// ---------------------------------------------------------------------------

func TestOverrides_ExplicitValues(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test-client
    client_key_credential: test-key

collection:
  schedule: "*/5 * * * *"
  stale_node_threshold_days: 14
  stale_cookbook_threshold_days: 180

concurrency:
  organisation_collection: 2
  node_page_fetching: 3

server:
  port: 9090
  listen_address: "127.0.0.1"

logging:
  level: DEBUG
  retention_days: 30
`
	cfg := mustParse(t, yaml)
	if cfg.Collection.Schedule != "*/5 * * * *" {
		t.Errorf("schedule not overridden: %q", cfg.Collection.Schedule)
	}
	if cfg.Collection.StaleNodeThresholdDays != 14 {
		t.Errorf("stale_node_threshold_days not overridden: %d", cfg.Collection.StaleNodeThresholdDays)
	}
	if cfg.Collection.StaleCookbookThresholdDays != 180 {
		t.Errorf("stale_cookbook_threshold_days not overridden: %d", cfg.Collection.StaleCookbookThresholdDays)
	}
	if cfg.Concurrency.OrganisationCollection != 2 {
		t.Errorf("concurrency not overridden: %d", cfg.Concurrency.OrganisationCollection)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port not overridden: %d", cfg.Server.Port)
	}
	if cfg.Server.ListenAddress != "127.0.0.1" {
		t.Errorf("listen_address not overridden: %q", cfg.Server.ListenAddress)
	}
	if cfg.Logging.Level != "DEBUG" {
		t.Errorf("logging level not overridden: %q", cfg.Logging.Level)
	}
}

// ---------------------------------------------------------------------------
// Environment variable overrides
// ---------------------------------------------------------------------------

func TestEnvOverride_DatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://env-host:5432/envdb")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Datastore.URL != "postgres://env-host:5432/envdb" {
		t.Errorf("DATABASE_URL override not applied: %q", cfg.Datastore.URL)
	}
}

func TestEnvOverride_ServerPort(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_PORT", "443")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.Port != 443 {
		t.Errorf("server port env override not applied: %d", cfg.Server.Port)
	}
}

func TestEnvOverride_ServerPort_InvalidIgnored(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_PORT", "notanumber")
	cfg := mustParse(t, minimalValidYAML())
	// Should keep the default since the env var is not a valid int
	if cfg.Server.Port != 8080 {
		t.Errorf("invalid port env should be ignored, got %d", cfg.Server.Port)
	}
}

func TestEnvOverride_TLSMode(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_MODE", "off")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.Mode != "off" {
		t.Errorf("TLS mode env override not applied: %q", cfg.Server.TLS.Mode)
	}
}

func TestEnvOverride_TLSCertPath(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_PATH", "/tmp/cert.pem")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.CertPath != "/tmp/cert.pem" {
		t.Errorf("cert_path env override not applied: %q", cfg.Server.TLS.CertPath)
	}
}

func TestEnvOverride_TLSKeyPath(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_KEY_PATH", "/tmp/key.pem")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.KeyPath != "/tmp/key.pem" {
		t.Errorf("key_path env override not applied: %q", cfg.Server.TLS.KeyPath)
	}
}

func TestEnvOverride_TLSCAPath(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_CA_PATH", "/tmp/ca.pem")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.CAPath != "/tmp/ca.pem" {
		t.Errorf("ca_path env override not applied: %q", cfg.Server.TLS.CAPath)
	}
}

func TestEnvOverride_TLSMinVersion(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_MIN_VERSION", "1.3")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.MinVersion != "1.3" {
		t.Errorf("min_version env override not applied: %q", cfg.Server.TLS.MinVersion)
	}
}

func TestEnvOverride_HTTPRedirectPort(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_HTTP_REDIRECT_PORT", "80")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.HTTPRedirectPort != 80 {
		t.Errorf("http_redirect_port env override not applied: %d", cfg.Server.TLS.HTTPRedirectPort)
	}
}

func TestEnvOverride_ACMEEmail(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_EMAIL", "test@example.com")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.ACME.Email != "test@example.com" {
		t.Errorf("acme email env override not applied: %q", cfg.Server.TLS.ACME.Email)
	}
}

func TestEnvOverride_ACMECAURL(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CA_URL", "https://staging.example.com")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.ACME.CAURL != "https://staging.example.com" {
		t.Errorf("acme ca_url env override not applied: %q", cfg.Server.TLS.ACME.CAURL)
	}
}

func TestEnvOverride_ACMEChallenge(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CHALLENGE", "dns-01")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.ACME.Challenge != "dns-01" {
		t.Errorf("acme challenge env override not applied: %q", cfg.Server.TLS.ACME.Challenge)
	}
}

func TestEnvOverride_ACMEDNSProvider(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_DNS_PROVIDER", "route53")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.ACME.DNSProvider != "route53" {
		t.Errorf("acme dns_provider env override not applied: %q", cfg.Server.TLS.ACME.DNSProvider)
	}
}

func TestEnvOverride_ACMEStoragePath(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_STORAGE_PATH", "/tmp/acme")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.Server.TLS.ACME.StoragePath != "/tmp/acme" {
		t.Errorf("acme storage_path env override not applied: %q", cfg.Server.TLS.ACME.StoragePath)
	}
}

func TestEnvOverride_ACMEAgreeToTOS(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS", "true")
	cfg := mustParse(t, minimalValidYAML())
	if !cfg.Server.TLS.ACME.AgreeToTOS {
		t.Error("acme agree_to_tos env override not applied")
	}
}

func TestEnvOverride_ACMEAgreeToTOS_CaseInsensitive(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS", "TRUE")
	cfg := mustParse(t, minimalValidYAML())
	if !cfg.Server.TLS.ACME.AgreeToTOS {
		t.Error("acme agree_to_tos env override should be case-insensitive")
	}
}

func TestEnvOverride_AnalysisToolsEmbeddedBinDir(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_ANALYSIS_TOOLS_EMBEDDED_BIN_DIR", "/custom/bin")
	cfg := mustParse(t, minimalValidYAML())
	if cfg.AnalysisTools.EmbeddedBinDir != "/custom/bin" {
		t.Errorf("embedded_bin_dir env override not applied: %q", cfg.AnalysisTools.EmbeddedBinDir)
	}
}

func TestEnvOverride_ElasticsearchEnabled(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_ELASTICSEARCH_ENABLED", "true")
	// Don't call mustParse because elasticsearch validation may fail on
	// output_directory. Just check the override is applied.
	var cfg Config
	cfg.setDefaults()
	cfg.applyEnvOverrides()
	if !cfg.Elasticsearch.Enabled {
		t.Error("elasticsearch.enabled env override not applied")
	}
}

func TestEnvOverride_ElasticsearchOutputDirectory(t *testing.T) {
	t.Setenv("CHEF_MIGRATION_METRICS_ELASTICSEARCH_OUTPUT_DIRECTORY", "/custom/es")
	var cfg Config
	cfg.setDefaults()
	cfg.applyEnvOverrides()
	if cfg.Elasticsearch.OutputDirectory != "/custom/es" {
		t.Errorf("elasticsearch output_directory env override not applied: %q", cfg.Elasticsearch.OutputDirectory)
	}
}

// ---------------------------------------------------------------------------
// Organisation validation
// ---------------------------------------------------------------------------

func TestValidation_NoOrganisations(t *testing.T) {
	yaml := `
target_chef_versions:
  - "18.5.0"
`
	expectParseError(t, yaml, "at least one organisation must be configured")
}

func TestValidation_OrgMissingName(t *testing.T) {
	yaml := `
organisations:
  - chef_server_url: https://chef.example.com
    org_name: test
    client_name: test
    client_key_credential: test-key
`
	expectParseError(t, yaml, "name is required")
}

func TestValidation_OrgDuplicateName(t *testing.T) {
	yaml := `
organisations:
  - name: dup
    chef_server_url: https://chef.example.com
    org_name: org1
    client_name: test
    client_key_credential: k1
  - name: dup
    chef_server_url: https://chef.example.com
    org_name: org2
    client_name: test
    client_key_credential: k2
`
	expectParseError(t, yaml, "duplicate organisation name")
}

func TestValidation_OrgMissingChefServerURL(t *testing.T) {
	yaml := `
organisations:
  - name: test
    org_name: test
    client_name: test
    client_key_credential: test-key
`
	expectParseError(t, yaml, "chef_server_url is required")
}

func TestValidation_OrgMissingOrgName(t *testing.T) {
	yaml := `
organisations:
  - name: test
    chef_server_url: https://chef.example.com
    client_name: test
    client_key_credential: test-key
`
	expectParseError(t, yaml, "org_name is required")
}

func TestValidation_OrgMissingClientName(t *testing.T) {
	yaml := `
organisations:
  - name: test
    chef_server_url: https://chef.example.com
    org_name: test
    client_key_credential: test-key
`
	expectParseError(t, yaml, "client_name is required")
}

func TestValidation_OrgMissingBothKeys(t *testing.T) {
	yaml := `
organisations:
  - name: test
    chef_server_url: https://chef.example.com
    org_name: test
    client_name: test
`
	expectParseError(t, yaml, "one of client_key_path or client_key_credential is required")
}

func TestValidation_OrgKeyPathNotFound(t *testing.T) {
	yaml := `
organisations:
  - name: test
    chef_server_url: https://chef.example.com
    org_name: test
    client_name: test
    client_key_path: /nonexistent/key.pem
`
	expectParseError(t, yaml, "client_key_path")
}

func TestValidation_OrgKeyPathExists(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "client.pem")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0600); err != nil {
		t.Fatal(err)
	}
	yaml := `
organisations:
  - name: test
    chef_server_url: https://chef.example.com
    org_name: test
    client_name: test
    client_key_path: ` + keyFile + `
`
	mustParse(t, yaml)
}

func TestValidation_OrgCredentialOnly(t *testing.T) {
	// client_key_credential is sufficient — no file needed
	mustParse(t, minimalValidYAML())
}

// ---------------------------------------------------------------------------
// Organisation SSLVerify
// ---------------------------------------------------------------------------

func TestOrganisation_SSLVerifyEnabled_DefaultTrue(t *testing.T) {
	// When ssl_verify is not set (nil), it should default to true.
	cfg := mustParse(t, minimalValidYAML())
	if !cfg.Organisations[0].SSLVerifyEnabled() {
		t.Error("expected SSLVerifyEnabled() to return true when ssl_verify is not set")
	}
	if cfg.Organisations[0].SSLVerify != nil {
		t.Error("expected SSLVerify to be nil when not set in YAML")
	}
}

func TestOrganisation_SSLVerifyEnabled_ExplicitTrue(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test-client
    client_key_credential: test-key
    ssl_verify: true

target_chef_versions:
  - "18.5.0"

datastore:
  url: postgres://localhost:5432/test
`
	cfg := mustParse(t, yaml)
	if !cfg.Organisations[0].SSLVerifyEnabled() {
		t.Error("expected SSLVerifyEnabled() to return true when ssl_verify is explicitly true")
	}
	if cfg.Organisations[0].SSLVerify == nil {
		t.Fatal("expected SSLVerify to be non-nil when explicitly set")
	}
	if !*cfg.Organisations[0].SSLVerify {
		t.Error("expected *SSLVerify to be true")
	}
}

func TestOrganisation_SSLVerifyEnabled_ExplicitFalse(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test-client
    client_key_credential: test-key
    ssl_verify: false

target_chef_versions:
  - "18.5.0"

datastore:
  url: postgres://localhost:5432/test
`
	cfg := mustParse(t, yaml)
	if cfg.Organisations[0].SSLVerifyEnabled() {
		t.Error("expected SSLVerifyEnabled() to return false when ssl_verify is explicitly false")
	}
	if cfg.Organisations[0].SSLVerify == nil {
		t.Fatal("expected SSLVerify to be non-nil when explicitly set")
	}
	if *cfg.Organisations[0].SSLVerify {
		t.Error("expected *SSLVerify to be false")
	}
}

func TestOrganisation_SSLVerifyEnabled_MixedOrgs(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "client.pem")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0600); err != nil {
		t.Fatal(err)
	}
	yaml := `
organisations:
  - name: org-default
    chef_server_url: https://chef.example.com
    org_name: org-default
    client_name: test-client
    client_key_credential: test-key
  - name: org-no-ssl
    chef_server_url: https://chef.example.com
    org_name: org-no-ssl
    client_name: test-client
    client_key_credential: test-key
    ssl_verify: false
  - name: org-ssl
    chef_server_url: https://chef.example.com
    org_name: org-ssl
    client_name: test-client
    client_key_credential: test-key
    ssl_verify: true

target_chef_versions:
  - "18.5.0"

datastore:
  url: postgres://localhost:5432/test
`
	cfg := mustParse(t, yaml)
	if len(cfg.Organisations) != 3 {
		t.Fatalf("expected 3 organisations, got %d", len(cfg.Organisations))
	}
	if !cfg.Organisations[0].SSLVerifyEnabled() {
		t.Error("org-default: expected SSLVerifyEnabled() to return true (default)")
	}
	if cfg.Organisations[1].SSLVerifyEnabled() {
		t.Error("org-no-ssl: expected SSLVerifyEnabled() to return false")
	}
	if !cfg.Organisations[2].SSLVerifyEnabled() {
		t.Error("org-ssl: expected SSLVerifyEnabled() to return true")
	}
}

// ---------------------------------------------------------------------------
// Target version validation
// ---------------------------------------------------------------------------

func TestValidation_TargetVersionValid(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

target_chef_versions:
  - "18.5.0"
  - "19.0.0"
  - "100.200.300"
`
	mustParse(t, yaml)
}

func TestValidation_TargetVersionInvalid(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

target_chef_versions:
  - "not-a-version"
`
	expectParseError(t, yaml, "not a valid semver")
}

func TestValidation_TargetVersionPartial(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

target_chef_versions:
  - "18.5"
`
	expectParseError(t, yaml, "not a valid semver")
}

// ---------------------------------------------------------------------------
// Collection validation
// ---------------------------------------------------------------------------

func TestValidation_InvalidCronSchedule(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

collection:
  schedule: "bad cron"
`
	expectParseError(t, yaml, "not a valid cron expression")
}

func TestValidation_ValidCronExpressions(t *testing.T) {
	expressions := []string{
		"0 * * * *",
		"*/5 * * * *",
		"0 0 * * 0",
		"30 4 1,15 * *",
	}
	for _, expr := range expressions {
		yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

collection:
  schedule: "` + expr + `"
`
		mustParse(t, yaml)
	}
}

// ---------------------------------------------------------------------------
// Server / TLS validation
// ---------------------------------------------------------------------------

func TestValidation_InvalidServerPort(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  port: 99999
`
	expectParseError(t, yaml, "not a valid port number")
}

func TestValidation_InvalidTLSMode(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: "banana"
`
	expectParseError(t, yaml, "must be 'off', 'static', or 'acme'")
}

func TestValidation_TLSStaticMissingCertPath(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: static
`
	expectParseError(t, yaml, "cert_path is required")
}

func TestValidation_TLSStaticMissingKeyPath(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	os.WriteFile(certFile, []byte("fake-cert"), 0644)
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: static
    cert_path: ` + certFile + `
`
	expectParseError(t, yaml, "key_path is required")
}

func TestValidation_TLSStaticCertNotFound(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: static
    cert_path: /nonexistent/cert.pem
    key_path: /nonexistent/key.pem
`
	expectParseError(t, yaml, "cert_path")
}

func TestValidation_TLSStaticKeyPermissions(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("fake-cert"), 0644)
	os.WriteFile(keyFile, []byte("fake-key"), 0644) // too permissive

	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: static
    cert_path: ` + certFile + `
    key_path: ` + keyFile + `
`
	_, warnings, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, msg := range warnings.Messages {
		if strings.Contains(msg, "permissions") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning about key file permissions")
	}
}

func TestValidation_TLSStaticValid(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("fake-cert"), 0644)
	os.WriteFile(keyFile, []byte("fake-key"), 0600)

	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: static
    cert_path: ` + certFile + `
    key_path: ` + keyFile + `
`
	mustParse(t, yaml)
}

func TestValidation_TLSStaticCAPathNotFound(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("fake-cert"), 0644)
	os.WriteFile(keyFile, []byte("fake-key"), 0600)

	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: static
    cert_path: ` + certFile + `
    key_path: ` + keyFile + `
    ca_path: /nonexistent/ca.pem
`
	expectParseError(t, yaml, "ca_path")
}

func TestValidation_TLSMinVersion_Invalid(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("fake-cert"), 0644)
	os.WriteFile(keyFile, []byte("fake-key"), 0600)

	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: static
    cert_path: ` + certFile + `
    key_path: ` + keyFile + `
    min_version: "1.0"
`
	expectParseError(t, yaml, "must be '1.2' or '1.3'")
}

func TestValidation_TLSHTTPRedirectPortInvalid(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    http_redirect_port: 99999
`
	expectParseError(t, yaml, "http_redirect_port")
}

// ---------------------------------------------------------------------------
// ACME validation
// ---------------------------------------------------------------------------

func TestValidation_ACMEMissingDomains(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    acme:
      email: test@example.com
      agree_to_tos: true
`
	expectParseError(t, yaml, "acme.domains is required")
}

func TestValidation_ACMEMissingEmail(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    http_redirect_port: 80
    acme:
      domains:
        - example.com
      agree_to_tos: true
`
	expectParseError(t, yaml, "acme.email is required")
}

func TestValidation_ACMEAgreeToTOSFalse(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    http_redirect_port: 80
    acme:
      domains:
        - example.com
      email: test@example.com
      agree_to_tos: false
`
	expectParseError(t, yaml, "agree_to_tos must be true")
}

func TestValidation_ACMEHTTP01MissingRedirectPort(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    acme:
      domains:
        - example.com
      email: test@example.com
      agree_to_tos: true
      challenge: http-01
`
	expectParseError(t, yaml, "http_redirect_port must be set")
}

func TestValidation_ACMEDNS01MissingProvider(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    acme:
      domains:
        - example.com
      email: test@example.com
      agree_to_tos: true
      challenge: dns-01
`
	expectParseError(t, yaml, "dns_provider is required")
}

func TestValidation_ACMEInvalidChallenge(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    http_redirect_port: 80
    acme:
      domains:
        - example.com
      email: test@example.com
      agree_to_tos: true
      challenge: banana
`
	expectParseError(t, yaml, "must be 'http-01', 'tls-alpn-01', or 'dns-01'")
}

// Note: renew_before_days: 0 in YAML is indistinguishable from "not set" for
// Go int fields, so setDefaults() overwrites it to 30. We test with -1 instead.
func TestValidation_ACMERenewBeforeDaysTooLow(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    http_redirect_port: 80
    acme:
      domains:
        - example.com
      email: test@example.com
      agree_to_tos: true
      renew_before_days: -1
`
	expectParseError(t, yaml, "renew_before_days")
}

func TestValidation_ACMERenewBeforeDaysTooHigh(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    http_redirect_port: 80
    acme:
      domains:
        - example.com
      email: test@example.com
      agree_to_tos: true
      renew_before_days: 90
`
	expectParseError(t, yaml, "renew_before_days")
}

func TestValidation_ACMETrustedRootsNotFound(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    mode: acme
    http_redirect_port: 80
    acme:
      domains:
        - example.com
      email: test@example.com
      agree_to_tos: true
      trusted_roots: /nonexistent/roots.pem
`
	expectParseError(t, yaml, "trusted_roots")
}

// ---------------------------------------------------------------------------
// Backward compatibility: tls.enabled
// ---------------------------------------------------------------------------

func TestTLSBackwardCompat_EnabledTrueNoMode(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("fake-cert"), 0644)
	os.WriteFile(keyFile, []byte("fake-key"), 0600)

	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

server:
  tls:
    enabled: true
    cert_path: ` + certFile + `
    key_path: ` + keyFile + `
`
	cfg := mustParse(t, yaml)
	if cfg.Server.TLS.Mode != "static" {
		t.Errorf("expected mode 'static' from enabled:true, got %q", cfg.Server.TLS.Mode)
	}
}

func TestTLSBackwardCompat_EnabledFalseNoMode(t *testing.T) {
	yaml := minimalValidYAML() + `
server:
  tls:
    enabled: false
`
	cfg := mustParse(t, yaml)
	if cfg.Server.TLS.Mode != "off" {
		t.Errorf("expected mode 'off' from enabled:false, got %q", cfg.Server.TLS.Mode)
	}
}

func TestTLSBackwardCompat_BothEnabledAndMode_Warning(t *testing.T) {
	yaml := minimalValidYAML() + `
server:
  tls:
    enabled: true
    mode: "off"
`
	_, warnings, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, msg := range warnings.Messages {
		if strings.Contains(msg, "deprecated") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected deprecation warning when both enabled and mode are set")
	}
}

// ---------------------------------------------------------------------------
// Logging validation
// ---------------------------------------------------------------------------

func TestValidation_LoggingLevelInvalid(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

logging:
  level: VERBOSE
`
	expectParseError(t, yaml, "must be one of DEBUG, INFO, WARN, ERROR")
}

func TestValidation_LoggingLevelValid(t *testing.T) {
	for _, level := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

logging:
  level: ` + level + `
`
		mustParse(t, yaml)
	}
}

// ---------------------------------------------------------------------------
// Auth provider validation
// ---------------------------------------------------------------------------

func TestValidation_AuthLocalProvider(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

auth:
  providers:
    - type: local
`
	mustParse(t, yaml)
}

func TestValidation_AuthLDAPMissingHost(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

auth:
  providers:
    - type: ldap
      base_dn: "ou=users,dc=example,dc=com"
`
	expectParseError(t, yaml, "host is required for ldap")
}

func TestValidation_AuthLDAPMissingBaseDN(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

auth:
  providers:
    - type: ldap
      host: ldap.example.com
`
	expectParseError(t, yaml, "base_dn is required for ldap")
}

func TestValidation_AuthSAMLMissingIDPURL(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

auth:
  providers:
    - type: saml
      sp_entity_id: test
`
	expectParseError(t, yaml, "idp_metadata_url is required for saml")
}

func TestValidation_AuthSAMLMissingSPEntityID(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

auth:
  providers:
    - type: saml
      idp_metadata_url: https://idp.example.com/metadata
`
	expectParseError(t, yaml, "sp_entity_id is required for saml")
}

func TestValidation_AuthUnknownProvider(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

auth:
  providers:
    - type: oauth2
`
	expectParseError(t, yaml, "unknown provider type")
}

// ---------------------------------------------------------------------------
// Notification validation
// ---------------------------------------------------------------------------

func TestValidation_NotificationsDisabledSkipsValidation(t *testing.T) {
	// Even if channels are misconfigured, disabled = no error
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: false
  channels:
    - name: bad
      type: unknown
`
	mustParse(t, yaml)
}

func TestValidation_NotificationsWebhookMissingURL(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: test-hook
      type: webhook
      events:
        - collection_failure
`
	expectParseError(t, yaml, "webhook channel requires url or url_env")
}

func TestValidation_NotificationsWebhookURLEnvUnset(t *testing.T) {
	// Make sure the env var is unset
	t.Setenv("NONEXISTENT_WEBHOOK_URL_XYZ", "")
	os.Unsetenv("NONEXISTENT_WEBHOOK_URL_XYZ")

	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: test-hook
      type: webhook
      url_env: NONEXISTENT_WEBHOOK_URL_XYZ
      events:
        - collection_failure
`
	expectParseError(t, yaml, "references an unset environment variable")
}

func TestValidation_NotificationsWebhookURLEnvSet(t *testing.T) {
	t.Setenv("TEST_WEBHOOK_URL", "https://hooks.example.com")

	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: test-hook
      type: webhook
      url_env: TEST_WEBHOOK_URL
      events:
        - collection_failure
`
	mustParse(t, yaml)
}

func TestValidation_NotificationsWebhookDirectURL(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: test-hook
      type: webhook
      url: https://hooks.example.com/test
      events:
        - collection_failure
`
	mustParse(t, yaml)
}

func TestValidation_NotificationsEmailMissingRecipients(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: email-ch
      type: email
      events:
        - collection_failure

smtp:
  host: smtp.example.com
  from_address: test@example.com
`
	expectParseError(t, yaml, "at least one recipient")
}

func TestValidation_NotificationsEmailMissingSMTP(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: email-ch
      type: email
      recipients:
        - team@example.com
      events:
        - collection_failure
`
	expectParseError(t, yaml, "smtp.host is required")
}

func TestValidation_NotificationsEmailMissingSMTPFromAddress(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: email-ch
      type: email
      recipients:
        - team@example.com
      events:
        - collection_failure

smtp:
  host: smtp.example.com
`
	expectParseError(t, yaml, "smtp.from_address is required")
}

func TestValidation_NotificationsUnknownChannelType(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: slack
      type: slack
      events:
        - collection_failure
`
	expectParseError(t, yaml, "type must be 'webhook' or 'email'")
}

func TestValidation_NotificationsDuplicateChannelName(t *testing.T) {
	t.Setenv("TEST_HOOK_URL_A", "https://a.example.com")
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: dup
      type: webhook
      url_env: TEST_HOOK_URL_A
      events:
        - collection_failure
    - name: dup
      type: webhook
      url_env: TEST_HOOK_URL_A
      events:
        - collection_failure
`
	expectParseError(t, yaml, "duplicate channel name")
}

func TestValidation_NotificationsUnknownEvent(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: hook
      type: webhook
      url: https://example.com
      events:
        - made_up_event
`
	expectParseError(t, yaml, "unknown event type")
}

func TestValidation_NotificationsMilestonesOutOfRange(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels: []
  readiness_milestones:
    - 110
`
	expectParseError(t, yaml, "must be between 0 and 100")
}

func TestValidation_NotificationsAllValidEvents(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

notifications:
  enabled: true
  channels:
    - name: all-events
      type: webhook
      url: https://example.com/hook
      events:
        - cookbook_status_change
        - readiness_milestone
        - new_incompatible_cookbook
        - collection_failure
        - stale_node_threshold_exceeded
        - certificate_expiry_warning
`
	mustParse(t, yaml)
}

// ---------------------------------------------------------------------------
// Exports validation
// ---------------------------------------------------------------------------

func TestValidation_ExportsOutputDirNotExist(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

exports:
  output_directory: /nonexistent/exports/dir
`
	expectParseError(t, yaml, "exports.output_directory")
}

func TestValidation_ExportsWritableDir(t *testing.T) {
	dir := t.TempDir()
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

exports:
  output_directory: ` + dir + `
`
	mustParse(t, yaml)
}

// ---------------------------------------------------------------------------
// Elasticsearch validation
// ---------------------------------------------------------------------------

func TestValidation_ElasticsearchDisabledSkipsValidation(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

elasticsearch:
  enabled: false
  output_directory: /nonexistent/dir
`
	mustParse(t, yaml)
}

func TestValidation_ElasticsearchEnabledBadDir(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

elasticsearch:
  enabled: true
  output_directory: /nonexistent/es/dir
`
	expectParseError(t, yaml, "elasticsearch.output_directory")
}

// Note: retention_hours: 0 in YAML is indistinguishable from "not set" for Go
// int fields, so setDefaults() overwrites it to 48. We cannot test that 0 is
// rejected — it simply gets defaulted. The spec constraint (>= 1) is
// effectively enforced by the default. A negative value in YAML would parse as
// a negative int, so we test that instead.
func TestValidation_ElasticsearchEnabledRetentionNegative(t *testing.T) {
	dir := t.TempDir()
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

elasticsearch:
  enabled: true
  output_directory: ` + dir + `
  retention_hours: -1
`
	expectParseError(t, yaml, "elasticsearch.retention_hours must be >= 1")
}

// ---------------------------------------------------------------------------
// Analysis tools validation
// ---------------------------------------------------------------------------

// Note: timeout values of 0 in YAML are indistinguishable from "not set" for
// Go int fields, so setDefaults() overwrites them to 10/30 respectively. We
// test with negative values instead, which are distinguishable.
func TestValidation_AnalysisToolsCookstyleTimeoutNegative(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

analysis_tools:
  cookstyle_timeout_minutes: -1
`
	expectParseError(t, yaml, "cookstyle_timeout_minutes must be >= 1")
}

func TestValidation_AnalysisToolsTKTimeoutNegative(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

analysis_tools:
  test_kitchen_timeout_minutes: -1
`
	expectParseError(t, yaml, "test_kitchen_timeout_minutes must be >= 1")
}

func TestValidation_AnalysisToolsEmbeddedBinDirWarning(t *testing.T) {
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

analysis_tools:
  embedded_bin_dir: /nonexistent/embedded/bin
`
	_, warnings, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, msg := range warnings.Messages {
		if strings.Contains(msg, "embedded_bin_dir") && strings.Contains(msg, "falling back") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning about nonexistent embedded_bin_dir")
	}
}

func TestValidation_AnalysisToolsEmbeddedBinDirExists(t *testing.T) {
	dir := t.TempDir()
	yaml := `
organisations:
  - name: test-org
    chef_server_url: https://chef.example.com
    org_name: test-org
    client_name: test
    client_key_credential: k

analysis_tools:
  embedded_bin_dir: ` + dir + `
`
	_, warnings, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, msg := range warnings.Messages {
		if strings.Contains(msg, "embedded_bin_dir") {
			t.Errorf("unexpected warning about embedded_bin_dir: %s", msg)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidationError formatting
// ---------------------------------------------------------------------------

func TestValidationError_Format(t *testing.T) {
	ve := &ValidationError{}
	ve.add("first error")
	ve.addf("second error: %d", 42)
	msg := ve.Error()
	if !strings.Contains(msg, "first error") {
		t.Error("missing first error in output")
	}
	if !strings.Contains(msg, "second error: 42") {
		t.Error("missing second error in output")
	}
	if !strings.Contains(msg, "configuration validation failed") {
		t.Error("missing header in output")
	}
}

func TestValidationError_HasErrors(t *testing.T) {
	ve := &ValidationError{}
	if ve.hasErrors() {
		t.Error("empty ValidationError should not have errors")
	}
	ve.add("an error")
	if !ve.hasErrors() {
		t.Error("ValidationError with entries should have errors")
	}
}

// ---------------------------------------------------------------------------
// Multiple errors collected
// ---------------------------------------------------------------------------

func TestValidation_MultipleErrors(t *testing.T) {
	yaml := `
organisations:
  - name: ""
    org_name: ""
    client_name: ""
`
	_, _, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected validation errors")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	// Should have multiple errors for missing fields
	if len(ve.Errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(ve.Errors), ve.Errors)
	}
}

// ---------------------------------------------------------------------------
// Full round-trip with all sections
// ---------------------------------------------------------------------------

func TestParse_FullConfig(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "client.pem")
	os.WriteFile(keyFile, []byte("fake"), 0600)
	exportDir := filepath.Join(dir, "exports")
	os.Mkdir(exportDir, 0755)

	t.Setenv("TEST_WEBHOOK_FULL", "https://hooks.example.com")

	yaml := `
credential_encryption_key_env: MY_KEY

organisations:
  - name: prod
    chef_server_url: https://chef.example.com
    org_name: prod
    client_name: metrics
    client_key_path: ` + keyFile + `
  - name: staging
    chef_server_url: https://chef.example.com
    org_name: staging
    client_name: metrics
    client_key_credential: staging-key

target_chef_versions:
  - "18.5.0"
  - "19.0.0"

git_base_urls:
  - https://github.com/myorg

collection:
  schedule: "*/30 * * * *"
  stale_node_threshold_days: 3
  stale_cookbook_threshold_days: 180

concurrency:
  organisation_collection: 2
  node_page_fetching: 5
  git_pull: 8
  cookstyle_scan: 4
  test_kitchen_run: 2
  readiness_evaluation: 10

analysis_tools:
  embedded_bin_dir: ` + dir + `
  cookstyle_timeout_minutes: 5
  test_kitchen_timeout_minutes: 15

readiness:
  min_free_disk_mb: 4096

notifications:
  enabled: true
  channels:
    - name: slack
      type: webhook
      url_env: TEST_WEBHOOK_FULL
      events:
        - cookbook_status_change
        - collection_failure

  readiness_milestones:
    - 25
    - 50
    - 75
    - 100
  stale_node_alert_count: 100

exports:
  max_rows: 50000
  async_threshold: 5000
  output_directory: ` + exportDir + `
  retention_hours: 48

datastore:
  url: postgres://db.example.com:5432/cmm

server:
  listen_address: "127.0.0.1"
  port: 443

frontend:
  base_path: "/metrics/"

logging:
  level: WARN
  retention_days: 30

auth:
  providers:
    - type: local
    - type: ldap
      host: ldap.example.com
      port: 636
      base_dn: "ou=users,dc=example,dc=com"
    - type: saml
      idp_metadata_url: https://idp.example.com/saml/metadata
      sp_entity_id: chef-migration-metrics
`
	cfg := mustParse(t, yaml)

	// Spot-check a selection of values
	if cfg.CredentialEncryptionKeyEnv != "MY_KEY" {
		t.Errorf("credential_encryption_key_env: %q", cfg.CredentialEncryptionKeyEnv)
	}
	if len(cfg.Organisations) != 2 {
		t.Errorf("expected 2 orgs, got %d", len(cfg.Organisations))
	}
	if len(cfg.TargetChefVersions) != 2 {
		t.Errorf("expected 2 target versions, got %d", len(cfg.TargetChefVersions))
	}
	if len(cfg.GitBaseURLs) != 1 {
		t.Errorf("expected 1 git_base_url, got %d", len(cfg.GitBaseURLs))
	}
	if cfg.Collection.Schedule != "*/30 * * * *" {
		t.Errorf("schedule: %q", cfg.Collection.Schedule)
	}
	if cfg.Concurrency.OrganisationCollection != 2 {
		t.Errorf("concurrency: %d", cfg.Concurrency.OrganisationCollection)
	}
	if cfg.AnalysisTools.CookstyleTimeoutMinutes != 5 {
		t.Errorf("cookstyle_timeout: %d", cfg.AnalysisTools.CookstyleTimeoutMinutes)
	}
	if cfg.Readiness.MinFreeDiskMB != 4096 {
		t.Errorf("min_free_disk_mb: %d", cfg.Readiness.MinFreeDiskMB)
	}
	if !cfg.Notifications.Enabled {
		t.Error("notifications should be enabled")
	}
	if len(cfg.Notifications.Channels) != 1 {
		t.Errorf("expected 1 channel, got %d", len(cfg.Notifications.Channels))
	}
	if cfg.Exports.MaxRows != 50000 {
		t.Errorf("max_rows: %d", cfg.Exports.MaxRows)
	}
	if cfg.Datastore.URL != "postgres://db.example.com:5432/cmm" {
		t.Errorf("datastore url: %q", cfg.Datastore.URL)
	}
	if cfg.Server.Port != 443 {
		t.Errorf("server port: %d", cfg.Server.Port)
	}
	if cfg.Frontend.BasePath != "/metrics/" {
		t.Errorf("base_path: %q", cfg.Frontend.BasePath)
	}
	if cfg.Logging.Level != "WARN" {
		t.Errorf("logging level: %q", cfg.Logging.Level)
	}
	if len(cfg.Auth.Providers) != 3 {
		t.Errorf("expected 3 auth providers, got %d", len(cfg.Auth.Providers))
	}
}

// ---------------------------------------------------------------------------
// checkDirWritable helper
// ---------------------------------------------------------------------------

func TestCheckDirWritable_Exists(t *testing.T) {
	dir := t.TempDir()
	if err := checkDirWritable(dir); err != nil {
		t.Errorf("expected no error for writable dir, got: %v", err)
	}
}

func TestCheckDirWritable_NotExist(t *testing.T) {
	err := checkDirWritable("/nonexistent/path/xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckDirWritable_NotADirectory(t *testing.T) {
	f, err := os.CreateTemp("", "config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	err = checkDirWritable(f.Name())
	if err == nil {
		t.Fatal("expected error for file (not dir)")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("unexpected error: %v", err)
	}
}
