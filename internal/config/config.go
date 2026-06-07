// Package config provides configuration loading, defaults, and validation for
// chef-migration-metrics. Configuration is read from a YAML file whose path is
// supplied via the CHEF_MIGRATION_METRICS_CONFIG environment variable or passed
// directly to Load. Environment variable overrides are applied on top of the
// file values.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Top-level configuration
// ---------------------------------------------------------------------------

// Config is the root configuration structure for the application.
type Config struct {
	CredentialEncryptionKeyEnv string              `yaml:"credential_encryption_key_env"`
	Organisations              []Organisation      `yaml:"organisations"`
	TargetChefVersions         []string            `yaml:"target_chef_versions"`
	GitBaseURLs                []string            `yaml:"git_base_urls"`
	Storage                    StorageConfig       `yaml:"storage"`
	Collection                 CollectionConfig    `yaml:"collection"`
	Concurrency                ConcurrencyConfig   `yaml:"concurrency"`
	AnalysisTools              AnalysisToolsConfig `yaml:"analysis_tools"`
	Readiness                  ReadinessConfig     `yaml:"readiness"`
	Exports                    ExportsConfig       `yaml:"exports"`
	Elasticsearch              ElasticsearchConfig `yaml:"elasticsearch"`
	Datastore                  DatastoreConfig     `yaml:"datastore"`
	Server                     ServerConfig        `yaml:"server"`
	Frontend                   FrontendConfig      `yaml:"frontend"`
	Logging                    LoggingConfig       `yaml:"logging"`
	Auth                       AuthConfig          `yaml:"auth"`
	Ownership                  OwnershipConfig     `yaml:"ownership"`
	SystemHealth               SystemHealthConfig  `yaml:"system_health"`
	Performance                PerformanceConfig   `yaml:"performance"`
	Backup                     BackupConfig        `yaml:"backup"`

	// explicitExportsDir tracks whether the user explicitly set exports.output_directory.
	explicitExportsDir bool
	// explicitESDir tracks whether the user explicitly set elasticsearch.output_directory.
	explicitESDir bool
}

// ---------------------------------------------------------------------------
// Chef server organisations
// ---------------------------------------------------------------------------

// Organisation describes a single Chef Infra Server organisation to collect
// data from.
type Organisation struct {
	Name                string `yaml:"name"`
	ChefServerURL       string `yaml:"chef_server_url"`
	OrgName             string `yaml:"org_name"`
	ClientName          string `yaml:"client_name"`
	ClientKeyPath       string `yaml:"client_key_path"`
	ClientKeyCredential string `yaml:"client_key_credential"`
	SSLVerify           *bool  `yaml:"ssl_verify"`
}

// SSLVerifyEnabled returns whether SSL verification is enabled for this
// organisation. The default is true (verify) when not explicitly set.
func (o *Organisation) SSLVerifyEnabled() bool {
	if o.SSLVerify == nil {
		return true
	}
	return *o.SSLVerify
}

// ---------------------------------------------------------------------------
// Storage paths
// ---------------------------------------------------------------------------

// StorageConfig controls the filesystem paths used for persistent data such
// as downloaded Chef Server cookbooks and cloned git repositories. All paths
// default to subdirectories under DataDir.
//
// For RPM/DEB installs DataDir defaults to /var/lib/chef-migration-metrics
// which is created by the package with correct ownership. For development
// (when the default is not writable) it falls back to $TMPDIR/chef-migration-metrics.
type StorageConfig struct {
	// DataDir is the base directory for all persistent application data.
	// CookbookCacheDir and GitCookbookDir default to subdirectories of
	// this path when not explicitly set.
	DataDir string `yaml:"data_dir"`

	// CookbookCacheDir is the directory where Chef Server cookbook files
	// are extracted after download. Structure:
	//   <cookbook_cache_dir>/<org_id>/<name>/<version>/
	CookbookCacheDir string `yaml:"cookbook_cache_dir"`

	// GitCookbookDir is the directory where git cookbook repositories are
	// cloned and pulled. Structure:
	//   <git_cookbook_dir>/<cookbook_name>/
	GitCookbookDir string `yaml:"git_cookbook_dir"`
}

// ---------------------------------------------------------------------------
// Collection schedule & thresholds
// ---------------------------------------------------------------------------

// CollectionConfig controls the background collection schedule and staleness
// thresholds.
type CollectionConfig struct {
	Schedule                       string `yaml:"schedule"`
	StaleNodeThresholdDays         int    `yaml:"stale_node_threshold_days"`
	StaleNodeWarningHours          int    `yaml:"stale_node_warning_hours"`
	StaleNodeCriticalDays          int    `yaml:"stale_node_critical_days"`
	StaleCookbookThresholdDays     int    `yaml:"stale_cookbook_threshold_days"`
	SkipServerCookbookDownload     bool   `yaml:"skip_server_cookbook_download"`
	DeleteServerCookbooksAfterScan *bool  `yaml:"delete_server_cookbooks_after_scan"`
}

// DeleteServerCookbooksAfterScanEnabled reports whether server cookbook files
// should be deleted after scanning. Defaults to false when omitted.
func (c *CollectionConfig) DeleteServerCookbooksAfterScanEnabled() bool {
	if c.DeleteServerCookbooksAfterScan == nil {
		return false
	}
	return *c.DeleteServerCookbooksAfterScan
}

// ---------------------------------------------------------------------------
// Concurrency / worker pool sizes
// ---------------------------------------------------------------------------

// ConcurrencyConfig controls worker pool sizes for parallelised tasks.
type ConcurrencyConfig struct {
	OrganisationCollection int `yaml:"organisation_collection"`
	NodePageFetching       int `yaml:"node_page_fetching"`
	GitPull                int `yaml:"git_pull"`
	CookbookDownload       int `yaml:"cookbook_download"`
	CookstyleScan          int `yaml:"cookstyle_scan"`
	ReadinessEvaluation    int `yaml:"readiness_evaluation"`
}

// ---------------------------------------------------------------------------
// Analysis tools (embedded CookStyle / Test Kitchen)
// ---------------------------------------------------------------------------

// AnalysisToolsConfig controls the embedded analysis tool locations and
// timeouts.
type AnalysisToolsConfig struct {
	EmbeddedBinDir            string            `yaml:"embedded_bin_dir"`
	CookstyleEnabled          *bool             `yaml:"cookstyle_enabled"`
	CookstyleTimeoutMinutes   int               `yaml:"cookstyle_timeout_minutes"`
	TestKitchenTimeoutMinutes int               `yaml:"test_kitchen_timeout_minutes"`
	TestKitchen               TestKitchenConfig `yaml:"test_kitchen"`
}

// resolveTestKitchenBackwardCompat emits a deprecation warning if the
// legacy test_kitchen_timeout_minutes field was used. The actual value
// migration happens in setDefaults (before nested defaults are applied).
func (a *AnalysisToolsConfig) resolveTestKitchenBackwardCompat(w *Warnings) {
	if a.TestKitchenTimeoutMinutes > 0 {
		w.add("analysis_tools.test_kitchen_timeout_minutes is deprecated; use analysis_tools.test_kitchen.timeout_minutes instead")
	}
}

// IsCookstyleEnabled returns true if CookStyle scanning is enabled in the
// configuration. Defaults to true when the field is omitted.
func (a *AnalysisToolsConfig) IsCookstyleEnabled() bool {
	if a.CookstyleEnabled == nil {
		return true
	}
	return *a.CookstyleEnabled
}

