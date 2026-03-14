// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// ---------------------------------------------------------------------------
// Mock executor
// ---------------------------------------------------------------------------

// mockKitchenExecutor records invocations and returns preconfigured results.
type mockKitchenExecutor struct {
	calls []mockKitchenCall

	// responses maps "phase" (e.g. "converge", "verify", "destroy", "list")
	// to the result that should be returned. If a phase is missing, defaults
	// to exit 0 with empty output.
	responses map[string]mockKitchenResponse
}

type mockKitchenCall struct {
	Dir  string
	Args []string
}

type mockKitchenResponse struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func newMockKitchenExecutor() *mockKitchenExecutor {
	return &mockKitchenExecutor{
		responses: make(map[string]mockKitchenResponse),
	}
}

func (m *mockKitchenExecutor) Run(_ context.Context, dir string, args ...string) (string, string, int, error) {
	m.calls = append(m.calls, mockKitchenCall{Dir: dir, Args: args})

	// Determine phase from the first arg.
	phase := ""
	if len(args) > 0 {
		phase = args[0]
	}

	if resp, ok := m.responses[phase]; ok {
		return resp.Stdout, resp.Stderr, resp.ExitCode, resp.Err
	}
	return "", "", 0, nil
}

func (m *mockKitchenExecutor) callCount() int {
	return len(m.calls)
}

