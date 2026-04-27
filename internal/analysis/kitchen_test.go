// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)
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
// Tests: credential environment variable helpers
// ---------------------------------------------------------------------------

func TestDriverSecretEnvVar(t *testing.T) {
	got := driverSecretEnvVar("vcenter_password")
	if got != "CMM_TK_SECRET_VCENTER_PASSWORD" {
		t.Errorf("driverSecretEnvVar() = %q, want %q", got, "CMM_TK_SECRET_VCENTER_PASSWORD")
	}
}

func TestTransportPasswordEnvVar(t *testing.T) {
	got := transportPasswordEnvVar("ubuntu-22.04")
	if got != "CMM_TK_TRANSPORT_UBUNTU_22_04" {
		t.Errorf("transportPasswordEnvVar() = %q, want %q", got, "CMM_TK_TRANSPORT_UBUNTU_22_04")
	}
}

func TestTransportKeyEnvVar(t *testing.T) {
	got := transportKeyEnvVar("ubuntu-22.04")
	if got != "CMM_TK_KEY_UBUNTU_22_04" {
		t.Errorf("transportKeyEnvVar() = %q, want %q", got, "CMM_TK_KEY_UBUNTU_22_04")
	}
}

func TestNormalizeEnvVarSuffix(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"vcenter_password", "VCENTER_PASSWORD"},
		{"ubuntu-22.04", "UBUNTU_22_04"},
		{"ALREADY-UPPER", "ALREADY_UPPER"},
		{"mixed.Case-name", "MIXED_CASE_NAME"},
		{"simple", "SIMPLE"},
	}
	for _, tc := range cases {
		got := normalizeEnvVarSuffix(tc.input)
		if got != tc.want {
			t.Errorf("normalizeEnvVarSuffix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeEnvVarSuffix_SpecialChars(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ubuntu/22.04", "UBUNTU_22_04"},
		{"Windows Server 2022", "WINDOWS_SERVER_2022"},
		{"my-platform.v2", "MY_PLATFORM_V2"},
		{"simple", "SIMPLE"},
	}
	for _, tc := range tests {
		got := normalizeEnvVarSuffix(tc.in)
		if got != tc.want {
			t.Errorf("normalizeEnvVarSuffix(%q) = %q, want %q", tc.in, got, tc.want)
		}
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