// TestKitchenConfig controls the pluggable driver architecture for Test
// Kitchen runs. It replaces the hardcoded Docker/dokken assumption with
// opaque settings bags that work for any driver (vcenter, vra, ec2, etc.).
// See test-kitchen-drivers.md for the full specification.
type TestKitchenConfig struct {
	// Enabled controls whether Test Kitchen testing is active. When set to
	// false, Test Kitchen is disabled even if the kitchen binary is
	// available. When omitted or set to true (the default), Test Kitchen
	// is enabled automatically if the kitchen binary is detected at startup.
	Enabled *bool `yaml:"enabled" json:"enabled"`

	// TimeoutMinutes is the maximum wall-clock time for a single Test
	// Kitchen converge or verify step. Defaults to 30.
	TimeoutMinutes int `yaml:"timeout_minutes" json:"timeout_minutes"`

	// Driver selects the Test Kitchen driver profile. Built-in profiles:
	// vcenter, vra, ec2, vagrant, proxmox. Required.
	Driver string `yaml:"driver" json:"driver"`

	// DriverSettings contains plaintext driver connection settings as
	// key-value pairs. Keys are driver-specific (e.g. vcenter_host,
	// region). These are serialised into the top-level driver: block of
	// the generated .kitchen.local.yml overlay.
	DriverSettings map[string]any `yaml:"driver_settings" json:"driver_settings"`

	// DriverSecrets maps driver setting names to credential names from
	// the credentials table. At runtime each secret is resolved via the
	// credential resolver and injected as CMM_TK_SECRET_<UPPER_KEY>
	// environment variables. The overlay references them via ERB.
	DriverSecrets map[string]string `yaml:"driver_secrets" json:"driver_secrets"`

	// ImageFieldName is the YAML key used for the image identifier in
	// platform map entries. Built-in profiles set this automatically
	// (e.g. "template" for vcenter, "ami" for ec2). Required only for
	// the "custom" profile.
	ImageFieldName string `yaml:"image_field_name" json:"image_field_name"`

	// PlatformMap is the alias table mapping cookbook platform names to
	// image registry entries. Each entry maps a kitchen_name to an image
	// name defined in the Images list.
	// For dokken with an empty platform map, all platforms pass through
	// unchanged (backward compatible).
	PlatformMap []PlatformMapEntry `yaml:"platform_map" json:"platform_map"`

	// Images is the registry of infrastructure images available to the
	// driver. Each entry defines the driver-specific image identifier,
	// transport credentials, per-image driver settings, and per-version
	// Chef package download URLs. Multiple platform map entries can
	// reference the same image.
	Images []ImageEntry `yaml:"images" json:"images"`

	// ChefLicenseKeyCredential is the credential name for the Chef
	// license key. Used as fallback for versions that have no
	// chef_download_urls entry in any image (public chef.io download).
	ChefLicenseKeyCredential string `yaml:"chef_license_key_credential" json:"chef_license_key_credential"`

	// HypervisorType selects the hypervisor backend for template discovery
	// and VM lifecycle management. Valid values: "vcenter", "proxmox", or ""
	// (auto-detect from driver name). When empty, the driver name is used
	// to infer the hypervisor type.
	HypervisorType string `yaml:"hypervisor_type" json:"hypervisor_type"`

	// VMTTLHours is the maximum VM lifetime in hours before flagging as
	// orphaned. Defaults to 4.
	VMTTLHours int `yaml:"vm_ttl_hours" json:"vm_ttl_hours"`

	// VMNamePrefix is the prefix used in the VM naming convention.
	// Defaults to "cmm".
	VMNamePrefix string `yaml:"vm_name_prefix" json:"vm_name_prefix"`

	// MaxConcurrentVMs is the global ceiling on concurrent VMs (the
	// kitchen queue worker-pool size). Concurrency is global only — there
	// is no per-batch concurrency. Defaults to DefaultMaxConcurrentVMs
	// when unset (<= 0). Changes apply dynamically (no restart).
	MaxConcurrentVMs int `yaml:"max_concurrent_vms" json:"max_concurrent_vms"`

	// OrphanSweepIntervalMinutes controls how often the background
	// hypervisor-side orphan sweep runs. 0 = default (30 min), -1 =
	// disabled. Minimum 5 minutes when positive.
	OrphanSweepIntervalMinutes int `yaml:"orphan_sweep_interval_minutes" json:"orphan_sweep_interval_minutes"`

	// OrphanSweepAgeMinutes is the minimum VM age before a VM is
	// eligible for sweep destruction. 0 = default (2× timeout).
	OrphanSweepAgeMinutes int `yaml:"orphan_sweep_age_minutes" json:"orphan_sweep_age_minutes"`

	// StartRateWindowMinutes is the global VM start-rate limiter window, set to
	// the DHCP lease time (e.g. 60, 90). With StartRateMaxPerWindow it bounds
	// cumulative lease consumption: no more than max starts occur in any
	// trailing window, charged for the full window regardless of early
	// teardown. 0 (with either value unset) disables the limiter. Dynamic.
	StartRateWindowMinutes int `yaml:"start_rate_window_minutes" json:"start_rate_window_minutes"`

	// StartRateMaxPerWindow is the maximum VM starts allowed per window, set to
	// the usable DHCP pool size (e.g. 25, 64). See StartRateWindowMinutes. 0
	// disables the limiter. Dynamic.
	StartRateMaxPerWindow int `yaml:"start_rate_max_per_window" json:"start_rate_max_per_window"`

	// SetupScripts opts cookbooks in to running repo-provided setup scripts
	// (e.g. user creation) on the guest before converge. Patterns are glob
	// patterns matched against repo file paths, scoped per OS family. Each
	// matched script body is inlined into a remote: pre_converge lifecycle
	// hook at overlay-generation time. These hooks MUST fail the run on a
	// non-zero exit — the cookbook depends on them. Dynamic. See
	// test-kitchen-drivers-overlay-generation.md § Customer setup scripts.
	SetupScripts *SetupScriptsConfig `yaml:"setup_scripts,omitempty" json:"setup_scripts,omitempty"`
}

// SetupScriptsConfig holds per-OS-family glob patterns for repo-provided
// setup scripts inlined into pre_converge. Per-image override is deferred.
type SetupScriptsConfig struct {
	// Linux globs match shell scripts run over SSH on linux platforms.
	Linux []string `yaml:"linux,omitempty" json:"linux,omitempty"`
	// Windows globs match scripts run over WinRM on windows platforms.
	Windows []string `yaml:"windows,omitempty" json:"windows,omitempty"`
}

// ImageEntry defines a single infrastructure image in the image registry.
type ImageEntry struct {
	// Name is the operator-defined label for this image. Must be unique
	// within the config. Used as the reference in platform_map[].image.
	Name string `yaml:"name" json:"name"`

	// ID is the driver-specific image identifier: a Proxmox template ID,
	// vCenter template name, AMI ID, Docker image, etc. The built-in
	// driver profile determines which YAML key it maps to in the overlay.
	ID string `yaml:"id" json:"id"`

	// DriverSettings contains per-image driver setting overrides merged
	// on top of the top-level DriverSettings in the overlay.
	DriverSettings map[string]any `yaml:"driver_settings" json:"driver_settings"`

	// Transport contains transport credentials for connecting to
	// instances provisioned from this image (SSH/WinRM).
	Transport *PlatformMapTransport `yaml:"transport" json:"transport"`

	// ChefDownloadURLs maps Chef version strings to direct package
	// download URLs for this image. When a URL is set for the target
	// version, the overlay uses download_url instead of product_version.
	// Platforms without an entry fall back to ChefLicenseKeyCredential.
	ChefDownloadURLs map[string]string `yaml:"chef_download_urls" json:"chef_download_urls"`

	// InstallMethod controls how Chef is installed on instances using this image.
	// "download" (default): use product_version or download_url to install Chef.
	// "baked_in": Chef is pre-installed in the image; emit require_chef_omnibus: false.
	InstallMethod string `yaml:"install_method,omitempty" json:"install_method,omitempty"`

	// ChefClientPath is the path to the chef-client binary when InstallMethod
	// is "baked_in" (e.g. "/usr/bin/chef-client", "/opt/chef/bin/chef-client").
	// Ignored when InstallMethod is "download".
	ChefClientPath string `yaml:"chef_client_path,omitempty" json:"chef_client_path,omitempty"`

	// ReleaseIPOnDestroy opts this image in to the best-effort IP-release
	// pre_destroy lifecycle hook (default off). When true, the generated
	// overlay injects a failure-isolated, transport-detached DHCP release
	// command (OS family derived from the platform name) so the lease is
	// returned promptly on teardown. A spike — enable only on images where
	// it is empirically confirmed to release the lease without abending the
	// run. Dynamic (read live from config, no restart). See
	// test-kitchen-drivers-overlay-generation.md § App-injected IP-release hook.
	ReleaseIPOnDestroy bool `yaml:"release_ip_on_destroy,omitempty" json:"release_ip_on_destroy,omitempty"`
}

// EffectiveInstallMethod returns the install method for the image,
// defaulting to "download" when unset.
func (e ImageEntry) EffectiveInstallMethod() string {
	if e.InstallMethod == "" {
		return "download"
	}
	return e.InstallMethod
}

// PlatformMapEntry maps a single kitchen platform name (or glob pattern) to
// an image in the image registry. When IsPattern is true, KitchenName is
// treated as a glob pattern. When Skip is true, the platform is excluded
// from TK runs.
type PlatformMapEntry struct {
	// KitchenName is the platform name as it appears in the cookbook's
	// .kitchen.yml (e.g. "ubuntu-22.04", "centos-7", "windows-2022").
	KitchenName string `yaml:"kitchen_name" json:"kitchen_name"`

	// Image is the name of an entry in the Images list.
	Image string `yaml:"image" json:"image"`

	// IsPattern indicates that KitchenName is a glob pattern (supports * and ?
	// wildcards) rather than an exact platform name. Pattern entries are
	// evaluated in order — first match wins — but exact matches always take
	// priority regardless of position.
	IsPattern bool `yaml:"is_pattern,omitempty" json:"is_pattern,omitempty"`

	// Skip marks this platform as explicitly excluded from Test Kitchen runs.
	// When true, the Image field is ignored. Skipped platforms count as
	// "handled" for validation purposes (not flagged as unmapped).
	Skip bool `yaml:"skip,omitempty" json:"skip,omitempty"`

	// Transport provides per-mapping transport credential overrides. When set,
	// these override the transport defined on the referenced ImageEntry.
	Transport *PlatformMapTransport `yaml:"transport,omitempty" json:"transport,omitempty"`
}

// PlatformMapTransport holds transport credentials for an image registry
// entry. Credentials are resolved at runtime from the credentials table.
type PlatformMapTransport struct {
	// Username is the SSH/WinRM username for connecting to the instance.
	Username string `yaml:"username" json:"username"`

	// PasswordCredential is the credential name for the SSH/WinRM password.
	// Resolved at runtime and injected as CMM_TK_TRANSPORT_<UPPER_IMAGE_NAME>.
	PasswordCredential string `yaml:"password_credential" json:"password_credential"`

	// SSHKeyCredential is the credential name for the SSH private key (PEM).
	// Resolved at runtime and injected as CMM_TK_KEY_<UPPER_IMAGE_NAME>.
	SSHKeyCredential string `yaml:"ssh_key_credential" json:"ssh_key_credential"`
}

// IsEnabled returns true if Test Kitchen testing is enabled in the
// configuration. Defaults to true when the field is omitted.
func (tk *TestKitchenConfig) IsEnabled() bool {
	if tk.Enabled == nil {
		return true
	}
	return *tk.Enabled
}

// EffectiveDriver returns the configured driver name.
func (tk *TestKitchenConfig) EffectiveDriver() string {
	return tk.Driver
}

// EffectiveTimeoutMinutes returns the configured timeout, defaulting to 30.
func (tk *TestKitchenConfig) EffectiveTimeoutMinutes() int {
	if tk.TimeoutMinutes <= 0 {
		return 30
	}
	return tk.TimeoutMinutes
}

// EffectiveVMTTLHours returns the configured VM TTL or the default of 4 hours.
func (c TestKitchenConfig) EffectiveVMTTLHours() int {
	if c.VMTTLHours > 0 {
		return c.VMTTLHours
	}
	return 4
}