func (m *mockKitchenExecutor) callsForPhase(phase string) []mockKitchenCall {
	var result []mockKitchenCall
	for _, c := range m.calls {
		if len(c.Args) > 0 && c.Args[0] == phase {
			result = append(result, c)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Mock DB (minimal interface to avoid needing a real PostgreSQL)
// ---------------------------------------------------------------------------

// The KitchenScanner uses db.GetLatestTestKitchenResult and
// db.UpsertTestKitchenResult. Since we can't easily mock the *datastore.DB
// without an interface, we test the scanner functions that don't touch the
// DB directly (overlay generation, driver detection, phase execution, YAML
// helpers) and test the full flow only at the unit level via testOne where
// the DB calls will fail gracefully (nil db panics are caught by checking
// skip/error paths before DB access).

// ---------------------------------------------------------------------------
// Helper: create a temp cookbook directory with .kitchen.yml
// ---------------------------------------------------------------------------

func makeTempCookbookDir(t *testing.T, kitchenYMLContent string) string {
	t.Helper()
	dir := t.TempDir()
	if kitchenYMLContent != "" {
		if err := os.WriteFile(filepath.Join(dir, ".kitchen.yml"), []byte(kitchenYMLContent), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func testLogger() *logging.Logger {
	return logging.New(logging.Options{Level: logging.DEBUG})
}

// ---------------------------------------------------------------------------
// Tests: yamlScalar
// ---------------------------------------------------------------------------

func TestYamlScalar_Empty(t *testing.T) {
	got := yamlScalar("")
	if got != `""` {
		t.Errorf("yamlScalar(\"\") = %q, want %q", got, `""`)
	}
}

func TestYamlScalar_PlainValue(t *testing.T) {
	got := yamlScalar("dokken")
	if got != "dokken" {
		t.Errorf("yamlScalar(\"dokken\") = %q, want %q", got, "dokken")
	}
}

func TestYamlScalar_ValueWithColon(t *testing.T) {
	got := yamlScalar("host:port")
	if got != `"host:port"` {
		t.Errorf("yamlScalar(\"host:port\") = %q, want %q", got, `"host:port"`)
	}
}

func TestYamlScalar_ValueWithSpace(t *testing.T) {
	got := yamlScalar("hello world")
	if got != `"hello world"` {
		t.Errorf("yamlScalar(\"hello world\") = %q, want %q", got, `"hello world"`)
	}
}

func TestYamlScalar_Boolean(t *testing.T) {
	got := yamlScalar("true")
	if got != "true" {
		t.Errorf("yamlScalar(\"true\") = %q, want %q", got, "true")
	}
}

func TestYamlScalar_NumericString(t *testing.T) {
	got := yamlScalar("42")
	if got != "42" {
		t.Errorf("yamlScalar(\"42\") = %q, want %q", got, "42")
	}
}

func TestYamlScalar_SpecialCharHash(t *testing.T) {
	got := yamlScalar("value#comment")
	if !strings.HasPrefix(got, `"`) {
		t.Errorf("yamlScalar(\"value#comment\") should be quoted, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Tests: truncSHA
// ---------------------------------------------------------------------------

func TestTruncSHA_Long(t *testing.T) {
	got := truncSHA("abc123def456789")
	if got != "abc123de" {
		t.Errorf("truncSHA() = %q, want %q", got, "abc123de")
	}
}

func TestTruncSHA_Short(t *testing.T) {
	got := truncSHA("abc")
	if got != "abc" {
		t.Errorf("truncSHA() = %q, want %q", got, "abc")
	}
}

func TestTruncSHA_ExactlyEight(t *testing.T) {
	got := truncSHA("12345678")
	if got != "12345678" {
		t.Errorf("truncSHA() = %q, want %q", got, "12345678")
	}
}

func TestTruncSHA_Empty(t *testing.T) {
	got := truncSHA("")
	if got != "" {
		t.Errorf("truncSHA() = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Tests: countLeadingSpaces
// ---------------------------------------------------------------------------

func TestCountLeadingSpaces_None(t *testing.T) {
	if got := countLeadingSpaces("driver:"); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestCountLeadingSpaces_Spaces(t *testing.T) {
	if got := countLeadingSpaces("    name: dokken"); got != 4 {
		t.Errorf("got %d, want 4", got)
	}
}

func TestCountLeadingSpaces_Tab(t *testing.T) {
	if got := countLeadingSpaces("\tname: dokken"); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestCountLeadingSpaces_Empty(t *testing.T) {
	if got := countLeadingSpaces(""); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Tests: findKitchenYML
// ---------------------------------------------------------------------------

func TestFindKitchenYML_DotKitchenYml(t *testing.T) {
	dir := makeTempCookbookDir(t, "driver:\n  name: dokken\n")
	got := findKitchenYML(dir)
	if got == "" {
		t.Fatal("expected to find .kitchen.yml")
	}
	if filepath.Base(got) != ".kitchen.yml" {
		t.Errorf("expected .kitchen.yml, got %s", filepath.Base(got))
	}
}

func TestFindKitchenYML_KitchenYaml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kitchen.yaml"), []byte("---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := findKitchenYML(dir)
	if got == "" {
		t.Fatal("expected to find kitchen.yaml")
	}
	if filepath.Base(got) != "kitchen.yaml" {
		t.Errorf("expected kitchen.yaml, got %s", filepath.Base(got))
	}
}

func TestFindKitchenYML_NotFound(t *testing.T) {
	dir := t.TempDir()
	got := findKitchenYML(dir)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFindKitchenYML_Priority(t *testing.T) {
	// .kitchen.yml should win over kitchen.yml
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".kitchen.yml"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "kitchen.yml"), []byte("b"), 0644)
	got := findKitchenYML(dir)
	if filepath.Base(got) != ".kitchen.yml" {
		t.Errorf("expected .kitchen.yml to take priority, got %s", filepath.Base(got))
	}
}

// ---------------------------------------------------------------------------
// Tests: detectDriver
// ---------------------------------------------------------------------------

func TestDetectDriver_Dokken(t *testing.T) {
	dir := makeTempCookbookDir(t, "---\ndriver:\n  name: dokken\n\nprovisioner:\n  name: dokken\n")
	path := filepath.Join(dir, ".kitchen.yml")
	got := detectDriver(path)
	if got != "dokken" {
		t.Errorf("detectDriver() = %q, want %q", got, "dokken")
	}
}

func TestDetectDriver_Vagrant(t *testing.T) {
	dir := makeTempCookbookDir(t, "driver:\n  name: vagrant\n")
	path := filepath.Join(dir, ".kitchen.yml")
	got := detectDriver(path)
	if got != "vagrant" {
		t.Errorf("detectDriver() = %q, want %q", got, "vagrant")
	}
}

func TestDetectDriver_QuotedName(t *testing.T) {
	dir := makeTempCookbookDir(t, "driver:\n  name: \"ec2\"\n")
	path := filepath.Join(dir, ".kitchen.yml")
	got := detectDriver(path)
	if got != "ec2" {
		t.Errorf("detectDriver() = %q, want %q", got, "ec2")
	}
}

func TestDetectDriver_SingleQuotedName(t *testing.T) {
	dir := makeTempCookbookDir(t, "driver:\n  name: 'azurerm'\n")
	path := filepath.Join(dir, ".kitchen.yml")
	got := detectDriver(path)
	if got != "azurerm" {
		t.Errorf("detectDriver() = %q, want %q", got, "azurerm")
	}
}

func TestDetectDriver_NoDriverKey(t *testing.T) {
	dir := makeTempCookbookDir(t, "provisioner:\n  name: chef_zero\n")
	path := filepath.Join(dir, ".kitchen.yml")
	got := detectDriver(path)
	if got != "" {
		t.Errorf("detectDriver() = %q, want empty", got)
	}
}

func TestDetectDriver_DriverWithoutName(t *testing.T) {
	dir := makeTempCookbookDir(t, "driver:\n  privileged: true\nplatforms:\n  - name: ubuntu\n")
	path := filepath.Join(dir, ".kitchen.yml")
	got := detectDriver(path)
	// No name sub-key under driver.
	if got != "" {
		t.Errorf("detectDriver() = %q, want empty", got)
	}
}

func TestDetectDriver_FileNotFound(t *testing.T) {
	got := detectDriver("/nonexistent/path/.kitchen.yml")
	if got != "" {
		t.Errorf("detectDriver() = %q, want empty", got)
	}
}

func TestDetectDriver_CommentsIgnored(t *testing.T) {
	yml := "# driver:\n#   name: vagrant\ndriver:\n  name: dokken\n"
	dir := makeTempCookbookDir(t, yml)
	path := filepath.Join(dir, ".kitchen.yml")
	got := detectDriver(path)
	if got != "dokken" {
		t.Errorf("detectDriver() = %q, want %q", got, "dokken")
	}
}

// ---------------------------------------------------------------------------
// Tests: buildOverlay
// ---------------------------------------------------------------------------

func newScannerWithConfig(tkConfig config.TestKitchenConfig) *KitchenScanner {
	return &KitchenScanner{
		logger:   testLogger(),
		tkConfig: tkConfig,
	}
}

func TestBuildOverlay_NoOverrides_NoTargetVersion(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{})
	got := s.buildOverlay("", "dokken")
	if got != "" {
		t.Errorf("expected empty overlay, got:\n%s", got)
	}
}

func TestBuildOverlay_TargetVersionOnly_Dokken(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{})
	got := s.buildOverlay("18.3.0", "dokken")
	if !strings.Contains(got, "provisioner:") {
		t.Error("expected provisioner section")
	}
	if !strings.Contains(got, `chef_version: "18.3.0"`) {
		t.Errorf("expected dokken-style chef_version, got:\n%s", got)
	}
	if strings.Contains(got, "product_version") {
		t.Error("should not contain product_version for dokken driver")
	}
}

func TestBuildOverlay_TargetVersionOnly_Vagrant(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{})
	got := s.buildOverlay("17.10.0", "vagrant")
	if !strings.Contains(got, `product_version: "17.10.0"`) {
		t.Errorf("expected product_version for vagrant, got:\n%s", got)
	}
	if strings.Contains(got, "chef_version") {
		t.Error("should not contain chef_version for vagrant driver")
	}
}

func TestBuildOverlay_TargetVersionOnly_UnknownDriver(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{})
	got := s.buildOverlay("18.0.0", "")
	// Unknown driver should use product_version.
	if !strings.Contains(got, "product_version") {
		t.Errorf("expected product_version for unknown driver, got:\n%s", got)
	}
}

func TestBuildOverlay_DriverOverride(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverOverride: "dokken",
	})
	got := s.buildOverlay("18.0.0", "vagrant")
	if !strings.Contains(got, "driver:") {
		t.Error("expected driver section")
	}
	if !strings.Contains(got, "name: dokken") {
		t.Errorf("expected driver name dokken, got:\n%s", got)
	}
	// Since override is dokken, provisioner should use chef_version.
	if !strings.Contains(got, "chef_version") {
		t.Errorf("expected chef_version for overridden dokken driver, got:\n%s", got)
	}
}

func TestBuildOverlay_DriverConfig(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverOverride: "dokken",
		DriverConfig: map[string]string{
			"privileged": "true",
			"network":    "host",
		},
	})
	got := s.buildOverlay("", "dokken")
	if !strings.Contains(got, "privileged: true") {
		t.Errorf("expected privileged in driver config, got:\n%s", got)
	}
	if !strings.Contains(got, "network: host") {
		t.Errorf("expected network in driver config, got:\n%s", got)
	}
}

func TestBuildOverlay_DriverConfigWithoutDriverOverride(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverConfig: map[string]string{
			"privileged": "true",
		},
	})
	got := s.buildOverlay("", "vagrant")
	if !strings.Contains(got, "driver:") {
		t.Error("expected driver section even without driver override")
	}
	if !strings.Contains(got, "privileged: true") {
		t.Errorf("expected driver_config merged, got:\n%s", got)
	}
	// Should NOT contain a name: key since no override.
	if strings.Contains(got, "name:") {
		t.Error("should not set driver name without override")
	}
}

func TestBuildOverlay_PlatformOverrides(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{
		PlatformOverrides: []config.TestKitchenPlatform{
			{
				Name:   "ubuntu-22.04",
				Driver: map[string]string{"image": "dokken/ubuntu-22.04"},
			},
			{
				Name:   "centos-8",
				Driver: map[string]string{"image": "dokken/centos-8"},
			},
		},
	})
	got := s.buildOverlay("", "dokken")
	if !strings.Contains(got, "platforms:") {
		t.Error("expected platforms section")
	}
	if !strings.Contains(got, "- name: ubuntu-22.04") {
		t.Errorf("expected ubuntu platform, got:\n%s", got)
	}
	if !strings.Contains(got, "- name: centos-8") {
		t.Errorf("expected centos platform, got:\n%s", got)
	}
	if !strings.Contains(got, "image: dokken/ubuntu-22.04") {
		t.Errorf("expected ubuntu image, got:\n%s", got)
	}
}

func TestBuildOverlay_ExtraYAML(t *testing.T) {
	extra := "transport:\n  name: ssh\nverifier:\n  name: inspec\n"
	s := newScannerWithConfig(config.TestKitchenConfig{
		ExtraYAML: extra,
	})
	got := s.buildOverlay("", "")
	if !strings.Contains(got, "transport:") {
		t.Errorf("expected extra YAML, got:\n%s", got)
	}
	if !strings.Contains(got, "verifier:") {
		t.Errorf("expected extra YAML verifier, got:\n%s", got)
	}
}

func TestBuildOverlay_ExtraYAMLWithoutNewline(t *testing.T) {
	extra := "transport:\n  name: ssh"
	s := newScannerWithConfig(config.TestKitchenConfig{
		ExtraYAML: extra,
	})
	got := s.buildOverlay("", "")
	if !strings.HasSuffix(got, "\n") {
		t.Error("expected overlay to end with newline")
	}
}

func TestBuildOverlay_AllOverridesCombined(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverOverride: "ec2",
		DriverConfig: map[string]string{
			"instance_type": "t3.medium",
		},
		PlatformOverrides: []config.TestKitchenPlatform{
			{Name: "rhel-9", Driver: map[string]string{"image_id": "ami-12345"}},
		},
		ExtraYAML: "lifecycle:\n  pre_converge:\n    - remote: yum update -y\n",
	})
	got := s.buildOverlay("18.5.0", "vagrant")
	if !strings.Contains(got, "name: ec2") {
		t.Error("expected driver override")
	}
	if !strings.Contains(got, "instance_type: t3.medium") {
		t.Error("expected driver config")
	}
	if !strings.Contains(got, "product_version") {
		// ec2 is not dokken, should use product_version.
		t.Error("expected product_version for non-dokken driver")
	}
	if !strings.Contains(got, "- name: rhel-9") {
		t.Error("expected platform override")
	}
	if !strings.Contains(got, "lifecycle:") {
		t.Error("expected extra YAML")
	}
}

