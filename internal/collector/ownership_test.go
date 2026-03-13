// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// ---------------------------------------------------------------------------
// matchNodeAttribute tests
// ---------------------------------------------------------------------------

func TestMatchNodeAttribute_MatchesExactValue(t *testing.T) {
	attrs := json.RawMessage(`{"automatic.cloud.provider": "aws", "automatic.os": "linux"}`)

	if !matchNodeAttribute(attrs, "automatic.cloud.provider", "aws") {
		t.Error("expected match for automatic.cloud.provider=aws")
	}
}

func TestMatchNodeAttribute_NoMatchDifferentValue(t *testing.T) {
	attrs := json.RawMessage(`{"automatic.cloud.provider": "gcp"}`)

	if matchNodeAttribute(attrs, "automatic.cloud.provider", "aws") {
		t.Error("expected no match for automatic.cloud.provider=aws when value is gcp")
	}
}

func TestMatchNodeAttribute_NoMatchMissingKey(t *testing.T) {
	attrs := json.RawMessage(`{"automatic.os": "linux"}`)

	if matchNodeAttribute(attrs, "automatic.cloud.provider", "aws") {
		t.Error("expected no match when key is absent")
	}
}

func TestMatchNodeAttribute_EmptyJSON(t *testing.T) {
	if matchNodeAttribute(nil, "some.path", "value") {
		t.Error("expected no match for nil custom attributes")
	}
	if matchNodeAttribute(json.RawMessage{}, "some.path", "value") {
		t.Error("expected no match for empty custom attributes")
	}
}

func TestMatchNodeAttribute_InvalidJSON(t *testing.T) {
	attrs := json.RawMessage(`not valid json`)
	if matchNodeAttribute(attrs, "key", "value") {
		t.Error("expected no match for invalid JSON")
	}
}

func TestMatchNodeAttribute_NumericValue(t *testing.T) {
	attrs := json.RawMessage(`{"automatic.cpu.count": 4}`)
	if !matchNodeAttribute(attrs, "automatic.cpu.count", "4") {
		t.Error("expected match for numeric value 4 compared as string")
	}
}

func TestMatchNodeAttribute_BooleanValue(t *testing.T) {
	attrs := json.RawMessage(`{"automatic.cloud.enabled": true}`)
	if !matchNodeAttribute(attrs, "automatic.cloud.enabled", "true") {
		t.Error("expected match for boolean true compared as string")
	}
}

// ---------------------------------------------------------------------------
// evaluateRule dispatch tests
// ---------------------------------------------------------------------------

func TestEvaluateRule_UnknownType(t *testing.T) {
	e := &OwnershipEvaluator{
		cfg: config.OwnershipConfig{Enabled: true},
	}

	rule := config.OwnershipAutoRule{
		Name:  "bad-rule",
		Owner: "team-a",
		Type:  "nonexistent_type",
	}

	_, err := e.evaluateRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for unknown rule type")
	}
	if got := err.Error(); got != `unknown rule type "nonexistent_type"` {
		t.Errorf("unexpected error message: %s", got)
	}
}

// ---------------------------------------------------------------------------
// evaluateNodeNamePatternRule tests (uses a nil db, so we test validation)
// ---------------------------------------------------------------------------

func TestEvaluateNodeNamePatternRule_EmptyPattern(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:  "empty-pattern",
		Owner: "team-a",
		Type:  "node_name_pattern",
	}

	_, err := e.evaluateNodeNamePatternRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestEvaluateNodeNamePatternRule_InvalidRegex(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:    "bad-regex",
		Owner:   "team-a",
		Type:    "node_name_pattern",
		Pattern: "[invalid",
	}

	_, err := e.evaluateNodeNamePatternRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

// ---------------------------------------------------------------------------
// evaluatePolicyMatchRule validation tests
// ---------------------------------------------------------------------------

func TestEvaluatePolicyMatchRule_NoPolicyNameOrPattern(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:  "no-policy",
		Owner: "team-a",
		Type:  "policy_match",
	}

	_, err := e.evaluatePolicyMatchRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error when neither policy_name nor pattern is set")
	}
}

func TestEvaluatePolicyMatchRule_InvalidRegex(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:    "bad-regex",
		Owner:   "team-a",
		Type:    "policy_match",
		Pattern: "(unclosed",
	}

	_, err := e.evaluatePolicyMatchRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

// ---------------------------------------------------------------------------
// evaluateCookbookNamePatternRule validation tests
// ---------------------------------------------------------------------------