// EffectiveVMNamePrefix returns the configured VM name prefix or "cmm".
func (c TestKitchenConfig) EffectiveVMNamePrefix() string {
	if c.VMNamePrefix != "" {
		return c.VMNamePrefix
	}
	return "cmm"
}

// DefaultMaxConcurrentVMs is the conservative global ceiling on concurrent
// kitchen VMs used when none is configured. Deliberately low because the
// target environment is DHCP-pool constrained; raise it via config (applies
// dynamically). The VM start-rate limiter, not this value, is the guarantee
// against DHCP pool exhaustion.
const DefaultMaxConcurrentVMs = 2

// EffectiveMaxConcurrentVMs returns the configured max concurrent VMs, or
// DefaultMaxConcurrentVMs when unset.
func (c TestKitchenConfig) EffectiveMaxConcurrentVMs() int {
	if c.MaxConcurrentVMs > 0 {
		return c.MaxConcurrentVMs
	}
	return DefaultMaxConcurrentVMs
}

// StartRateLimit returns the global VM start-rate limiter parameters. enabled
// is false unless both window and max are positive — a partial config cannot
// bound anything, so the limiter stays off rather than guessing a value.
func (c TestKitchenConfig) StartRateLimit() (window time.Duration, maxPerWindow int, enabled bool) {
	if c.StartRateWindowMinutes <= 0 || c.StartRateMaxPerWindow <= 0 {
		return 0, 0, false
	}
	return time.Duration(c.StartRateWindowMinutes) * time.Minute, c.StartRateMaxPerWindow, true
}

// EffectiveHypervisorType returns the hypervisor type — either explicitly
// configured or inferred from the driver name.
func (c TestKitchenConfig) EffectiveHypervisorType() string {
	if c.HypervisorType != "" {
		return c.HypervisorType
	}
	// Auto-detect from driver name.
	switch c.Driver {
	case "vcenter":
		return "vcenter"
	case "proxmox":
		return "proxmox"
	default:
		return ""
	}
}

// EffectiveOrphanSweepInterval returns the sweep interval duration.
// Returns 30 min if 0, 0 (disabled) if -1, minimum 5 min when positive.
func (tk *TestKitchenConfig) EffectiveOrphanSweepInterval() time.Duration {
	switch {
	case tk.OrphanSweepIntervalMinutes < 0:
		return 0
	case tk.OrphanSweepIntervalMinutes == 0:
		return 30 * time.Minute
	case tk.OrphanSweepIntervalMinutes < 5:
		return 5 * time.Minute
	default:
		return time.Duration(tk.OrphanSweepIntervalMinutes) * time.Minute
	}
}

// EffectiveOrphanSweepAge returns the minimum VM age before sweep
// destruction. Returns 2× EffectiveTimeoutMinutes if 0.
func (tk *TestKitchenConfig) EffectiveOrphanSweepAge() time.Duration {
	if tk.OrphanSweepAgeMinutes > 0 {
		return time.Duration(tk.OrphanSweepAgeMinutes) * time.Minute
	}
	return time.Duration(2*tk.EffectiveTimeoutMinutes()) * time.Minute
}

// ---------------------------------------------------------------------------
// Upgrade readiness
// ---------------------------------------------------------------------------

// ReadinessConfig controls the upgrade readiness evaluation parameters.
type ReadinessConfig struct {
	// Deprecated: use InstallSizeMBLinux/InstallSizeMBWindows instead.
	MinFreeDiskMB int `yaml:"min_free_disk_mb,omitempty"`

	InstallPathLinux        string `yaml:"install_path_linux"`
	InstallPathWindows      string `yaml:"install_path_windows"`
	InstallSizeMBLinux      int    `yaml:"install_size_mb_linux"`
	InstallSizeMBWindows    int    `yaml:"install_size_mb_windows"`
	MinRemainingFreePercent int    `yaml:"min_remaining_free_percent"`
}

// SystemHealthConfig controls host-level resource monitoring and the
// collection circuit breaker.
type SystemHealthConfig struct {
	DiskPaths                 []string `yaml:"disk_paths"`
	DiskUsedWarningPercent    float64  `yaml:"disk_used_warning_percent"`
	DiskUsedCriticalPercent   float64  `yaml:"disk_used_critical_percent"`
	CPULoadWarningPerCPU      float64  `yaml:"cpu_load_warning_per_cpu"`
	CPULoadCriticalPerCPU     float64  `yaml:"cpu_load_critical_per_cpu"`
	MemUsedWarningPercent     float64  `yaml:"mem_used_warning_percent"`
	MemUsedCriticalPercent    float64  `yaml:"mem_used_critical_percent"`
	PauseCollectionOnCritical *bool    `yaml:"pause_collection_on_critical"`
}

// IsPauseCollectionOnCritical returns whether collection should be paused
// when a critical system health alert is detected. Defaults to true.
func (sh SystemHealthConfig) IsPauseCollectionOnCritical() bool {
	if sh.PauseCollectionOnCritical == nil {
		return true
	}
	return *sh.PauseCollectionOnCritical
}

// ---------------------------------------------------------------------------
// Performance
// ---------------------------------------------------------------------------

// PerformanceConfig controls the in-app performance diagnostics.
type PerformanceConfig struct {
	Enabled       *bool `yaml:"enabled"`
	PprofEnabled  bool  `yaml:"pprof_enabled"`
	WindowSeconds int   `yaml:"window_seconds"`
}

// IsEnabled returns true if performance instrumentation is enabled.
// Defaults to true when the field is omitted from configuration.
func (pc PerformanceConfig) IsEnabled() bool {
	if pc.Enabled == nil {
		return true
	}
	return *pc.Enabled
}

// ---------------------------------------------------------------------------
// Backup
// ---------------------------------------------------------------------------

// BackupConfig controls database backup and restore behaviour.
type BackupConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Dir            string `yaml:"dir"`
	MaxGenerations int    `yaml:"max_generations"`
	Schedule       string `yaml:"schedule"`
	PgDumpPath     string `yaml:"pg_dump_path"`
	PgRestorePath  string `yaml:"pg_restore_path"`
}

// BackupDir returns the configured backup directory, falling back to
// <data_dir>/backups when not explicitly set.
func (c *Config) BackupDir() string {
	if c.Backup.Dir != "" {
		return c.Backup.Dir
	}
	return filepath.Join(c.Storage.DataDir, "backups")
}

// BackupMaxGenerations returns the number of backups to retain, defaulting to 7.
func (c *Config) BackupMaxGenerations() int {
	if c.Backup.MaxGenerations > 0 {
		return c.Backup.MaxGenerations
	}
	return 7
}

// BackupSchedule returns the cron expression for scheduled backups,
// defaulting to "0 2 * * *" (daily at 02:00).
func (c *Config) BackupSchedule() string {
	if c.Backup.Schedule != "" {
		return c.Backup.Schedule
	}
	return "0 2 * * *"
}

// ---------------------------------------------------------------------------
// Data exports
// ---------------------------------------------------------------------------

// ExportsConfig controls the behaviour of data export operations.
type ExportsConfig struct {
	MaxRows         int    `yaml:"max_rows"`
	AsyncThreshold  int    `yaml:"async_threshold"`
	OutputDirectory string `yaml:"output_directory"`
	RetentionHours  int    `yaml:"retention_hours"`
}

// ---------------------------------------------------------------------------
// Elasticsearch NDJSON export
// ---------------------------------------------------------------------------

// ElasticsearchConfig controls the NDJSON export pipeline for Elasticsearch.
type ElasticsearchConfig struct {
	Enabled         bool   `yaml:"enabled"`
	OutputDirectory string `yaml:"output_directory"`
	RetentionHours  int    `yaml:"retention_hours"`
}

// ---------------------------------------------------------------------------
// Datastore
// ---------------------------------------------------------------------------

// DatastoreConfig holds the database connection settings and pool tuning.
type DatastoreConfig struct {
	URL                    string `yaml:"url"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes"`
	ConnMaxIdleTimeMinutes int    `yaml:"conn_max_idle_time_minutes"`
}

// ---------------------------------------------------------------------------
// Web server & TLS
// ---------------------------------------------------------------------------

// ServerConfig controls the HTTP/HTTPS listener.
type ServerConfig struct {
	ListenAddress           string          `yaml:"listen_address"`
	Port                    int             `yaml:"port"`
	TLS                     TLSConfig       `yaml:"tls"`
	WebSocket               WebSocketConfig `yaml:"websocket"`
	GracefulShutdownSeconds int             `yaml:"graceful_shutdown_seconds"`
	// TrustedProxy controls whether X-Forwarded-Proto is trusted when
	// determining whether a request arrived over TLS. Set to true only
	// when the application is deployed behind a trusted reverse proxy.
	// Default false (safe).
	TrustedProxy bool `yaml:"trusted_proxy"`
}

// WebSocketConfig controls the real-time event WebSocket endpoint.
type WebSocketConfig struct {
	Enabled             *bool `yaml:"enabled"` // nil means "use default (true)"
	MaxConnections      int   `yaml:"max_connections"`
	SendBufferSize      int   `yaml:"send_buffer_size"`
	WriteTimeoutSeconds int   `yaml:"write_timeout_seconds"`
	PingIntervalSeconds int   `yaml:"ping_interval_seconds"`
	PongTimeoutSeconds  int   `yaml:"pong_timeout_seconds"`
}

// IsEnabled returns whether the WebSocket endpoint is enabled. If the
// Enabled field was not set in configuration, the default is true.
func (ws *WebSocketConfig) IsEnabled() bool {
	if ws.Enabled == nil {
		return true
	}
	return *ws.Enabled
}