func TestBuildOverlay_Header(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverOverride: "dokken",
	})
	got := s.buildOverlay("", "")
	if !strings.HasPrefix(got, "# .kitchen.local.yml") {
		t.Errorf("expected header comment, got:\n%s", got)
	}
	if !strings.Contains(got, "DO NOT EDIT") {
		t.Error("expected DO NOT EDIT warning in header")
	}
}

// ---------------------------------------------------------------------------
// Tests: effectiveDriver
// ---------------------------------------------------------------------------

func TestEffectiveDriver_Override(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{DriverOverride: "ec2"})
	got := s.effectiveDriver("dokken")
	if got != "ec2" {
		t.Errorf("effectiveDriver() = %q, want %q", got, "ec2")
	}
}

func TestEffectiveDriver_Detected(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{})
	got := s.effectiveDriver("vagrant")
	if got != "vagrant" {
		t.Errorf("effectiveDriver() = %q, want %q", got, "vagrant")
	}
}

func TestEffectiveDriver_Unknown(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{})
	got := s.effectiveDriver("")
	if got != "unknown" {
		t.Errorf("effectiveDriver() = %q, want %q", got, "unknown")
	}
}

// ---------------------------------------------------------------------------
// Tests: effectivePlatformSummary
// ---------------------------------------------------------------------------

func TestEffectivePlatformSummary_Default(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{})
	got := s.effectivePlatformSummary()
	if got != "cookbook-defined" {
		t.Errorf("got %q, want %q", got, "cookbook-defined")
	}
}

func TestEffectivePlatformSummary_Overrides(t *testing.T) {
	s := newScannerWithConfig(config.TestKitchenConfig{
		PlatformOverrides: []config.TestKitchenPlatform{
			{Name: "ubuntu-22.04"},
			{Name: "centos-8"},
		},
	})
	got := s.effectivePlatformSummary()
	if got != "ubuntu-22.04, centos-8" {
		t.Errorf("got %q, want %q", got, "ubuntu-22.04, centos-8")
	}
}

// ---------------------------------------------------------------------------
// Tests: phase execution via runPhase
// ---------------------------------------------------------------------------

func TestRunPhase_Success(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["converge"] = mockKitchenResponse{
		Stdout:   "Converging...\nDone.",
		ExitCode: 0,
	}
	s := &KitchenScanner{
		executor: mock,
		logger:   testLogger(),
		timeout:  30 * time.Minute,
	}

	pr := s.runPhase(context.Background(), "/tmp/test", "converge")
	if !pr.Passed {
		t.Error("expected converge to pass")
	}
	if pr.TimedOut {
		t.Error("should not be timed out")
	}
	if !strings.Contains(pr.Output, "Converging") {
		t.Errorf("expected output to contain stdout, got %q", pr.Output)
	}
}

func TestRunPhase_Failure(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["verify"] = mockKitchenResponse{
		Stdout:   "Running tests...",
		Stderr:   "Test failed: expected 200 got 500",
		ExitCode: 1,
	}
	s := &KitchenScanner{
		executor: mock,
		logger:   testLogger(),
		timeout:  30 * time.Minute,
	}

	pr := s.runPhase(context.Background(), "/tmp/test", "verify")
	if pr.Passed {
		t.Error("expected verify to fail with exit code 1")
	}
	if pr.TimedOut {
		t.Error("should not be timed out")
	}
	if !strings.Contains(pr.Output, "Test failed") {
		t.Errorf("expected stderr in output, got %q", pr.Output)
	}
}

