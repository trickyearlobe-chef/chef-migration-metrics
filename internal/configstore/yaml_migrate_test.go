// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// MigrateFromYAML — happy path: full YAML migrated into empty config store
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_FullMigration(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	cfg := minimalFullConfig()
	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")

	writeYAML(t, yamlPath, fullYAMLContent())

	result, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("MigrateFromYAML: %v", err)
	}

	if result.Skipped {
		t.Fatal("expected migration to run, but it was skipped")
	}
	if result.SectionsMigrated == 0 {
		t.Fatal("expected sections to be migrated, got 0")
	}

	// Verify config store now has entries.
	empty, err := store.IsEmpty(ctx)
	if err != nil {
		t.Fatalf("IsEmpty: %v", err)
	}
	if empty {
		t.Fatal("config store should not be empty after migration")
	}

	// Verify original file was renamed.
	if _, err := os.Stat(yamlPath + ".migrated"); os.IsNotExist(err) {
		t.Fatal("expected config.yml.migrated to exist")
	}
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Fatal("expected bootstrap config.yml to exist")
	}

	// Verify bootstrap file contains only bootstrap keys.
	bootstrapData, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read bootstrap file: %v", err)
	}
	bootstrapStr := string(bootstrapData)
	if !strings.Contains(bootstrapStr, "database_url") {
		t.Error("bootstrap file should contain database_url")
	}
	if !strings.Contains(bootstrapStr, "listen_address") {
		t.Error("bootstrap file should contain listen_address")
	}
	if !strings.Contains(bootstrapStr, "listen_port") {
		t.Error("bootstrap file should contain listen_port")
	}
	// Should NOT contain non-bootstrap keys.
	if strings.Contains(bootstrapStr, "organisations") {
		t.Error("bootstrap file should not contain organisations")
	}
	if strings.Contains(bootstrapStr, "concurrency") {
		t.Error("bootstrap file should not contain concurrency")
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — skips when config store already has entries
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_SkipsWhenNotEmpty(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	// Pre-populate config store.
	if err := store.Set(ctx, "existing_key", json.RawMessage(`"value"`), false, "test"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	cfg := minimalFullConfig()
	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")
	writeYAML(t, yamlPath, fullYAMLContent())

	result, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("MigrateFromYAML: %v", err)
	}

	if !result.Skipped {
		t.Fatal("expected migration to be skipped when config store is not empty")
	}
	if result.SkipReason == "" {
		t.Fatal("expected skip reason to be set")
	}

	// Verify original file was NOT renamed.
	if _, err := os.Stat(yamlPath + ".migrated"); !os.IsNotExist(err) {
		t.Fatal("config.yml.migrated should not exist when migration was skipped")
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — idempotent: second call is a no-op
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	cfg := minimalFullConfig()
	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")
	writeYAML(t, yamlPath, fullYAMLContent())

	// First migration.
	result1, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("first MigrateFromYAML: %v", err)
	}
	if result1.Skipped {
		t.Fatal("first migration should not be skipped")
	}

	count1, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	// Second call — config store is now populated, so it should skip.
	// The original YAML was already renamed, so pass the bootstrap path.
	result2, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("second MigrateFromYAML: %v", err)
	}
	if !result2.Skipped {
		t.Fatal("second migration should be skipped")
	}

	count2, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count after second call: %v", err)
	}
	if count1 != count2 {
		t.Fatalf("entry count changed between calls: %d → %d", count1, count2)
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — sections can be round-tripped through assembly
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_RoundTripAssembly(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	cfg := minimalFullConfig()
	cfg.Concurrency.OrganisationCollection = 5
	cfg.Concurrency.NodePageFetching = 10
	cfg.Concurrency.CookstyleScan = 3
	cfg.Collection.StaleNodeThresholdDays = 30
	cfg.Logging.Level = "DEBUG"

	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")
	writeYAML(t, yamlPath, fullYAMLContent())

	_, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("MigrateFromYAML: %v", err)
	}

	// Reassemble config from the store.
	assembled, _, err := AssembleConfig(ctx, store)
	if err != nil {
		t.Fatalf("AssembleConfig: %v", err)
	}

	// Verify specific fields survived the round-trip.
	if assembled.Concurrency.OrganisationCollection != 5 {
		t.Errorf("OrganisationCollection: got %d, want 5", assembled.Concurrency.OrganisationCollection)
	}
	if assembled.Concurrency.NodePageFetching != 10 {
		t.Errorf("NodePageFetching: got %d, want 10", assembled.Concurrency.NodePageFetching)
	}
	if assembled.Concurrency.CookstyleScan != 3 {
		t.Errorf("CookstyleScan: got %d, want 3", assembled.Concurrency.CookstyleScan)
	}
	if assembled.Collection.StaleNodeThresholdDays != 30 {
		t.Errorf("StaleNodeThresholdDays: got %d, want 30", assembled.Collection.StaleNodeThresholdDays)
	}
	if assembled.Logging.Level != "DEBUG" {
		t.Errorf("Logging.Level: got %q, want %q", assembled.Logging.Level, "DEBUG")
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — YAML file does not exist (no-op, not an error)
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_FileDoesNotExist(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	cfg := minimalFullConfig()

	result, err := MigrateFromYAML(ctx, store, cfg, "/nonexistent/config.yml")
	if err != nil {
		t.Fatalf("MigrateFromYAML should not error on missing file: %v", err)
	}
	if !result.Skipped {
		t.Fatal("expected migration to be skipped when file does not exist")
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — backup file already exists (do not overwrite)
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_BackupAlreadyExists(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	cfg := minimalFullConfig()
	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")
	backupPath := yamlPath + ".migrated"

	writeYAML(t, yamlPath, fullYAMLContent())

	// Pre-create the backup file with known content.
	backupContent := []byte("# previous migration backup\n")
	if err := os.WriteFile(backupPath, backupContent, 0o600); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	result, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("MigrateFromYAML: %v", err)
	}

	if result.Skipped {
		t.Fatal("migration should still run even if backup exists")
	}

	// Verify the backup was NOT overwritten — the previous backup is preserved.
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != string(backupContent) {
		t.Error("existing backup file should not be overwritten")
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — bootstrap YAML content
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_BootstrapContent(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	cfg := minimalFullConfig()
	cfg.Datastore.URL = "postgres://user:pass@localhost:5432/cmm"
	cfg.Server.ListenAddress = "127.0.0.1"
	cfg.Server.Port = 9090

	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")
	writeYAML(t, yamlPath, fullYAMLContent())

	_, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("MigrateFromYAML: %v", err)
	}

	// Parse the bootstrap file to verify it contains correct values.
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}

	var bootstrap bootstrapConfigTest
	if err := parseBootstrapYAML(data, &bootstrap); err != nil {
		t.Fatalf("parse bootstrap: %v", err)
	}

	if bootstrap.DatabaseURL != "postgres://user:pass@localhost:5432/cmm" {
		t.Errorf("database_url: got %q, want %q", bootstrap.DatabaseURL, "postgres://user:pass@localhost:5432/cmm")
	}
	if bootstrap.ListenAddress != "127.0.0.1" {
		t.Errorf("listen_address: got %q, want %q", bootstrap.ListenAddress, "127.0.0.1")
	}
	if bootstrap.ListenPort != 9090 {
		t.Errorf("listen_port: got %d, want %d", bootstrap.ListenPort, 9090)
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — store error propagated
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_StoreErrorPropagated(t *testing.T) {
	ctx := context.Background()

	// Use an errorDB that fails on IsEmpty.
	errDB := &errorDB{err: errTestDB}
	store := mustNewStoreWithErrorDB(t, errDB)

	cfg := minimalFullConfig()
	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")
	writeYAML(t, yamlPath, fullYAMLContent())

	_, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err == nil {
		t.Fatal("expected error from store failure")
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — ConfigToSections keys stored correctly
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_AllSectionsStored(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	cfg := minimalFullConfig()
	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")
	writeYAML(t, yamlPath, fullYAMLContent())

	result, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("MigrateFromYAML: %v", err)
	}

	// Verify known section keys are in the store.
	expectedKeys := []string{
		KeyOrganisations,
		KeyCollection,
		KeyConcurrency,
		KeyLogging,
		KeyFrontend,
	}

	for _, key := range expectedKeys {
		val, getErr := store.Get(ctx, key)
		if getErr != nil {
			t.Errorf("Get(%q): %v", key, getErr)
			continue
		}
		if len(val) == 0 {
			t.Errorf("Get(%q): empty value", key)
		}
	}

	// SectionsMigrated should match the number of sections from ConfigToSections.
	sections, err := ConfigToSections(cfg)
	if err != nil {
		t.Fatalf("ConfigToSections: %v", err)
	}
	if result.SectionsMigrated != len(sections) {
		t.Errorf("SectionsMigrated: got %d, want %d", result.SectionsMigrated, len(sections))
	}
}

// ---------------------------------------------------------------------------
// IsFullYAML detection
// ---------------------------------------------------------------------------

func TestIsFullYAML(t *testing.T) {
	full := minimalFullConfig()
	full.Organisations = []config.Organisation{
		{Name: "test", ChefServerURL: "https://chef.example.com", OrgName: "testorg", ClientName: "client"},
	}

	if !IsFullYAML(full) {
		t.Error("config with organisations should be detected as full YAML")
	}

	bootstrap := &config.Config{}
	bootstrap.Datastore.URL = "postgres://localhost/cmm"
	bootstrap.Server.ListenAddress = "127.0.0.1"
	bootstrap.Server.Port = 8080

	if IsFullYAML(bootstrap) {
		t.Error("config without organisations should not be detected as full YAML")
	}
}

func TestIsFullYAML_EmptyOrganisations(t *testing.T) {
	cfg := &config.Config{}
	cfg.Organisations = []config.Organisation{}

	if IsFullYAML(cfg) {
		t.Error("config with empty organisations slice should not be detected as full YAML")
	}
}

// ---------------------------------------------------------------------------
// MigrateFromYAML — file permissions preserved on bootstrap
// ---------------------------------------------------------------------------

func TestMigrateFromYAML_BootstrapFilePermissions(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	cfg := minimalFullConfig()
	yamlDir := t.TempDir()
	yamlPath := filepath.Join(yamlDir, "config.yml")
	writeYAML(t, yamlPath, fullYAMLContent())

	// Set specific permissions on the original file.
	if err := os.Chmod(yamlPath, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	_, err := MigrateFromYAML(ctx, store, cfg, yamlPath)
	if err != nil {
		t.Fatalf("MigrateFromYAML: %v", err)
	}

	info, err := os.Stat(yamlPath)
	if err != nil {
		t.Fatalf("stat bootstrap file: %v", err)
	}

	// Bootstrap file should have restrictive permissions (0600).
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		t.Errorf("bootstrap file permissions too open: %o (want 0600)", perm)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// errTestDB is a sentinel error used by yaml_migrate test fakes.
var errTestDB = errForTest("yaml migrate db error")

type errForTest string

func (e errForTest) Error() string { return string(e) }

// minimalFullConfig returns a Config that passes validation and has
// organisations set — representing a "full" YAML config as opposed to a
// bootstrap-only config.
func minimalFullConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Organisations = []config.Organisation{
		{
			Name:                "test-org",
			ChefServerURL:       "https://chef.example.com",
			OrgName:             "testorg",
			ClientName:          "testclient",
			ClientKeyCredential: "test-org-key",
		},
	}
	cfg.Datastore.URL = "postgres://user:pass@localhost:5432/cmm"
	cfg.Server.ListenAddress = "127.0.0.1"
	cfg.Server.Port = 8080
	cfg.TargetChefVersions = []string{"18.0.0"}
	cfg.ApplyDefaults()
	return cfg
}

// fullYAMLContent returns a minimal but valid full YAML config string that
// contains the organisations key (making it detectable as a full config).
func fullYAMLContent() string {
	return `database_url: "postgres://user:pass@localhost:5432/cmm"
listen_address: "127.0.0.1"
listen_port: 8080
organisations:
  - name: test-org
    chef_server_url: "https://chef.example.com"
    org_name: testorg
    client_name: testclient
    client_key_credential: test-org-key
target_chef_versions:
  - "18.0.0"
collection:
  stale_node_threshold_days: 30
concurrency:
  organisation_collection: 2
logging:
  level: INFO
`
}

// writeYAML writes content to a file for test use.
func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write YAML: %v", err)
	}
}

// bootstrapConfigTest mirrors the bootstrap YAML structure for test parsing.
type bootstrapConfigTest struct {
	DatabaseURL   string `yaml:"database_url"`
	ListenAddress string `yaml:"listen_address"`
	ListenPort    int    `yaml:"listen_port"`
}

// parseBootstrapYAML is a test helper that parses bootstrap YAML content.
func parseBootstrapYAML(data []byte, out *bootstrapConfigTest) error {
	return yaml.Unmarshal(data, out)
}