// TLSConfig holds all TLS-related settings including static certificate and
// ACME modes.
type TLSConfig struct {
	Mode             string     `yaml:"mode"`
	Enabled          *bool      `yaml:"enabled,omitempty"` // deprecated, backward compat
	CertPath         string     `yaml:"cert_path"`
	KeyPath          string     `yaml:"key_path"`
	CAPath           string     `yaml:"ca_path"`
	MinVersion       string     `yaml:"min_version"`
	HTTPRedirectPort int        `yaml:"http_redirect_port"`
	ACME             ACMEConfig `yaml:"acme"`
}

// ACMEConfig holds Automatic Certificate Management Environment settings.
type ACMEConfig struct {
	Domains           []string          `yaml:"domains"`
	Email             string            `yaml:"email"`
	CAURL             string            `yaml:"ca_url"`
	Challenge         string            `yaml:"challenge"`
	DNSProvider       string            `yaml:"dns_provider"`
	DNSProviderConfig map[string]string `yaml:"dns_provider_config"`
	StoragePath       string            `yaml:"storage_path"`
	RenewBeforeDays   int               `yaml:"renew_before_days"`
	AgreeToTOS        bool              `yaml:"agree_to_tos"`
	TrustedRoots      string            `yaml:"trusted_roots"`
}

// ---------------------------------------------------------------------------
// Frontend
// ---------------------------------------------------------------------------

// FrontendConfig controls the embedded SPA serving behaviour.
type FrontendConfig struct {
	BasePath string `yaml:"base_path"`
}

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