func TestEvaluateCookbookNamePatternRule_EmptyPattern(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:  "no-pattern",
		Owner: "team-a",
		Type:  "cookbook_name_pattern",
	}

	_, err := e.evaluateCookbookNamePatternRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestEvaluateCookbookNamePatternRule_InvalidRegex(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:    "bad-regex",
		Owner:   "team-a",
		Type:    "cookbook_name_pattern",
		Pattern: `\p{`,
	}

	_, err := e.evaluateCookbookNamePatternRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

// ---------------------------------------------------------------------------
// evaluateGitRepoURLPatternRule validation tests
// ---------------------------------------------------------------------------

func TestEvaluateGitRepoURLPatternRule_EmptyPattern(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:  "no-pattern",
		Owner: "team-a",
		Type:  "git_repo_url_pattern",
	}

	_, err := e.evaluateGitRepoURLPatternRule(nil, rule)
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestEvaluateGitRepoURLPatternRule_InvalidRegex(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:    "bad-regex",
		Owner:   "team-a",
		Type:    "git_repo_url_pattern",
		Pattern: `(?P<name`,
	}

	_, err := e.evaluateGitRepoURLPatternRule(nil, rule)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

// ---------------------------------------------------------------------------
// evaluateRoleMatchRule validation tests
// ---------------------------------------------------------------------------

func TestEvaluateRoleMatchRule_NoMatchValueOrPattern(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:  "no-match",
		Owner: "team-a",
		Type:  "role_match",
	}

	_, err := e.evaluateRoleMatchRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error when neither match_value nor pattern is set")
	}
}

func TestEvaluateRoleMatchRule_InvalidRegex(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:    "bad-regex",
		Owner:   "team-a",
		Type:    "role_match",
		Pattern: `[z-a]`,
	}

	_, err := e.evaluateRoleMatchRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

// ---------------------------------------------------------------------------
// evaluateNodeAttributeRule validation tests
// ---------------------------------------------------------------------------

func TestEvaluateNodeAttributeRule_MissingAttributePath(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:       "no-path",
		Owner:      "team-a",
		Type:       "node_attribute",
		MatchValue: "aws",
	}

	_, err := e.evaluateNodeAttributeRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for missing attribute_path")
	}
}

func TestEvaluateNodeAttributeRule_MissingMatchValue(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:          "no-value",
		Owner:         "team-a",
		Type:          "node_attribute",
		AttributePath: "automatic.cloud.provider",
	}

	_, err := e.evaluateNodeAttributeRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for missing match_value")
	}
}

// ---------------------------------------------------------------------------
// EvaluateAfterCollection — disabled config
// ---------------------------------------------------------------------------

func TestEvaluateAfterCollection_DisabledConfig(t *testing.T) {
	e := NewOwnershipEvaluator(nil, config.OwnershipConfig{Enabled: false}, nil)

	err := e.EvaluateAfterCollection(nil, "org-id", "org-name")
	if err != nil {
		t.Fatalf("expected nil error when ownership is disabled, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// EvaluateAfterCollection — empty rules
// ---------------------------------------------------------------------------

func TestEvaluateAfterCollection_EmptyRules(t *testing.T) {
	mw := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{mw},
	})
	e := NewOwnershipEvaluator(nil, config.OwnershipConfig{
		Enabled:   true,
		AutoRules: nil,
	}, logger)

	err := e.EvaluateAfterCollection(nil, "org-id", "org-name")
	if err != nil {
		t.Fatalf("expected nil error with empty rules, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// entityMatch key generation
// ---------------------------------------------------------------------------

func TestEntityMatchKey(t *testing.T) {
	m := entityMatch{EntityType: "node", EntityKey: "web-1.example.com"}
	key := m.EntityType + ":" + m.EntityKey
	if key != "node:web-1.example.com" {
		t.Errorf("unexpected key: %s", key)
	}
}

// ---------------------------------------------------------------------------
// extractCMDBOwner tests
// ---------------------------------------------------------------------------

func TestExtractCMDBOwner_MatchesOwnerField(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.node": {"owner": "platform-team", "environment": "prod"}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "platform-team" {
		t.Errorf("expected %q, got %q", "platform-team", got)
	}
}

func TestExtractCMDBOwner_CustomOwnerAttribute(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.node": {"responsible_team": "db-team", "owner": "ignored"}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "responsible_team")
	if got != "db-team" {
		t.Errorf("expected %q, got %q", "db-team", got)
	}
}

func TestExtractCMDBOwner_CookbookObjectType(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.cookbook": {"owner": "cookbook-team"}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.cookbook", "owner")
	if got != "cookbook-team" {
		t.Errorf("expected %q, got %q", "cookbook-team", got)
	}
}

func TestExtractCMDBOwner_ProfileObjectType(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.profile": {"owner": "security-team"}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.profile", "owner")
	if got != "security-team" {
		t.Errorf("expected %q, got %q", "security-team", got)
	}
}

func TestExtractCMDBOwner_RoleObjectType(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.role": {"owner": "infra-team"}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.role", "owner")
	if got != "infra-team" {
		t.Errorf("expected %q, got %q", "infra-team", got)
	}
}