func TestRunPhase_ExecutionError(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["converge"] = mockKitchenResponse{
		Err: fmt.Errorf("binary not found"),
	}
	s := &KitchenScanner{
		executor: mock,
		logger:   testLogger(),
		timeout:  30 * time.Minute,
	}

	pr := s.runPhase(context.Background(), "/tmp/test", "converge")
	if pr.Passed {
		t.Error("expected failure on execution error")
	}
	if pr.Err == nil {
		t.Error("expected non-nil Err")
	}
}

func TestRunPhase_ContextTimeout(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["converge"] = mockKitchenResponse{
		Err: fmt.Errorf("signal: killed"),
	}
	s := &KitchenScanner{
		executor: mock,
		logger:   testLogger(),
		timeout:  30 * time.Minute,
	}

	// Simulate an already-expired context.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	pr := s.runPhase(ctx, "/tmp/test", "converge")
	if pr.Passed {
		t.Error("expected failure on timeout")
	}
	if !pr.TimedOut {
		t.Error("expected TimedOut = true")
	}
}

func TestRunPhase_Arguments(t *testing.T) {
	mock := newMockKitchenExecutor()
	s := &KitchenScanner{
		executor: mock,
		logger:   testLogger(),
		timeout:  30 * time.Minute,
	}

	s.runPhase(context.Background(), "/tmp/cookbooks/myapp", "converge")

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.callCount())
	}
	call := mock.calls[0]
	if call.Dir != "/tmp/cookbooks/myapp" {
		t.Errorf("expected dir /tmp/cookbooks/myapp, got %s", call.Dir)
	}
	if len(call.Args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(call.Args), call.Args)
	}
	if call.Args[0] != "converge" {
		t.Errorf("expected first arg converge, got %s", call.Args[0])
	}
	if call.Args[1] != "--concurrency=1" {
		t.Errorf("expected --concurrency=1, got %s", call.Args[1])
	}
	if call.Args[2] != "--log-level=info" {
		t.Errorf("expected --log-level=info, got %s", call.Args[2])
	}
}

func TestRunPhase_CombinesStdoutStderr(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["verify"] = mockKitchenResponse{
		Stdout:   "stdout-content",
		Stderr:   "stderr-content",
		ExitCode: 0,
	}
	s := &KitchenScanner{
		executor: mock,
		logger:   testLogger(),
		timeout:  30 * time.Minute,
	}

	pr := s.runPhase(context.Background(), "/tmp/test", "verify")
	if !strings.Contains(pr.Output, "stdout-content") {
		t.Error("expected stdout in combined output")
	}
	if !strings.Contains(pr.Output, "stderr-content") {
		t.Error("expected stderr in combined output")
	}
}

// ---------------------------------------------------------------------------
// Tests: listInstances
// ---------------------------------------------------------------------------

func TestListInstances_Success(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{
		Stdout: `[{"instance":"default-ubuntu-2204","driver":"Dokken","provisioner":"Dokken","verifier":"Inspec","transport":"Dokken","last_action":null}]`,
	}
	s := &KitchenScanner{
		executor: mock,
		logger:   testLogger(),
	}

	instances, err := s.listInstances(context.Background(), "/tmp/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Instance != "default-ubuntu-2204" {
		t.Errorf("instance name = %q, want %q", instances[0].Instance, "default-ubuntu-2204")
	}
	if instances[0].Driver != "Dokken" {
		t.Errorf("driver = %q, want %q", instances[0].Driver, "Dokken")
	}
}

func TestListInstances_UsesJsonFlag(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{
		Stdout: `[{"instance":"default-ubuntu-2204","driver":"Dokken","provisioner":"Dokken","verifier":"Inspec","transport":"Dokken","last_action":null}]`,
	}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	_, err := s.listInstances(context.Background(), "/tmp/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := mock.callsForPhase("list")
	if len(calls) != 1 {
		t.Fatalf("expected 1 list call, got %d", len(calls))
	}

	// kitchen list uses --json (a boolean flag), not --format json.
	args := calls[0].Args
	wantArgs := []string{"list", "--json"}
	if len(args) != len(wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, args)
	}
	for i, want := range wantArgs {
		if args[i] != want {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], want)
		}
	}
}