// LoggingConfig controls structured logging and log retention.
type LoggingConfig struct {
	Level         string `yaml:"level"`
	RetentionDays int    `yaml:"retention_days"`
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// AuthConfig holds authentication provider configuration.
type AuthConfig struct {
	Providers         []AuthProvider `yaml:"providers"`
	SessionExpiry     string         `yaml:"session_expiry"`
	MinPasswordLength int            `yaml:"min_password_length"`
	LockoutAttempts   int            `yaml:"lockout_attempts"`
}

// AuthProvider is a single authentication provider (local or SAML).
type AuthProvider struct {
	Type           string `yaml:"type"`
	IDPMetadataURL string `yaml:"idp_metadata_url,omitempty"`
	IDPMetadataPath        string `yaml:"idp_metadata_path,omitempty"`
	SPEntityID             string `yaml:"sp_entity_id,omitempty"`

	// SAML SP signing credentials (stored in encrypted credential store).
	SPCertificateCredential string `yaml:"sp_certificate_credential,omitempty"`
	SPPrivateKeyCredential  string `yaml:"sp_private_key_credential,omitempty"`

	// SAML attribute mappings for extracting identity from assertions.
	UsernameAttr    string `yaml:"username_attr,omitempty"`
	EmailAttr       string `yaml:"email_attr,omitempty"`
	DisplayNameAttr string `yaml:"display_name_attr,omitempty"`
	GroupsAttr      string `yaml:"groups_attr,omitempty"`
	RoleAttr        string `yaml:"role_attr,omitempty"`

	// SAML role mapping: IdP group name → application role.
	RoleMapping map[string]string `yaml:"role_mapping,omitempty"`

	// SAML behaviour options.
	AllowIDPInitiated        bool   `yaml:"allow_idp_initiated,omitempty"`
	SignRequests             bool   `yaml:"sign_requests,omitempty"`
	ClockSkewTolerance       string `yaml:"clock_skew_tolerance,omitempty"`
	MetadataRefreshInterval  string `yaml:"metadata_refresh_interval,omitempty"`
}

// ---------------------------------------------------------------------------
// Ownership
// ---------------------------------------------------------------------------

// OwnershipConfig controls the ownership tracking feature.
type OwnershipConfig struct {
	AuditLog  OwnershipAuditLog   `yaml:"audit_log"`
	AutoRules []OwnershipAutoRule `yaml:"auto_rules"`
}

// OwnershipAuditLog controls retention of the ownership audit log.
type OwnershipAuditLog struct {
	RetentionDays int `yaml:"retention_days"`
}

// OwnershipAutoRule defines a single auto-derivation rule for ownership.
type OwnershipAutoRule struct {
	Name           string `yaml:"name"`
	Owner          string `yaml:"owner"`
	Type           string `yaml:"type"`
	AttributePath  string `yaml:"attribute_path"`
	MatchValue     string `yaml:"match_value"`
	Pattern        string `yaml:"pattern"`
	PolicyName     string `yaml:"policy_name"`
	Organisation   string `yaml:"organisation"`
	ObjectType     string `yaml:"object_type"`     // cmdb_attribute: one of node, cookbook, profile, role
	OwnerAttribute string `yaml:"owner_attribute"` // cmdb_attribute: field within itil.cmdb.<object_type> holding the owner name (default: "owner")
}

// ValidCMDBObjectTypes lists the allowed values for OwnershipAutoRule.ObjectType
// when Type is "cmdb_attribute". Each corresponds to a Chef normal attribute
// subtree at itil.cmdb.<object_type>.
var ValidCMDBObjectTypes = map[string]bool{
	"node":     true,
	"cookbook": true,
	"profile":  true,
	"role":     true,
}

// CMDBSearchKeys returns the additional partial-search attribute keys and
// paths needed to collect CMDB ownership data from Chef nodes. Each
// configured cmdb_attribute rule contributes a key of the form
// "itil.cmdb.<object_type>" mapped to the Chef attribute path
// ["itil", "cmdb", "<object_type>"]. Duplicate object types are
// deduplicated so each path is requested at most once.
//
// The returned map is intended to be merged into the standard
// NodeSearchAttributes query before executing partial search.
func (c *OwnershipConfig) CMDBSearchKeys() map[string][]string {
	keys := make(map[string][]string)
	for _, rule := range c.AutoRules {
		if rule.Type != "cmdb_attribute" || rule.ObjectType == "" {
			continue
		}
		key := "itil.cmdb." + rule.ObjectType
		if _, exists := keys[key]; !exists {
			keys[key] = []string{"itil", "cmdb", rule.ObjectType}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

// setDefaults populates zero-value fields with the specification defaults.
// ApplyDefaults fills in zero-valued fields with sensible defaults. This is
// exported for use by the configstore assembly path which builds a Config
// from database entries rather than YAML. The YAML Parse() path calls this
// internally via setDefaults().
func (c *Config) ApplyDefaults() {
	c.setDefaults()
}

func (c *Config) setDefaults() {
	if c.CredentialEncryptionKeyEnv == "" {
		c.CredentialEncryptionKeyEnv = "CMM_CREDENTIAL_ENCRYPTION_KEY"
	}

	// Storage — resolve DataDir first, then derive subdirectories from it.
	if c.Storage.DataDir == "" {
		// Prefer the standard package install location; fall back to a
		// temp-based path for development where /var/lib may not be writable.
		candidate := "/var/lib/chef-migration-metrics"
		if info, err := os.Stat(candidate); err != nil || !info.IsDir() {
			candidate = filepath.Join(os.TempDir(), "chef-migration-metrics")
		}
		c.Storage.DataDir = candidate
	}
	if c.Storage.CookbookCacheDir == "" {
		c.Storage.CookbookCacheDir = filepath.Join(c.Storage.DataDir, "cookbook-cache")
	}
	if c.Storage.GitCookbookDir == "" {
		c.Storage.GitCookbookDir = filepath.Join(c.Storage.DataDir, "git-cookbooks")
	}

	// Collection
	if c.Collection.Schedule == "" {
		c.Collection.Schedule = "0 * * * *"
	}
	if c.Collection.StaleNodeThresholdDays == 0 {
		c.Collection.StaleNodeThresholdDays = 7
	}
	if c.Collection.StaleNodeWarningHours == 0 {
		c.Collection.StaleNodeWarningHours = 72
	}
	if c.Collection.StaleNodeCriticalDays == 0 {
		// Backward compat: if old threshold is set, use it for critical.
		if c.Collection.StaleNodeThresholdDays > 0 {
			c.Collection.StaleNodeCriticalDays = c.Collection.StaleNodeThresholdDays
		} else {
			c.Collection.StaleNodeCriticalDays = 7
		}
	}
	if c.Collection.StaleCookbookThresholdDays == 0 {
		c.Collection.StaleCookbookThresholdDays = 365
	}
	if c.Collection.DeleteServerCookbooksAfterScan == nil {
		f := false
		c.Collection.DeleteServerCookbooksAfterScan = &f
	}

	// Concurrency
	if c.Concurrency.OrganisationCollection == 0 {
		c.Concurrency.OrganisationCollection = 5
	}
	if c.Concurrency.NodePageFetching == 0 {
		c.Concurrency.NodePageFetching = 10
	}
	if c.Concurrency.GitPull == 0 {
		c.Concurrency.GitPull = 10
	}
	if c.Concurrency.CookbookDownload == 0 {
		c.Concurrency.CookbookDownload = 4
	}
	if c.Concurrency.CookstyleScan == 0 {
		c.Concurrency.CookstyleScan = 8
	}
	if c.Concurrency.ReadinessEvaluation == 0 {
		c.Concurrency.ReadinessEvaluation = 20
	}

	// Analysis tools
	if c.AnalysisTools.EmbeddedBinDir == "" {
		c.AnalysisTools.EmbeddedBinDir = "/opt/chef-migration-metrics/embedded/bin"
	}
	if c.AnalysisTools.CookstyleTimeoutMinutes == 0 {
		c.AnalysisTools.CookstyleTimeoutMinutes = 10
	}
	// Backward compat: migrate deprecated test_kitchen_timeout_minutes
	// into the nested field BEFORE applying defaults, so the legacy value
	// is preserved when the nested field was not explicitly set.
	if c.AnalysisTools.TestKitchenTimeoutMinutes > 0 && c.AnalysisTools.TestKitchen.TimeoutMinutes == 0 {
		c.AnalysisTools.TestKitchen.TimeoutMinutes = c.AnalysisTools.TestKitchenTimeoutMinutes
	}

	// Test Kitchen nested defaults.
	if c.AnalysisTools.TestKitchen.TimeoutMinutes == 0 {
		c.AnalysisTools.TestKitchen.TimeoutMinutes = 30
	}
	if c.AnalysisTools.CookstyleEnabled == nil {
		t := true
		c.AnalysisTools.CookstyleEnabled = &t
	}
	if c.AnalysisTools.TestKitchen.Enabled == nil {
		t := true
		c.AnalysisTools.TestKitchen.Enabled = &t
	}

	// Readiness
	if c.Readiness.InstallPathLinux == "" {
		c.Readiness.InstallPathLinux = "/hab"
	}
	if c.Readiness.InstallPathWindows == "" {
		c.Readiness.InstallPathWindows = `C:\hab`
	}
	if c.Readiness.InstallSizeMBLinux == 0 {
		c.Readiness.InstallSizeMBLinux = 3072
	}
	if c.Readiness.InstallSizeMBWindows == 0 {
		c.Readiness.InstallSizeMBWindows = 6144
	}
	if c.Readiness.MinRemainingFreePercent == 0 {
		c.Readiness.MinRemainingFreePercent = 20
	}
	// Backward compat: if old min_free_disk_mb is set but new fields are
	// at defaults, honour it for both platforms.
	if c.Readiness.MinFreeDiskMB != 0 {
		if c.Readiness.InstallSizeMBLinux == 3072 {
			c.Readiness.InstallSizeMBLinux = c.Readiness.MinFreeDiskMB
		}
		if c.Readiness.InstallSizeMBWindows == 6144 {
			c.Readiness.InstallSizeMBWindows = c.Readiness.MinFreeDiskMB
		}
	}

	// Exports
	if c.Exports.MaxRows == 0 {
		c.Exports.MaxRows = 100000
	}
	if c.Exports.AsyncThreshold == 0 {
		c.Exports.AsyncThreshold = 10000
	}
	if c.Exports.OutputDirectory == "" {
		c.Exports.OutputDirectory = "/var/lib/chef-migration-metrics/exports"
	}
	if c.Exports.RetentionHours == 0 {
		c.Exports.RetentionHours = 24
	}

	// Elasticsearch
	if c.Elasticsearch.OutputDirectory == "" {
		c.Elasticsearch.OutputDirectory = "/var/lib/chef-migration-metrics/elasticsearch"
	}
	if c.Elasticsearch.RetentionHours == 0 {
		c.Elasticsearch.RetentionHours = 48
	}

	// Datastore
	if c.Datastore.URL == "" {
		c.Datastore.URL = "postgres://localhost:5432/chef_migration_metrics"
	}
	if c.Datastore.MaxOpenConns == 0 {
		c.Datastore.MaxOpenConns = 25
	}
	if c.Datastore.MaxIdleConns == 0 {
		c.Datastore.MaxIdleConns = 5
	}
	if c.Datastore.ConnMaxLifetimeMinutes == 0 {
		c.Datastore.ConnMaxLifetimeMinutes = 5
	}
	if c.Datastore.ConnMaxIdleTimeMinutes == 0 {
		c.Datastore.ConnMaxIdleTimeMinutes = 1
	}

	// Server
	if c.Server.ListenAddress == "" {
		c.Server.ListenAddress = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.GracefulShutdownSeconds == 0 {
		c.Server.GracefulShutdownSeconds = 30
	}

	// WebSocket defaults — Enabled uses *bool so nil (not set) defaults to
	// true via IsEnabled(), while explicit `enabled: false` is preserved.
	if c.Server.WebSocket.MaxConnections == 0 {
		c.Server.WebSocket.MaxConnections = 100
	}
	if c.Server.WebSocket.SendBufferSize == 0 {
		c.Server.WebSocket.SendBufferSize = 64
	}
	if c.Server.WebSocket.WriteTimeoutSeconds == 0 {
		c.Server.WebSocket.WriteTimeoutSeconds = 10
	}
	if c.Server.WebSocket.PingIntervalSeconds == 0 {
		c.Server.WebSocket.PingIntervalSeconds = 30
	}
	if c.Server.WebSocket.PongTimeoutSeconds == 0 {
		c.Server.WebSocket.PongTimeoutSeconds = 60
	}

	// TLS defaults
	c.resolveTLSMode()
	if c.Server.TLS.MinVersion == "" {
		c.Server.TLS.MinVersion = "1.2"
	}
	if c.Server.TLS.ACME.CAURL == "" {
		c.Server.TLS.ACME.CAURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	if c.Server.TLS.ACME.Challenge == "" {
		c.Server.TLS.ACME.Challenge = "http-01"
	}
	if c.Server.TLS.ACME.StoragePath == "" {
		c.Server.TLS.ACME.StoragePath = "/var/lib/chef-migration-metrics/acme"
	}
	if c.Server.TLS.ACME.RenewBeforeDays == 0 {
		c.Server.TLS.ACME.RenewBeforeDays = 30
	}

	// Performance defaults — Enabled uses *bool so nil defaults to true
	// via IsEnabled(). WindowSeconds defaults to 300 (5 minutes).
	if c.Performance.WindowSeconds == 0 {
		c.Performance.WindowSeconds = 300
	}

	// Frontend
	if c.Frontend.BasePath == "" {
		c.Frontend.BasePath = "/"
	}

	// Auth
	if c.Auth.SessionExpiry == "" {
		c.Auth.SessionExpiry = "8h"
	}
	if c.Auth.MinPasswordLength == 0 {
		c.Auth.MinPasswordLength = 8
	}
	if c.Auth.LockoutAttempts == 0 {
		c.Auth.LockoutAttempts = 5
	}

	// Logging
	if c.Logging.Level == "" {
		c.Logging.Level = "INFO"
	}
	if c.Logging.RetentionDays == 0 {
		c.Logging.RetentionDays = 90
	}

	// Ownership
	if c.Ownership.AuditLog.RetentionDays == 0 {
		c.Ownership.AuditLog.RetentionDays = 365
	}
	for i := range c.Ownership.AutoRules {
		if c.Ownership.AutoRules[i].Type == "cmdb_attribute" && c.Ownership.AutoRules[i].OwnerAttribute == "" {
			c.Ownership.AutoRules[i].OwnerAttribute = "owner"
		}
	}

	// System health
	if len(c.SystemHealth.DiskPaths) == 0 {
		c.SystemHealth.DiskPaths = []string{
			c.Storage.DataDir,
			c.Storage.CookbookCacheDir,
			c.Storage.GitCookbookDir,
			c.Exports.OutputDirectory,
		}
	}
	if c.SystemHealth.DiskUsedWarningPercent == 0 {
		c.SystemHealth.DiskUsedWarningPercent = 80
	}
	if c.SystemHealth.DiskUsedCriticalPercent == 0 {
		c.SystemHealth.DiskUsedCriticalPercent = 90
	}
	if c.SystemHealth.CPULoadWarningPerCPU == 0 {
		c.SystemHealth.CPULoadWarningPerCPU = 2.0
	}
	if c.SystemHealth.CPULoadCriticalPerCPU == 0 {
		c.SystemHealth.CPULoadCriticalPerCPU = 4.0
	}
	if c.SystemHealth.MemUsedWarningPercent == 0 {
		c.SystemHealth.MemUsedWarningPercent = 80
	}
	if c.SystemHealth.MemUsedCriticalPercent == 0 {
		c.SystemHealth.MemUsedCriticalPercent = 90
	}
	if c.SystemHealth.PauseCollectionOnCritical == nil {
		t := true
		c.SystemHealth.PauseCollectionOnCritical = &t
	}

	// Performance
	if c.Performance.WindowSeconds <= 0 {
		c.Performance.WindowSeconds = 300
	}
}

// resolveTLSMode handles the deprecated tls.enabled boolean and normalises
// the mode field.
func (c *Config) resolveTLSMode() {
	if c.Server.TLS.Mode != "" {
		// Explicit mode always wins.
		return
	}
	if c.Server.TLS.Enabled != nil && *c.Server.TLS.Enabled {
		c.Server.TLS.Mode = "static"
	} else {
		c.Server.TLS.Mode = "off"
	}
}

// ---------------------------------------------------------------------------
// Environment variable overrides
// ---------------------------------------------------------------------------

// applyEnvOverrides applies well-known environment variable overrides on top
// of the loaded configuration.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.Datastore.URL = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.Port = p
		}
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_MODE"); v != "" {
		c.Server.TLS.Mode = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_PATH"); v != "" {
		c.Server.TLS.CertPath = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_KEY_PATH"); v != "" {
		c.Server.TLS.KeyPath = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_CA_PATH"); v != "" {
		c.Server.TLS.CAPath = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_MIN_VERSION"); v != "" {
		c.Server.TLS.MinVersion = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_HTTP_REDIRECT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.TLS.HTTPRedirectPort = p
		}
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_EMAIL"); v != "" {
		c.Server.TLS.ACME.Email = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CA_URL"); v != "" {
		c.Server.TLS.ACME.CAURL = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CHALLENGE"); v != "" {
		c.Server.TLS.ACME.Challenge = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_DNS_PROVIDER"); v != "" {
		c.Server.TLS.ACME.DNSProvider = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_STORAGE_PATH"); v != "" {
		c.Server.TLS.ACME.StoragePath = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS"); v != "" {
		c.Server.TLS.ACME.AgreeToTOS = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_ANALYSIS_TOOLS_EMBEDDED_BIN_DIR"); v != "" {
		c.AnalysisTools.EmbeddedBinDir = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_ELASTICSEARCH_ENABLED"); v != "" {
		c.Elasticsearch.Enabled = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_ELASTICSEARCH_OUTPUT_DIRECTORY"); v != "" {
		c.Elasticsearch.OutputDirectory = v
	}
	if v := os.Getenv("CMM_OWNERSHIP_AUDIT_LOG_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			c.Ownership.AuditLog.RetentionDays = n
		}
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ValidationError collects one or more configuration validation failures.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("configuration validation failed:\n  - %s", strings.Join(e.Errors, "\n  - "))
}

// add appends an error message to the list.
func (e *ValidationError) add(msg string) {
	e.Errors = append(e.Errors, msg)
}

// addf appends a formatted error message.
func (e *ValidationError) addf(format string, args ...any) {
	e.Errors = append(e.Errors, fmt.Sprintf(format, args...))
}

// hasErrors returns true if any errors have been recorded.
func (e *ValidationError) hasErrors() bool {
	return len(e.Errors) > 0
}

// Warnings collects non-fatal issues detected during validation.
type Warnings struct {
	Messages []string
}

func (w *Warnings) add(msg string) {
	w.Messages = append(w.Messages, msg)
}

func (w *Warnings) addf(format string, args ...any) {
	w.Messages = append(w.Messages, fmt.Sprintf(format, args...))
}

// Validate checks the configuration against the specification rules. It
// returns a non-nil *ValidationError if fatal problems are found and a
// Warnings struct for non-fatal issues.
func (c *Config) Validate() (*Warnings, error) {
	ve := &ValidationError{}
	w := &Warnings{}

	c.validateOrganisations(ve)
	c.validateTargetVersions(ve)
	c.validateCollection(ve, w)
	c.validateConcurrency(ve)
	c.validateAnalysisTools(ve, w)
	c.validateExports(ve, w)
	c.validateElasticsearch(ve, w)
	c.validateServer(ve, w)
	c.validateLogging(ve)
	c.validateAuth(ve)
	c.validateOwnership(ve)

	if ve.hasErrors() {
		return w, ve
	}
	return w, nil
}

// --- per-section validators ---

func (c *Config) validateOrganisations(ve *ValidationError) {
	if len(c.Organisations) == 0 {
		ve.add("at least one organisation must be configured")
		return
	}
	seen := make(map[string]bool)
	for i, org := range c.Organisations {
		prefix := fmt.Sprintf("organisations[%d]", i)
		if org.Name == "" {
			ve.addf("%s: name is required", prefix)
		} else if seen[org.Name] {
			ve.addf("%s: duplicate organisation name %q", prefix, org.Name)
		} else {
			seen[org.Name] = true
		}
		if org.ChefServerURL == "" {
			ve.addf("%s: chef_server_url is required", prefix)
		}
		if org.OrgName == "" {
			ve.addf("%s: org_name is required", prefix)
		}
		if org.ClientName == "" {
			ve.addf("%s: client_name is required", prefix)
		}
		if org.ClientKeyPath == "" && org.ClientKeyCredential == "" {
			ve.addf("%s: one of client_key_path or client_key_credential is required", prefix)
		}
		if org.ClientKeyPath != "" {
			if _, err := os.Stat(org.ClientKeyPath); err != nil {
				ve.addf("%s: client_key_path %q: %v", prefix, org.ClientKeyPath, err)
			}
		}
	}
}

// semverRe is a simple check for major.minor.patch format.
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func (c *Config) validateTargetVersions(ve *ValidationError) {
	for i, v := range c.TargetChefVersions {
		if !semverRe.MatchString(v) {
			ve.addf("target_chef_versions[%d]: %q is not a valid semver string (expected MAJOR.MINOR.PATCH)", i, v)
		}
	}
}

// HighestVersion returns the highest semver string from a slice of version
// strings. Each string is expected to be in MAJOR.MINOR.PATCH format (as
// validated by validateTargetVersions). Returns an empty string if the
// slice is empty.
func HighestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}

	best := versions[0]
	bestParts := parseSemverParts(best)

	for _, v := range versions[1:] {
		parts := parseSemverParts(v)
		if compareSemverParts(parts, bestParts) > 0 {
			best = v
			bestParts = parts
		}
	}

	return best
}