func TestExtractCMDBOwner_MissingCMDBKey(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.cookbook": {"owner": "team-a"}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string for missing CMDB key, got %q", got)
	}
}

func TestExtractCMDBOwner_MissingOwnerAttribute(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.node": {"environment": "prod"}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string for missing owner attribute, got %q", got)
	}
}

func TestExtractCMDBOwner_EmptyOwnerValue(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.node": {"owner": ""}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string for empty owner value, got %q", got)
	}
}

func TestExtractCMDBOwner_NullCustomAttrs(t *testing.T) {
	got := extractCMDBOwner(nil, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string for nil custom attributes, got %q", got)
	}
}

func TestExtractCMDBOwner_EmptyCustomAttrs(t *testing.T) {
	got := extractCMDBOwner(json.RawMessage{}, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string for empty custom attributes, got %q", got)
	}
}

func TestExtractCMDBOwner_InvalidJSON(t *testing.T) {
	got := extractCMDBOwner(json.RawMessage(`not json`), "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string for invalid JSON, got %q", got)
	}
}

func TestExtractCMDBOwner_SubtreeNotAMap(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.node": "not-a-map"}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string when subtree is not a map, got %q", got)
	}
}

func TestExtractCMDBOwner_SubtreeIsNull(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.node": null}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string when subtree is null, got %q", got)
	}
}

func TestExtractCMDBOwner_OwnerIsNotAString(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.node": {"owner": 12345}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string when owner is not a string, got %q", got)
	}
}

func TestExtractCMDBOwner_OwnerIsBool(t *testing.T) {
	attrs := json.RawMessage(`{"itil.cmdb.node": {"owner": true}}`)
	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "" {
		t.Errorf("expected empty string when owner is a boolean, got %q", got)
	}
}