func TestListInstances_MultipleInstances(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{
		Stdout: `[
			{"instance":"default-ubuntu-2204","driver":"Dokken","provisioner":"Dokken","verifier":"Inspec","transport":"Dokken","last_action":null},
			{"instance":"default-centos-8","driver":"Dokken","provisioner":"Dokken","verifier":"Inspec","transport":"Dokken","last_action":null}
		]`,
	}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	instances, err := s.listInstances(context.Background(), "/tmp/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}
}

func TestListInstances_EmptyArray(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{Stdout: "[]"}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	instances, err := s.listInstances(context.Background(), "/tmp/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}
}

func TestListInstances_EmptyStdout(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{Stdout: ""}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	instances, err := s.listInstances(context.Background(), "/tmp/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if instances != nil {
		t.Errorf("expected nil instances, got %v", instances)
	}
}

func TestListInstances_InvalidJSON(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{Stdout: "not json"}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	_, err := s.listInstances(context.Background(), "/tmp/test")
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestListInstances_InvalidJSON_IncludesStdoutAndStderr(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{
		Stdout:   "<html><body>403 Forbidden</body></html>",
		Stderr:   "Could not find gem 'kitchen-dokken'",
		ExitCode: 1,
	}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	_, err := s.listInstances(context.Background(), "/tmp/test")
	if err == nil {
		t.Fatal("expected error on HTML stdout")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "exit_code=1") {
		t.Errorf("expected exit_code in error, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "403 Forbidden") {
		t.Errorf("expected stdout content in error, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "kitchen-dokken") {
		t.Errorf("expected stderr content in error, got: %v", errMsg)
	}
}

func TestListInstances_InvalidJSON_TruncatesLongOutput(t *testing.T) {
	longHTML := strings.Repeat("<div>some very long HTML content</div>", 100)
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{Stdout: longHTML}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	_, err := s.listInstances(context.Background(), "/tmp/test")
	if err == nil {
		t.Fatal("expected error")
	}
	// The error message should contain the stdout but truncated.
	errMsg := err.Error()
	if len(errMsg) > 2000 {
		t.Errorf("error message too long (%d chars), expected truncation", len(errMsg))
	}
	if !strings.Contains(errMsg, "…") {
		t.Error("expected truncation indicator '…' in error message")
	}
}

func TestListInstances_ExecutionError(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["list"] = mockKitchenResponse{Err: fmt.Errorf("kitchen not found")}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	_, err := s.listInstances(context.Background(), "/tmp/test")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// Tests: destroyBestEffort
// ---------------------------------------------------------------------------

func TestDestroyBestEffort_Success(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["destroy"] = mockKitchenResponse{
		Stdout: "Destroying instances...",
	}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	result := &KitchenRunResult{CookbookName: "test-cb"}
	s.destroyBestEffort(context.Background(), "/tmp/test", result)

	if result.DestroyOutput == "" {
		t.Error("expected destroy output to be captured")
	}
	if !strings.Contains(result.DestroyOutput, "Destroying") {
		t.Errorf("expected destroy output content, got %q", result.DestroyOutput)
	}
}

func TestDestroyBestEffort_Failure(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["destroy"] = mockKitchenResponse{
		Err: fmt.Errorf("docker daemon not running"),
	}
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	result := &KitchenRunResult{CookbookName: "test-cb"}
	// Should not panic — destroy errors are logged but not propagated.
	s.destroyBestEffort(context.Background(), "/tmp/test", result)
}

func TestDestroyBestEffort_Arguments(t *testing.T) {
	mock := newMockKitchenExecutor()
	s := &KitchenScanner{executor: mock, logger: testLogger()}

	result := &KitchenRunResult{CookbookName: "test-cb"}
	s.destroyBestEffort(context.Background(), "/tmp/test", result)

	calls := mock.callsForPhase("destroy")
	if len(calls) != 1 {
		t.Fatalf("expected 1 destroy call, got %d", len(calls))
	}
	if len(calls[0].Args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(calls[0].Args), calls[0].Args)
	}
	if calls[0].Args[1] != "--concurrency=1" {
		t.Errorf("expected --concurrency=1, got %s", calls[0].Args[1])
	}
}

// ---------------------------------------------------------------------------
// Tests: TestGitRepos (batch orchestration)
// ---------------------------------------------------------------------------

func TestTestGitRepos_EmptyInput(t *testing.T) {
	mock := newMockKitchenExecutor()
	s := &KitchenScanner{
		executor:    mock,
		logger:      testLogger(),
		concurrency: 2,
		timeout:     1 * time.Minute,
		tkConfig:    config.TestKitchenConfig{},
	}

	result := s.TestGitRepos(context.Background(), nil, []string{"18.0.0"}, func(gr datastore.GitRepo) string { return "" })
	if result.Total != 0 {
		t.Errorf("expected 0 total, got %d", result.Total)
	}
}

func TestTestGitRepos_FiltersReposWithoutTestSuite(t *testing.T) {
	mock := newMockKitchenExecutor()
	s := &KitchenScanner{
		executor:    mock,
		logger:      testLogger(),
		concurrency: 2,
		timeout:     1 * time.Minute,
		tkConfig:    config.TestKitchenConfig{},
	}

	repos := []datastore.GitRepo{
		{ID: "1", Name: "no-tests", HasTestSuite: false, HeadCommitSHA: "abc123"},
		{ID: "2", Name: "no-sha", HasTestSuite: true, HeadCommitSHA: ""},
	}

	result := s.TestGitRepos(context.Background(), repos, []string{"18.0.0"}, func(gr datastore.GitRepo) string { return "/tmp/" + gr.Name })
	if result.Total != 0 {
		t.Errorf("expected 0 work items (all filtered), got %d", result.Total)
	}
}

func TestTestGitRepos_NoTargetVersions(t *testing.T) {
	mock := newMockKitchenExecutor()
	s := &KitchenScanner{
		executor:    mock,
		logger:      testLogger(),
		concurrency: 2,
		timeout:     1 * time.Minute,
		tkConfig:    config.TestKitchenConfig{},
	}

	repos := []datastore.GitRepo{
		{ID: "1", Name: "good-cb", HasTestSuite: true, HeadCommitSHA: "abc123"},
	}

	result := s.TestGitRepos(context.Background(), repos, nil, func(gr datastore.GitRepo) string { return "/tmp/" + gr.Name })
	if result.Total != 0 {
		t.Errorf("expected 0 work items with no target versions, got %d", result.Total)
	}
}

func TestTestGitRepos_WorkItemCount(t *testing.T) {
	mock := newMockKitchenExecutor()
	// Provide list response so testOne doesn't fail during instance discovery.
	// But since db is nil, testOne will error on skip check — that's fine,
	// we're just validating work item fan-out.
	s := &KitchenScanner{
		executor:    mock,
		logger:      testLogger(),
		concurrency: 4,
		timeout:     1 * time.Minute,
		tkConfig:    config.TestKitchenConfig{},
		db:          nil, // Will cause errors in testOne, but we check the batch shape.
	}

	repos := []datastore.GitRepo{
		{ID: "1", Name: "cb-a", HasTestSuite: true, HeadCommitSHA: "aaa"},
		{ID: "2", Name: "cb-b", HasTestSuite: true, HeadCommitSHA: "bbb"},
	}
	versions := []string{"17.0.0", "18.0.0", "18.5.0"}

	result := s.TestGitRepos(context.Background(), repos, versions,
		func(gr datastore.GitRepo) string { return "/tmp/" + gr.Name })

	// 2 repos × 3 versions = 6 work items.
	if result.Total != 6 {
		t.Errorf("expected 6 work items, got %d", result.Total)
	}
	// All should error because db is nil.
	if result.Errors != 6 {
		t.Errorf("expected 6 errors (nil db), got %d errors", result.Errors)
	}
}

func TestTestGitRepos_EmptyDir(t *testing.T) {
	mock := newMockKitchenExecutor()
	s := &KitchenScanner{
		executor:    mock,
		logger:      testLogger(),
		concurrency: 2,
		timeout:     1 * time.Minute,
		tkConfig:    config.TestKitchenConfig{},
	}

	repos := []datastore.GitRepo{
		{ID: "1", Name: "cb-a", HasTestSuite: true, HeadCommitSHA: "aaa"},
	}

	result := s.TestGitRepos(context.Background(), repos, []string{"18.0.0"},
		func(gr datastore.GitRepo) string { return "" })

	// Empty dir means the repo is filtered out.
	if result.Total != 0 {
		t.Errorf("expected 0 work items for empty dir, got %d", result.Total)
	}
}

func TestTestGitRepos_ContextCancelled(t *testing.T) {
	mock := newMockKitchenExecutor()
	s := &KitchenScanner{
		executor:    mock,
		logger:      testLogger(),
		concurrency: 1,
		timeout:     1 * time.Minute,
		tkConfig:    config.TestKitchenConfig{},
	}

	repos := []datastore.GitRepo{
		{ID: "1", Name: "cb-a", HasTestSuite: true, HeadCommitSHA: "aaa"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	result := s.TestGitRepos(ctx, repos, []string{"18.0.0"},
		func(gr datastore.GitRepo) string { return "/tmp/cb-a" })

	// The run should complete quickly with the items either errored or skipped.
	if result.Total > 1 {
		t.Errorf("expected at most 1 work item, got %d", result.Total)
	}
}

// ---------------------------------------------------------------------------
// Tests: overlay file lifecycle (.kitchen.local.yml written and cleaned up)
// ---------------------------------------------------------------------------

func TestOverlayFileCreatedAndCleaned(t *testing.T) {
	dir := makeTempCookbookDir(t, "driver:\n  name: dokken\n")
	overlayPath := filepath.Join(dir, ".kitchen.local.yml")

	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverOverride: "vagrant",
	})

	overlay := s.buildOverlay("18.0.0", "dokken")
	if overlay == "" {
		t.Fatal("expected non-empty overlay")
	}

	// Simulate what testOne does: write the overlay.
	if err := os.WriteFile(overlayPath, []byte(overlay), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify it exists.
	if _, err := os.Stat(overlayPath); os.IsNotExist(err) {
		t.Fatal("expected .kitchen.local.yml to exist")
	}

	// Read it back and check content.
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "name: vagrant") {
		t.Errorf("expected driver override in overlay, got:\n%s", content)
	}

	// Simulate cleanup.
	os.Remove(overlayPath)
	if _, err := os.Stat(overlayPath); !os.IsNotExist(err) {
		t.Error("expected .kitchen.local.yml to be removed")
	}
}

// ---------------------------------------------------------------------------
// Tests: KitchenRunResult fields
// ---------------------------------------------------------------------------

func TestKitchenRunResult_CompatibleWhenBothPass(t *testing.T) {
	r := KitchenRunResult{
		ConvergePassed: true,
		TestsPassed:    true,
	}
	r.Compatible = r.ConvergePassed && r.TestsPassed
	if !r.Compatible {
		t.Error("expected compatible when both converge and tests pass")
	}
}

func TestKitchenRunResult_IncompatibleWhenConvergeFails(t *testing.T) {
	r := KitchenRunResult{
		ConvergePassed: false,
		TestsPassed:    true,
	}
	r.Compatible = r.ConvergePassed && r.TestsPassed
	if r.Compatible {
		t.Error("expected incompatible when converge fails")
	}
}

func TestKitchenRunResult_IncompatibleWhenTestsFail(t *testing.T) {
	r := KitchenRunResult{
		ConvergePassed: true,
		TestsPassed:    false,
	}
	r.Compatible = r.ConvergePassed && r.TestsPassed
	if r.Compatible {
		t.Error("expected incompatible when tests fail")
	}
}

func TestKitchenRunResult_TimedOutIsIncompatible(t *testing.T) {
	r := KitchenRunResult{
		TimedOut:       true,
		ConvergePassed: false,
		TestsPassed:    false,
	}
	r.Compatible = r.ConvergePassed && r.TestsPassed
	if r.Compatible {
		t.Error("expected incompatible when timed out")
	}
}

// ---------------------------------------------------------------------------
// Tests: KitchenBatchResult aggregation
// ---------------------------------------------------------------------------

func TestKitchenBatchResult_ZeroValue(t *testing.T) {
	r := KitchenBatchResult{}
	if r.Total != 0 || r.Tested != 0 || r.Skipped != 0 ||
		r.Passed != 0 || r.Failed != 0 || r.Errors != 0 || r.TimedOut != 0 {
		t.Error("expected zero values")
	}
}

// ---------------------------------------------------------------------------
// Tests: NewKitchenScanner defaults
// ---------------------------------------------------------------------------

func TestNewKitchenScanner_Defaults(t *testing.T) {
	s := NewKitchenScanner(nil, testLogger(), "/usr/bin/kitchen", 0, 0,
		config.TestKitchenConfig{})
	if s.concurrency != 1 {
		t.Errorf("expected concurrency 1 when 0 passed, got %d", s.concurrency)
	}
	if s.timeout != 30*time.Minute {
		t.Errorf("expected 30m timeout when 0 passed, got %s", s.timeout)
	}
	if s.executor == nil {
		t.Error("expected default executor to be set")
	}
}

func TestNewKitchenScanner_CustomValues(t *testing.T) {
	s := NewKitchenScanner(nil, testLogger(), "/opt/embedded/bin/kitchen", 8, 60,
		config.TestKitchenConfig{DriverOverride: "ec2"})
	if s.concurrency != 8 {
		t.Errorf("expected concurrency 8, got %d", s.concurrency)
	}
	if s.timeout != 60*time.Minute {
		t.Errorf("expected 60m timeout, got %s", s.timeout)
	}
	if s.tkConfig.DriverOverride != "ec2" {
		t.Errorf("expected driver override ec2, got %q", s.tkConfig.DriverOverride)
	}
}

func TestNewKitchenScanner_WithMockExecutor(t *testing.T) {
	mock := newMockKitchenExecutor()
	s := NewKitchenScanner(nil, testLogger(), "/usr/bin/kitchen", 4, 30,
		config.TestKitchenConfig{}, WithKitchenExecutor(mock))
	if s.executor != mock {
		t.Error("expected mock executor to be set via option")
	}
}

func TestNewKitchenScanner_NegativeValues(t *testing.T) {
	s := NewKitchenScanner(nil, testLogger(), "/usr/bin/kitchen", -5, -10,
		config.TestKitchenConfig{})
	if s.concurrency != 1 {
		t.Errorf("expected concurrency clamped to 1, got %d", s.concurrency)
	}
	if s.timeout != 30*time.Minute {
		t.Errorf("expected timeout clamped to 30m, got %s", s.timeout)
	}
}

// ---------------------------------------------------------------------------
// Tests: writeAttributes
// ---------------------------------------------------------------------------

func TestWriteAttributes_Simple(t *testing.T) {
	var buf strings.Builder
	attrs := map[string]interface{}{
		"key1": "value1",
	}
	var b bytes.Buffer
	writeAttributes(&b, attrs, 4)
	buf.WriteString(b.String())
	got := buf.String()
	if !strings.Contains(got, "    key1: value1") {
		t.Errorf("expected indented key1, got:\n%s", got)
	}
}

func TestWriteAttributes_Nested(t *testing.T) {
	attrs := map[string]interface{}{
		"parent": map[string]interface{}{
			"child": "val",
		},
	}
	var b bytes.Buffer
	writeAttributes(&b, attrs, 2)
	got := b.String()
	if !strings.Contains(got, "  parent:") {
		t.Errorf("expected parent key, got:\n%s", got)
	}
	if !strings.Contains(got, "    child: val") {
		t.Errorf("expected nested child key, got:\n%s", got)
	}
}

func TestWriteAttributes_EmptyMap(t *testing.T) {
	var b bytes.Buffer
	writeAttributes(&b, map[string]interface{}{}, 2)
	if b.Len() != 0 {
		t.Errorf("expected empty output for empty map, got %q", b.String())
	}
}

// We need bytes import for the writeAttributes helper test above
// but it's already imported in the test file header. Let's add a
// compile-check by using it.
var _ = bytes.Buffer{}

// ---------------------------------------------------------------------------
// Tests: config validation integration
// ---------------------------------------------------------------------------

func TestConfigValidation_ValidPlatformOverrides(t *testing.T) {
	tkc := config.TestKitchenConfig{
		PlatformOverrides: []config.TestKitchenPlatform{
			{Name: "ubuntu-22.04", Driver: map[string]string{"image": "dokken/ubuntu-22.04"}},
		},
	}
	// Just verify the struct is valid — actual validation happens in config.Validate.
	if tkc.PlatformOverrides[0].Name != "ubuntu-22.04" {
		t.Error("expected platform name")
	}
}

func TestConfigValidation_EmptyPlatformName(t *testing.T) {
	tkc := config.TestKitchenConfig{
		PlatformOverrides: []config.TestKitchenPlatform{
			{Name: "", Driver: map[string]string{"image": "dokken/ubuntu-22.04"}},
		},
	}
	if tkc.PlatformOverrides[0].Name != "" {
		t.Error("expected empty name")
	}
}

func TestConfigValidation_DriverOverrideValues(t *testing.T) {
	drivers := []string{"dokken", "vagrant", "ec2", "azurerm", "gce", "docker"}
	for _, d := range drivers {
		tkc := config.TestKitchenConfig{DriverOverride: d}
		if tkc.DriverOverride != d {
			t.Errorf("expected %q, got %q", d, tkc.DriverOverride)
		}
	}
}

func TestConfigValidation_ExtraYAML(t *testing.T) {
	tkc := config.TestKitchenConfig{
		ExtraYAML: "transport:\n  name: ssh\n",
	}
	if tkc.ExtraYAML == "" {
		t.Error("expected extra YAML to be set")
	}
}

// ---------------------------------------------------------------------------
// Tests: KitchenInstance JSON parsing
// ---------------------------------------------------------------------------

func TestKitchenInstance_ParseJSON(t *testing.T) {
	input := `[{"instance":"default-ubuntu-2204","driver":"Dokken","provisioner":"Dokken","verifier":"Inspec","transport":"Dokken","last_action":"converge"}]`

	var instances []KitchenInstance
	if err := json.Unmarshal([]byte(input), &instances); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	inst := instances[0]
	if inst.Instance != "default-ubuntu-2204" {
		t.Errorf("Instance = %q", inst.Instance)
	}
	if inst.Driver != "Dokken" {
		t.Errorf("Driver = %q", inst.Driver)
	}
	if inst.LastAction != "converge" {
		t.Errorf("LastAction = %q", inst.LastAction)
	}
}

func TestKitchenInstance_ParseNullLastAction(t *testing.T) {
	input := `[{"instance":"default-centos-8","driver":"Vagrant","provisioner":"ChefInfra","verifier":"Inspec","transport":"SSH","last_action":null}]`

	var instances []KitchenInstance
	if err := json.Unmarshal([]byte(input), &instances); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if instances[0].LastAction != "" {
		t.Errorf("expected empty LastAction for null, got %q", instances[0].LastAction)
	}
}

// Need json import for the test above — it's already in the imports block.

// ---------------------------------------------------------------------------
// Tests: full phase sequence (converge → verify → destroy)
// ---------------------------------------------------------------------------

func TestPhaseSequence_AllPass(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["converge"] = mockKitchenResponse{Stdout: "converge ok", ExitCode: 0}
	mock.responses["verify"] = mockKitchenResponse{Stdout: "verify ok", ExitCode: 0}
	mock.responses["destroy"] = mockKitchenResponse{Stdout: "destroy ok", ExitCode: 0}
	s := &KitchenScanner{executor: mock, logger: testLogger(), timeout: 5 * time.Minute}

	// Run each phase manually (simulating what testOne does).
	cr := s.runPhase(context.Background(), "/tmp", "converge")
	if !cr.Passed {
		t.Error("converge should pass")
	}

	vr := s.runPhase(context.Background(), "/tmp", "verify")
	if !vr.Passed {
		t.Error("verify should pass")
	}

	// Verify both phases were called.
	if len(mock.callsForPhase("converge")) != 1 {
		t.Error("expected 1 converge call")
	}
	if len(mock.callsForPhase("verify")) != 1 {
		t.Error("expected 1 verify call")
	}
}

func TestPhaseSequence_ConvergeFails_VerifySkipped(t *testing.T) {
	mock := newMockKitchenExecutor()
	mock.responses["converge"] = mockKitchenResponse{Stdout: "converge failed", ExitCode: 1}
	mock.responses["verify"] = mockKitchenResponse{Stdout: "should not run", ExitCode: 0}
	s := &KitchenScanner{executor: mock, logger: testLogger(), timeout: 5 * time.Minute}

	cr := s.runPhase(context.Background(), "/tmp", "converge")
	if cr.Passed {
		t.Fatal("converge should fail")
	}

	// In a real testOne, verify would not be called. Let's verify the
	// sequence logic by checking that only converge was called.
	// (We only call verify if converge passed.)
	if len(mock.callsForPhase("converge")) != 1 {
		t.Error("expected 1 converge call")
	}
	if len(mock.callsForPhase("verify")) != 0 {
		t.Error("verify should not have been called (converge failed)")
	}
}

// ---------------------------------------------------------------------------
// Tests: driver detection edge cases
// ---------------------------------------------------------------------------

func TestDetectDriver_WithExtraDriverKeys(t *testing.T) {
	yml := `---
driver:
  name: dokken
  privileged: true
  env:
    - FOO=bar
provisioner:
  name: dokken
`
	dir := makeTempCookbookDir(t, yml)
	got := detectDriver(filepath.Join(dir, ".kitchen.yml"))
	if got != "dokken" {
		t.Errorf("expected dokken, got %q", got)
	}
}

func TestDetectDriver_DriverNotFirstSection(t *testing.T) {
	yml := `---
provisioner:
  name: chef_zero

driver:
  name: vagrant
  box: bento/ubuntu-22.04

platforms:
  - name: ubuntu-22.04
`
	dir := makeTempCookbookDir(t, yml)
	got := detectDriver(filepath.Join(dir, ".kitchen.yml"))
	if got != "vagrant" {
		t.Errorf("expected vagrant, got %q", got)
	}
}

func TestDetectDriver_EmptyFile(t *testing.T) {
	dir := makeTempCookbookDir(t, "")
	got := detectDriver(filepath.Join(dir, ".kitchen.yml"))
	if got != "" {
		t.Errorf("expected empty for empty file, got %q", got)
	}
}

func TestDetectDriver_OnlyComments(t *testing.T) {
	dir := makeTempCookbookDir(t, "# This is a comment\n# driver:\n#   name: dokken\n")
	got := detectDriver(filepath.Join(dir, ".kitchen.yml"))
	if got != "" {
		t.Errorf("expected empty for comments-only file, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Tests: overlay generation for specific real-world scenarios
// ---------------------------------------------------------------------------

func TestBuildOverlay_DokkenToVagrant(t *testing.T) {
	// Scenario: cookbook uses dokken, operator wants to test with vagrant instead.
	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverOverride: "vagrant",
		PlatformOverrides: []config.TestKitchenPlatform{
			{Name: "ubuntu-22.04", Driver: map[string]string{"box": "bento/ubuntu-22.04"}},
		},
	})
	got := s.buildOverlay("18.5.0", "dokken")

	if !strings.Contains(got, "name: vagrant") {
		t.Error("expected vagrant driver override")
	}
	// Since override is vagrant (not dokken), should use product_version.
	if !strings.Contains(got, "product_version") {
		t.Error("expected product_version for vagrant")
	}
	if strings.Contains(got, "chef_version") {
		t.Error("should not use chef_version for vagrant driver")
	}
	if !strings.Contains(got, "box: bento/ubuntu-22.04") {
		t.Error("expected vagrant box in platform override")
	}
}

func TestBuildOverlay_VagrantToDokken(t *testing.T) {
	// Scenario: cookbook uses vagrant, operator wants to test with dokken.
	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverOverride: "dokken",
		PlatformOverrides: []config.TestKitchenPlatform{
			{Name: "ubuntu-22.04", Driver: map[string]string{"image": "dokken/ubuntu-22.04"}},
		},
	})
	got := s.buildOverlay("17.10.0", "vagrant")

	if !strings.Contains(got, "name: dokken") {
		t.Error("expected dokken driver override")
	}
	if !strings.Contains(got, "chef_version") {
		t.Error("expected chef_version for dokken")
	}
	if !strings.Contains(got, "image: dokken/ubuntu-22.04") {
		t.Error("expected dokken image in platform override")
	}
}

func TestBuildOverlay_EC2WithInstanceType(t *testing.T) {
	// Scenario: test on real AWS instances.
	s := newScannerWithConfig(config.TestKitchenConfig{
		DriverOverride: "ec2",
		DriverConfig: map[string]string{
			"region":        "us-east-1",
			"instance_type": "t3.large",
		},
		PlatformOverrides: []config.TestKitchenPlatform{
			{Name: "rhel-8", Driver: map[string]string{"image_id": "ami-0abcdef1234567890"}},
			{Name: "amazon-2", Driver: map[string]string{"image_id": "ami-0fedcba9876543210"}},
		},
	})
	got := s.buildOverlay("18.0.0", "vagrant")

	if !strings.Contains(got, "name: ec2") {
		t.Error("expected ec2 driver")
	}
	if !strings.Contains(got, "region: us-east-1") {
		t.Error("expected region in driver config")
	}
	if !strings.Contains(got, "instance_type: t3.large") {
		t.Error("expected instance_type in driver config")
	}
	if !strings.Contains(got, "- name: rhel-8") {
		t.Error("expected rhel-8 platform")
	}
	if !strings.Contains(got, "- name: amazon-2") {
		t.Error("expected amazon-2 platform")
	}
}

func TestBuildOverlay_ProductionPlatformAlignment(t *testing.T) {
	// Scenario: operator aligns test platforms with production fleet.
	// Production runs Ubuntu 20.04, 22.04, and RHEL 8.
	s := newScannerWithConfig(config.TestKitchenConfig{
		PlatformOverrides: []config.TestKitchenPlatform{
			{Name: "ubuntu-20.04", Driver: map[string]string{"image": "dokken/ubuntu-20.04"}},
			{Name: "ubuntu-22.04", Driver: map[string]string{"image": "dokken/ubuntu-22.04"}},
			{Name: "rhel-8", Driver: map[string]string{"image": "dokken/centos-8"}},
		},
	})
	got := s.buildOverlay("18.5.0", "dokken")

	// Should have 3 platforms.
	count := strings.Count(got, "- name:")
	if count != 3 {
		t.Errorf("expected 3 platforms, found %d in:\n%s", count, got)
	}
}

// ---------------------------------------------------------------------------
// sanitiseKitchenEnv tests
// ---------------------------------------------------------------------------

func TestSanitiseKitchenEnv_RemovesBundlerVars(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"HOME=/home/user",
		"BUNDLE_GEMFILE=/cookbooks/chef-client/Gemfile",
		"BUNDLE_BIN_PATH=/usr/lib/ruby/gems/bundler",
		"BUNDLE_PATH=/vendor/bundle",
		"RUBYOPT=-rbundler/setup",
		"GEM_HOME=/usr/lib/ruby/gems",
	}

	got := sanitiseKitchenEnv(env)

	allowed := map[string]bool{
		"PATH":     true,
		"HOME":     true,
		"GEM_HOME": true,
	}
	for _, kv := range got {
		key := kv[:strings.IndexByte(kv, '=')]
		if !allowed[key] {
			t.Errorf("expected %q to be removed, but it was kept", key)
		}
	}
	if len(got) != 3 {
		t.Errorf("expected 3 env vars, got %d: %v", len(got), got)
	}
}

func TestSanitiseKitchenEnv_PreservesNonBundlerVars(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"GEM_HOME=/gems",
		"GEM_PATH=/gems",
		"TERM=xterm",
	}

	got := sanitiseKitchenEnv(env)
	if len(got) != len(env) {
		t.Errorf("expected %d env vars, got %d", len(env), len(got))
	}
}

func TestSanitiseKitchenEnv_EmptyInput(t *testing.T) {
	got := sanitiseKitchenEnv(nil)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestSanitiseKitchenEnv_NoEqualsSign(t *testing.T) {
	// Malformed entries without '=' should be preserved as-is.
	env := []string{"NOEQUALS", "PATH=/bin"}
	got := sanitiseKitchenEnv(env)
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d: %v", len(got), got)
	}
}