// chefMajorVersionFromString returns the major version number from a
// "MAJOR.MINOR.PATCH" string. Returns 0 for invalid strings.
func chefMajorVersionFromString(v string) int {
	return parseSemverParts(v)[0]
}

// parseSemverParts splits a "MAJOR.MINOR.PATCH" string into three ints.
// Non-numeric or missing segments default to 0.
func parseSemverParts(v string) [3]int {
	var parts [3]int
	segs := strings.SplitN(v, ".", 3)
	for i := 0; i < len(segs) && i < 3; i++ {
		n, _ := strconv.Atoi(segs[i])
		parts[i] = n
	}
	return parts
}

// compareSemverParts returns >0 if a > b, <0 if a < b, 0 if equal.
func compareSemverParts(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}
	return 0
}

// cronFieldRe is a deliberately permissive check — a cron expression has 5
// space-separated fields. Full parsing is left to the scheduler library.
var cronFieldRe = regexp.MustCompile(`^(\S+\s+){4}\S+$`)

func (c *Config) validateCollection(ve *ValidationError, w *Warnings) {
	if !cronFieldRe.MatchString(c.Collection.Schedule) {
		ve.addf("collection.schedule: %q is not a valid cron expression", c.Collection.Schedule)
	}
	if c.Collection.StaleNodeThresholdDays < 1 {
		ve.add("collection.stale_node_threshold_days must be >= 1")
	}
	if c.Collection.StaleNodeWarningHours < 1 {
		ve.add("collection.stale_node_warning_hours must be >= 1")
	}
	if c.Collection.StaleNodeCriticalDays < 1 {
		ve.add("collection.stale_node_critical_days must be >= 1")
	}
	// Warning threshold must be strictly less than critical threshold.
	warningAsDays := float64(c.Collection.StaleNodeWarningHours) / 24.0
	if warningAsDays >= float64(c.Collection.StaleNodeCriticalDays) {
		ve.add("collection.stale_node_warning_hours must be less than stale_node_critical_days (converted to same unit)")
	}
	if c.Collection.StaleCookbookThresholdDays < 1 {
		ve.add("collection.stale_cookbook_threshold_days must be >= 1")
	}

}

func (c *Config) validateConcurrency(ve *ValidationError) {
	check := func(name string, val int) {
		if val < 1 {
			ve.addf("concurrency.%s must be >= 1", name)
		}
	}
	check("organisation_collection", c.Concurrency.OrganisationCollection)
	check("node_page_fetching", c.Concurrency.NodePageFetching)
	check("git_pull", c.Concurrency.GitPull)
	check("cookbook_download", c.Concurrency.CookbookDownload)
	check("cookstyle_scan", c.Concurrency.CookstyleScan)
	check("readiness_evaluation", c.Concurrency.ReadinessEvaluation)
}

