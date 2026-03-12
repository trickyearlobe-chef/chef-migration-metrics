// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"encoding/json"
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