func TestExtractCMDBOwner_MultipleObjectTypes(t *testing.T) {
	attrs := json.RawMessage(`{
		"itil.cmdb.node": {"owner": "node-team"},
		"itil.cmdb.cookbook": {"owner": "cookbook-team"},
		"itil.cmdb.role": {"owner": "role-team"}
	}`)

	tests := []struct {
		cmdbKey string
		want    string
	}{
		{"itil.cmdb.node", "node-team"},
		{"itil.cmdb.cookbook", "cookbook-team"},
		{"itil.cmdb.role", "role-team"},
		{"itil.cmdb.profile", ""},
	}
	for _, tt := range tests {
		got := extractCMDBOwner(attrs, tt.cmdbKey, "owner")
		if got != tt.want {
			t.Errorf("extractCMDBOwner(%s): expected %q, got %q", tt.cmdbKey, tt.want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// evaluateCMDBAttributeRule validation tests
// ---------------------------------------------------------------------------

func TestEvaluateCMDBAttributeRule_MissingObjectType(t *testing.T) {
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name: "no-object-type",
		Type: "cmdb_attribute",
	}

	_, err := e.evaluateCMDBAttributeRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error for missing object_type")
	}
}

func TestEvaluateCMDBAttributeRule_DefaultOwnerAttribute(t *testing.T) {
	// Verify that when OwnerAttribute is empty, it defaults to "owner".
	// We can't do a full integration test without a DB, but we can
	// verify the function doesn't error on an empty OwnerAttribute
	// (beyond the DB call). This is a structural validation test.
	e := &OwnershipEvaluator{}

	rule := config.OwnershipAutoRule{
		Name:       "test-default-attr",
		Type:       "cmdb_attribute",
		ObjectType: "node",
		// OwnerAttribute deliberately left empty — should default to "owner"
	}

	// This will fail at the DB call (nil db), which is expected.
	// The point is it should NOT fail on validation of the OwnerAttribute.
	_, err := e.evaluateCMDBAttributeRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error from nil db, but got nil")
	}
	// The error should be about listing nodes, not about owner_attribute.
	expected := "listing nodes"
	if got := err.Error(); len(got) < len(expected) || got[:len(expected)] != expected {
		// Accept any error that is about the DB call, not about missing config.
		if got == "owner_attribute is required for cmdb_attribute rule" {
			t.Error("OwnerAttribute should default to 'owner' when empty")
		}
	}
}

// ---------------------------------------------------------------------------
// evaluateRule dispatch — cmdb_attribute
// ---------------------------------------------------------------------------

func TestEvaluateRule_CMDBAttribute_ReturnsError(t *testing.T) {
	e := &OwnershipEvaluator{
		cfg: config.OwnershipConfig{Enabled: true},
	}

	rule := config.OwnershipAutoRule{
		Name:       "cmdb-rule",
		Type:       "cmdb_attribute",
		ObjectType: "node",
	}

	_, err := e.evaluateRule(nil, "org-id", rule)
	if err == nil {
		t.Fatal("expected error when cmdb_attribute is dispatched via evaluateRule")
	}
	expected := "cmdb_attribute rules must be evaluated via evaluateCMDBRule"
	if got := err.Error(); len(got) < len(expected) || got[:len(expected)] != expected {
		t.Errorf("unexpected error message: %s", got)
	}
}

// ---------------------------------------------------------------------------
// ownerEntityMatch tests
// ---------------------------------------------------------------------------

func TestOwnerEntityMatch_StructFields(t *testing.T) {
	m := ownerEntityMatch{
		entityMatch: entityMatch{EntityType: "node", EntityKey: "web-01"},
		OwnerName:   "platform-team",
	}

	if m.EntityType != "node" {
		t.Errorf("expected EntityType 'node', got %q", m.EntityType)
	}
	if m.EntityKey != "web-01" {
		t.Errorf("expected EntityKey 'web-01', got %q", m.EntityKey)
	}
	if m.OwnerName != "platform-team" {
		t.Errorf("expected OwnerName 'platform-team', got %q", m.OwnerName)
	}

	// Verify it participates in match-key generation the same way.
	key := m.EntityType + ":" + m.EntityKey
	if key != "node:web-01" {
		t.Errorf("unexpected key: %s", key)
	}
}

// ---------------------------------------------------------------------------
// extractCMDBOwner — all four object types with realistic data
// ---------------------------------------------------------------------------

func TestExtractCMDBOwner_RealisticNodeData(t *testing.T) {
	// Simulate a node with CMDB data set via a cookbook:
	//   node.normal['itil']['cmdb']['node'] = { 'owner' => 'linux-platform', 'ci_id' => 'CI00123' }
	attrs := json.RawMessage(`{
		"itil.cmdb.node": {
			"owner": "linux-platform",
			"ci_id": "CI00123",
			"environment": "production",
			"support_group": "L2-Linux"
		}
	}`)

	got := extractCMDBOwner(attrs, "itil.cmdb.node", "owner")
	if got != "linux-platform" {
		t.Errorf("expected %q, got %q", "linux-platform", got)
	}

	// Also test reading support_group as owner attribute
	got = extractCMDBOwner(attrs, "itil.cmdb.node", "support_group")
	if got != "L2-Linux" {
		t.Errorf("expected %q, got %q", "L2-Linux", got)
	}
}

func TestExtractCMDBOwner_AllObjectTypesPresent(t *testing.T) {
	attrs := json.RawMessage(`{
		"itil.cmdb.node":     {"owner": "node-team",     "ci_id": "CI001"},
		"itil.cmdb.cookbook":  {"owner": "cookbook-team",  "ci_id": "CI002"},
		"itil.cmdb.profile":  {"owner": "security-team", "ci_id": "CI003"},
		"itil.cmdb.role":     {"owner": "infra-team",    "ci_id": "CI004"}
	}`)

	objectTypes := []string{"node", "cookbook", "profile", "role"}
	expected := []string{"node-team", "cookbook-team", "security-team", "infra-team"}

	for i, ot := range objectTypes {
		cmdbKey := fmt.Sprintf("itil.cmdb.%s", ot)
		got := extractCMDBOwner(attrs, cmdbKey, "owner")
		if got != expected[i] {
			t.Errorf("object_type %q: expected %q, got %q", ot, expected[i], got)
		}
	}
}