func (c *Config) validateAnalysisTools(ve *ValidationError, w *Warnings) {
	if c.AnalysisTools.CookstyleTimeoutMinutes < 1 {
		ve.add("analysis_tools.cookstyle_timeout_minutes must be >= 1")
	}

	// Backward compat: migrate deprecated test_kitchen_timeout_minutes.
	c.AnalysisTools.resolveTestKitchenBackwardCompat(w)

	if c.AnalysisTools.TestKitchenTimeoutMinutes < 0 {
		ve.add("analysis_tools.test_kitchen_timeout_minutes must be >= 0")
	}

	if c.AnalysisTools.EmbeddedBinDir != "" {
		if info, err := os.Stat(c.AnalysisTools.EmbeddedBinDir); err != nil || !info.IsDir() {
			w.addf("analysis_tools.embedded_bin_dir %q does not exist or is not a directory; falling back to PATH lookup", c.AnalysisTools.EmbeddedBinDir)
		}
	}

	// Validate Test Kitchen driver configuration.
	tk := c.AnalysisTools.TestKitchen

	if tk.TimeoutMinutes < 0 {
		ve.add("analysis_tools.test_kitchen.timeout_minutes must be >= 0")
	}

	// Driver profile validation.
	driver := tk.EffectiveDriver()
	knownDrivers := map[string]bool{
		"vcenter": true, "vra": true, "ec2": true,
		"vagrant": true, "proxmox": true,
	}
	if driver != "" && !knownDrivers[driver] {
		w.addf("analysis_tools.test_kitchen.driver %q is not a recognised driver profile", driver)
	}

	// Hypervisor integration validation.
	knownHypervisors := map[string]bool{
		"vcenter": true, "proxmox": true, "": true,
	}
	if !knownHypervisors[tk.HypervisorType] {
		w.addf("analysis_tools.test_kitchen.hypervisor_type %q is not a recognised hypervisor type", tk.HypervisorType)
	}
	if tk.VMTTLHours < 0 {
		ve.add("analysis_tools.test_kitchen.vm_ttl_hours must be >= 0")
	}
	if tk.MaxConcurrentVMs < 0 {
		ve.add("analysis_tools.test_kitchen.max_concurrent_vms must be >= 0")
	}
	if tk.StartRateWindowMinutes < 0 {
		ve.add("analysis_tools.test_kitchen.start_rate_window_minutes must be >= 0")
	}
	if tk.StartRateMaxPerWindow < 0 {
		ve.add("analysis_tools.test_kitchen.start_rate_max_per_window must be >= 0")
	}
	if (tk.StartRateWindowMinutes > 0) != (tk.StartRateMaxPerWindow > 0) {
		w.addf("analysis_tools.test_kitchen: start-rate limiter needs both " +
			"start_rate_window_minutes and start_rate_max_per_window; only one is set, so the limiter is disabled")
	}

	// Setup-script glob patterns must be syntactically valid and non-empty.
	if tk.SetupScripts != nil {
		validateSetupScriptGlobs := func(family string, patterns []string) {
			for i, p := range patterns {
				if p == "" {
					ve.addf("analysis_tools.test_kitchen.setup_scripts.%s[%d] is empty", family, i)
					continue
				}
				if _, err := filepath.Match(p, ""); err != nil {
					ve.addf("analysis_tools.test_kitchen.setup_scripts.%s[%d] %q is not a valid glob: %v", family, i, p, err)
				}
			}
		}
		validateSetupScriptGlobs("linux", tk.SetupScripts.Linux)
		validateSetupScriptGlobs("windows", tk.SetupScripts.Windows)
	}

	// Image registry validation.
	seenImageNames := make(map[string]int, len(tk.Images))
	for i, img := range tk.Images {
		if img.Name == "" {
			ve.addf("analysis_tools.test_kitchen.images[%d].name is required", i)
		} else if prev, ok := seenImageNames[img.Name]; ok {
			ve.addf("analysis_tools.test_kitchen.images[%d].name %q duplicates entry [%d]", i, img.Name, prev)
		} else {
			seenImageNames[img.Name] = i
		}
		if img.ID == "" {
			w.addf("analysis_tools.test_kitchen.images[%d].id is empty; image %q will be skipped", i, img.Name)
		}
		if img.InstallMethod != "" && img.InstallMethod != "download" && img.InstallMethod != "baked_in" {
			ve.addf("analysis_tools.test_kitchen.images[%d].install_method %q must be \"download\" or \"baked_in\"", i, img.InstallMethod)
		}
		if img.InstallMethod == "baked_in" && img.ChefClientPath == "" {
			ve.addf("analysis_tools.test_kitchen.images[%d].chef_client_path is required when install_method is \"baked_in\"", i)
		}
		if img.InstallMethod != "baked_in" && img.ChefClientPath != "" {
			w.addf("analysis_tools.test_kitchen.images[%d].chef_client_path is set but install_method is not \"baked_in\"; it will be ignored", i)
		}
		for ver := range img.ChefDownloadURLs {
			found := false
			for _, tv := range c.TargetChefVersions {
				if tv == ver {
					found = true
					break
				}
			}
			if !found {
				w.addf("analysis_tools.test_kitchen.images[%d].chef_download_urls key %q is not in target_chef_versions", i, ver)
			}
		}
	}

	if len(tk.Images) == 0 {
		w.add("analysis_tools.test_kitchen.images is empty; Test Kitchen will skip all cookbooks")
	}

	// Platform map validation.
	seenKitchenNames := make(map[string]int, len(tk.PlatformMap))
	for i, entry := range tk.PlatformMap {
		if entry.KitchenName == "" {
			ve.addf("analysis_tools.test_kitchen.platform_map[%d].kitchen_name is required", i)
		} else if prev, ok := seenKitchenNames[entry.KitchenName]; ok {
			ve.addf("analysis_tools.test_kitchen.platform_map[%d].kitchen_name %q duplicates entry [%d]", i, entry.KitchenName, prev)
		} else {
			seenKitchenNames[entry.KitchenName] = i
		}
		if entry.Image == "" {
			w.addf("analysis_tools.test_kitchen.platform_map[%d].image is empty; platform %q will be skipped", i, entry.KitchenName)
		} else if _, ok := seenImageNames[entry.Image]; !ok {
			w.addf("analysis_tools.test_kitchen.platform_map[%d].image %q does not reference a defined image", i, entry.Image)
		}
	}

	if len(tk.PlatformMap) == 0 {
		w.add("analysis_tools.test_kitchen.platform_map is empty; Test Kitchen will skip all cookbooks")
	}

	// Chef license key validation for v19+.
	for _, v := range c.TargetChefVersions {
		if chefMajorVersionFromString(v) >= 19 && tk.ChefLicenseKeyCredential == "" {
			// Check if every image has a download_url for this version.
			allCovered := len(tk.Images) > 0
			for _, img := range tk.Images {
				if img.ChefDownloadURLs[v] == "" {
					allCovered = false
					break
				}
			}
			if !allCovered {
				w.addf("analysis_tools.test_kitchen: target version %q requires chef_license_key_credential or per-image chef_download_urls for Chef 19+ installation", v)
				break
			}
		}
	}
}

func (c *Config) validateExports(ve *ValidationError, w *Warnings) {
	if c.Exports.RetentionHours < 1 {
		ve.add("exports.retention_hours must be >= 1")
	}
	if c.Exports.MaxRows < 1 {
		ve.add("exports.max_rows must be >= 1")
	}
	if c.Exports.AsyncThreshold < 1 {
		ve.add("exports.async_threshold must be >= 1")
	}
	// Only validate the output directory if the user explicitly configured it.
	// The default path (/var/lib/...) may not exist in dev/test environments;
	// the application will create it at runtime if needed.
	if c.explicitExportsDir {
		if err := checkDirWritable(c.Exports.OutputDirectory); err != nil {
			ve.addf("exports.output_directory %q: %v", c.Exports.OutputDirectory, err)
		}
	}
}

func (c *Config) validateElasticsearch(ve *ValidationError, w *Warnings) {
	if !c.Elasticsearch.Enabled {
		return
	}
	if c.Elasticsearch.RetentionHours < 1 {
		ve.add("elasticsearch.retention_hours must be >= 1")
	}
	// Only validate the output directory if the user explicitly configured it
	// or if elasticsearch is enabled (user opted in, so the dir matters).
	if c.explicitESDir {
		if err := checkDirWritable(c.Elasticsearch.OutputDirectory); err != nil {
			ve.addf("elasticsearch.output_directory %q: %v", c.Elasticsearch.OutputDirectory, err)
		}
	}
}

func (c *Config) validateServer(ve *ValidationError, w *Warnings) {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		ve.addf("server.port: %d is not a valid port number (1-65535)", c.Server.Port)
	}

	// WebSocket validation
	if c.Server.WebSocket.MaxConnections < 1 {
		ve.addf("server.websocket.max_connections: must be at least 1, got %d", c.Server.WebSocket.MaxConnections)
	}
	if c.Server.WebSocket.SendBufferSize < 1 {
		ve.addf("server.websocket.send_buffer_size: must be at least 1, got %d", c.Server.WebSocket.SendBufferSize)
	}
	if c.Server.WebSocket.WriteTimeoutSeconds < 1 {
		ve.addf("server.websocket.write_timeout_seconds: must be at least 1, got %d", c.Server.WebSocket.WriteTimeoutSeconds)
	}
	if c.Server.WebSocket.PingIntervalSeconds < 1 {
		ve.addf("server.websocket.ping_interval_seconds: must be at least 1, got %d", c.Server.WebSocket.PingIntervalSeconds)
	}
	if c.Server.WebSocket.PongTimeoutSeconds <= c.Server.WebSocket.PingIntervalSeconds {
		ve.addf("server.websocket.pong_timeout_seconds: must be greater than ping_interval_seconds (%d), got %d",
			c.Server.WebSocket.PingIntervalSeconds, c.Server.WebSocket.PongTimeoutSeconds)
	}

	// Backward compatibility warning
	if c.Server.TLS.Enabled != nil && c.Server.TLS.Mode != "" {
		w.add("server.tls.enabled is deprecated; server.tls.mode takes precedence")
	}

	switch c.Server.TLS.Mode {
	case "off":
		// nothing to validate
	case "static":
		c.validateTLSStatic(ve, w)
	case "acme":
		c.validateTLSACME(ve)
	default:
		ve.addf("server.tls.mode: must be 'off', 'static', or 'acme', got %q", c.Server.TLS.Mode)
	}

	if c.Server.TLS.Mode == "static" || c.Server.TLS.Mode == "acme" {
		switch c.Server.TLS.MinVersion {
		case "1.2", "1.3":
			// ok
		default:
			ve.addf("server.tls.min_version: must be '1.2' or '1.3', got %q", c.Server.TLS.MinVersion)
		}
	}

	if c.Server.TLS.HTTPRedirectPort != 0 {
		if c.Server.TLS.HTTPRedirectPort < 1 || c.Server.TLS.HTTPRedirectPort > 65535 {
			ve.addf("server.tls.http_redirect_port: %d is not a valid port number (1-65535)", c.Server.TLS.HTTPRedirectPort)
		}
	}
}

func (c *Config) validateTLSStatic(ve *ValidationError, w *Warnings) {
	if c.Server.TLS.CertPath == "" {
		ve.add("server.tls.cert_path is required when tls.mode is 'static'")
	} else if _, err := os.Stat(c.Server.TLS.CertPath); err != nil {
		ve.addf("server.tls.cert_path %q: %v", c.Server.TLS.CertPath, err)
	}
	if c.Server.TLS.KeyPath == "" {
		ve.add("server.tls.key_path is required when tls.mode is 'static'")
	} else if info, err := os.Stat(c.Server.TLS.KeyPath); err != nil {
		ve.addf("server.tls.key_path %q: %v", c.Server.TLS.KeyPath, err)
	} else if info.Mode().Perm()&0o077 != 0 {
		w.addf("server.tls.key_path %q has permissions %o; recommended 0600", c.Server.TLS.KeyPath, info.Mode().Perm())
	}
	if c.Server.TLS.CAPath != "" {
		if _, err := os.Stat(c.Server.TLS.CAPath); err != nil {
			ve.addf("server.tls.ca_path %q: %v", c.Server.TLS.CAPath, err)
		}
	}
}

func (c *Config) validateTLSACME(ve *ValidationError) {
	if len(c.Server.TLS.ACME.Domains) == 0 {
		ve.add("server.tls.acme.domains is required when tls.mode is 'acme'")
	}
	if c.Server.TLS.ACME.Email == "" {
		ve.add("server.tls.acme.email is required when tls.mode is 'acme'")
	}
	if !c.Server.TLS.ACME.AgreeToTOS {
		ve.add("server.tls.acme.agree_to_tos must be true when tls.mode is 'acme'")
	}

	switch c.Server.TLS.ACME.Challenge {
	case "http-01":
		if c.Server.TLS.HTTPRedirectPort == 0 {
			ve.add("server.tls.http_redirect_port must be set when acme.challenge is 'http-01'")
		}
	case "tls-alpn-01":
		// no extra config needed
	case "dns-01":
		if c.Server.TLS.ACME.DNSProvider == "" {
			ve.add("server.tls.acme.dns_provider is required when acme.challenge is 'dns-01'")
		}
	default:
		ve.addf("server.tls.acme.challenge: must be 'http-01', 'tls-alpn-01', or 'dns-01', got %q", c.Server.TLS.ACME.Challenge)
	}

	if c.Server.TLS.ACME.RenewBeforeDays < 1 || c.Server.TLS.ACME.RenewBeforeDays > 89 {
		ve.addf("server.tls.acme.renew_before_days: must be between 1 and 89, got %d", c.Server.TLS.ACME.RenewBeforeDays)
	}

	if c.Server.TLS.ACME.CAURL != "" {
		if _, err := url.ParseRequestURI(c.Server.TLS.ACME.CAURL); err != nil {
			ve.addf("server.tls.acme.ca_url: not a valid URL: %v", err)
		}
	}

	if c.Server.TLS.ACME.TrustedRoots != "" {
		if _, err := os.Stat(c.Server.TLS.ACME.TrustedRoots); err != nil {
			ve.addf("server.tls.acme.trusted_roots %q: %v", c.Server.TLS.ACME.TrustedRoots, err)
		}
	}
}

func (c *Config) validateLogging(ve *ValidationError) {
	switch strings.ToUpper(c.Logging.Level) {
	case "DEBUG", "INFO", "WARN", "ERROR":
		// ok
	default:
		ve.addf("logging.level: must be one of DEBUG, INFO, WARN, ERROR, got %q", c.Logging.Level)
	}
}

func (c *Config) validateAuth(ve *ValidationError) {
	for i, p := range c.Auth.Providers {
		prefix := fmt.Sprintf("auth.providers[%d]", i)
		switch p.Type {
		case "local":
			// no additional config required
		case "saml":
			if p.IDPMetadataURL == "" && p.IDPMetadataPath == "" {
				ve.addf("%s: idp_metadata_url or idp_metadata_path is required for saml provider", prefix)
			} else if p.IDPMetadataURL != "" && !isHTTPSURL(p.IDPMetadataURL) {
				ve.addf("%s: idp_metadata_url must be an https:// URL", prefix)
			}
			if p.SPEntityID == "" {
				ve.addf("%s: sp_entity_id is required for saml provider", prefix)
			}
			if p.SPPrivateKeyCredential == "" {
				ve.addf("%s: sp_private_key_credential is required for saml provider", prefix)
			}
			if p.SPCertificateCredential == "" {
				ve.addf("%s: sp_certificate_credential is required for saml provider", prefix)
			}
			// Validate role_mapping values are known roles.
			validRoles := map[string]bool{"viewer": true, "operator": true, "admin": true}
			for group, role := range p.RoleMapping {
				if !validRoles[role] {
					ve.addf("%s: role_mapping[%q] has invalid role %q (expected viewer, operator, or admin)", prefix, group, role)
				}
			}
			// Validate duration fields if specified.
			if p.ClockSkewTolerance != "" {
				if _, err := time.ParseDuration(p.ClockSkewTolerance); err != nil {
					ve.addf("%s: clock_skew_tolerance is not a valid duration: %v", prefix, err)
				}
			}
			if p.MetadataRefreshInterval != "" {
				if _, err := time.ParseDuration(p.MetadataRefreshInterval); err != nil {
					ve.addf("%s: metadata_refresh_interval is not a valid duration: %v", prefix, err)
				}
			}
		default:
			ve.addf("%s: unknown provider type %q (expected local or saml)", prefix, p.Type)
		}
	}
}

func (c *Config) validateOwnership(ve *ValidationError) {
	names := make(map[string]bool)
	for i, rule := range c.Ownership.AutoRules {
		prefix := fmt.Sprintf("ownership.auto_rules[%d]", i)
		if rule.Name == "" {
			ve.addf("%s.name is required", prefix)
		} else if names[rule.Name] {
			ve.addf("%s.name %q is duplicated", prefix, rule.Name)
		} else {
			names[rule.Name] = true
		}
		// cmdb_attribute rules derive the owner from node attributes, so
		// the Owner field is not required (and should not be set).
		if rule.Type != "cmdb_attribute" {
			if rule.Owner == "" {
				ve.addf("%s.owner is required", prefix)
			}
		}
		switch rule.Type {
		case "node_attribute":
			if rule.AttributePath == "" {
				ve.addf("%s.attribute_path is required for node_attribute rules", prefix)
			}
			if rule.MatchValue == "" {
				ve.addf("%s.match_value is required for node_attribute rules", prefix)
			}
		case "node_name_pattern", "cookbook_name_pattern", "git_repo_url_pattern", "role_match":
			if rule.Pattern == "" {
				ve.addf("%s.pattern is required for %s rules", prefix, rule.Type)
			}
		case "policy_match":
			if rule.PolicyName == "" && rule.Pattern == "" {
				ve.addf("%s.policy_name or pattern is required for policy_match rules", prefix)
			}
		case "cmdb_attribute":
			if rule.ObjectType == "" {
				ve.addf("%s.object_type is required for cmdb_attribute rules", prefix)
			} else if !ValidCMDBObjectTypes[rule.ObjectType] {
				ve.addf("%s.object_type %q is not valid (must be one of: node, cookbook, profile, role)", prefix, rule.ObjectType)
			}
			if rule.Owner != "" {
				ve.addf("%s.owner must not be set for cmdb_attribute rules (owner is derived from the attribute value)", prefix)
			}
		case "":
			ve.addf("%s.type is required", prefix)
		default:
			ve.addf("%s.type %q is not valid (must be one of: node_attribute, node_name_pattern, policy_match, cookbook_name_pattern, git_repo_url_pattern, role_match, cmdb_attribute)", prefix, rule.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isHTTPSURL returns true if s starts with "https://".
func isHTTPSURL(s string) bool {
	return len(s) > 8 && strings.EqualFold(s[:8], "https://")
}

// checkDirWritable checks that the given path exists and is a writable
// directory. It tries to create it if it doesn't exist.
func checkDirWritable(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist")
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}

	// Quick writability probe — try creating a temp file.
	tmp := filepath.Join(path, ".config-write-probe")
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	f.Close()
	os.Remove(tmp)
	return nil
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// Load reads configuration from the given YAML file path, applies defaults,
// applies environment variable overrides, and validates the result.
// If path is empty, it checks the CHEF_MIGRATION_METRICS_CONFIG environment
// variable.
func Load(path string) (*Config, *Warnings, error) {
	if path == "" {
		path = os.Getenv("CHEF_MIGRATION_METRICS_CONFIG")
	}
	if path == "" {
		return nil, nil, fmt.Errorf("no configuration file path provided (set CHEF_MIGRATION_METRICS_CONFIG or pass path to Load)")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading configuration file: %w", err)
	}

	cfg, warnings, err := Parse(data)
	if err != nil {
		return cfg, warnings, err
	}
	return cfg, warnings, nil
}

// Parse unmarshals YAML bytes into a Config, applies defaults, applies
// environment variable overrides, and validates.
func Parse(data []byte) (*Config, *Warnings, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parsing configuration YAML: %w", err)
	}

	// Track which directory fields were explicitly set before defaults fill them in.
	cfg.explicitExportsDir = cfg.Exports.OutputDirectory != ""
	cfg.explicitESDir = cfg.Elasticsearch.OutputDirectory != ""

	cfg.setDefaults()
	cfg.applyEnvOverrides()

	// Environment overrides count as explicit.
	if os.Getenv("CHEF_MIGRATION_METRICS_ELASTICSEARCH_OUTPUT_DIRECTORY") != "" {
		cfg.explicitESDir = true
	}

	warnings, err := cfg.Validate()
	if err != nil {
		return &cfg, warnings, err
	}
	return &cfg, warnings, nil
}

// ParseRaw unmarshals YAML bytes into a Config, applies defaults, and
// applies environment variable overrides — but does NOT validate. This is
// used when loading a bootstrap-only YAML file (after YAML-to-DB
// migration); validation runs later on the assembled config from the
// database.
func ParseRaw(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing configuration YAML: %w", err)
	}

	// Also parse flat bootstrap keys written by writeBootstrapYAML
	// (database_url, listen_address, listen_port). These don't map to
	// Config's nested YAML structure so they are silently ignored by the
	// unmarshal above; we apply them explicitly before setDefaults so
	// that defaults don't clobber them and env overrides can still win.
	var bootstrap struct {
		DatabaseURL   string `yaml:"database_url"`
		ListenAddress string `yaml:"listen_address"`
		ListenPort    int    `yaml:"listen_port"`
	}
	if err := yaml.Unmarshal(data, &bootstrap); err == nil {
		if cfg.Datastore.URL == "" && bootstrap.DatabaseURL != "" {
			cfg.Datastore.URL = bootstrap.DatabaseURL
		}
		if cfg.Server.ListenAddress == "" && bootstrap.ListenAddress != "" {
			cfg.Server.ListenAddress = bootstrap.ListenAddress
		}
		if cfg.Server.Port == 0 && bootstrap.ListenPort != 0 {
			cfg.Server.Port = bootstrap.ListenPort
		}
	}

	cfg.explicitExportsDir = cfg.Exports.OutputDirectory != ""
	cfg.explicitESDir = cfg.Elasticsearch.OutputDirectory != ""

	cfg.setDefaults()
	cfg.applyEnvOverrides()

	return &cfg, nil
}

// LoadRaw reads a YAML config file and parses it without validation.
// This is the non-validating equivalent of Load, used for bootstrap
// YAML files where validation happens later on the assembled config.
func LoadRaw(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("CHEF_MIGRATION_METRICS_CONFIG")
	}
	if path == "" {
		return nil, fmt.Errorf("no configuration file path provided (set CHEF_MIGRATION_METRICS_CONFIG or pass path to Load)")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading configuration file: %w", err)
	}

	return ParseRaw(data)
}
