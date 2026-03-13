// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// entityMatch represents a single matched entity produced by evaluating an
// ownership auto-derivation rule.
type entityMatch struct {
	EntityType string
	EntityKey  string
}

// ownerEntityMatch extends entityMatch with a per-match owner name. This is
// used by cmdb_attribute rules where the owner is read from each node's
// attributes rather than being fixed in the rule configuration.
type ownerEntityMatch struct {
	entityMatch
	OwnerName string
}

// OwnershipEvaluator evaluates ownership auto-derivation rules after each
// collection run completes. It examines collected data (nodes, cookbooks,
// roles, git repos) and creates or removes ownership assignments based on
// the configured rules.
type OwnershipEvaluator struct {
	db     *datastore.DB
	cfg    config.OwnershipConfig
	logger *logging.Logger
}

// NewOwnershipEvaluator creates a new OwnershipEvaluator with the given
// dependencies.
func NewOwnershipEvaluator(db *datastore.DB, cfg config.OwnershipConfig, logger *logging.Logger) *OwnershipEvaluator {
	return &OwnershipEvaluator{
		db:     db,
		cfg:    cfg,
		logger: logger,
	}
}

// EvaluateAfterCollection is the main entry point called after each
// collection run completes. It evaluates all configured auto-derivation
// rules for the given organisation, creating and removing ownership
// assignments as appropriate.
func (e *OwnershipEvaluator) EvaluateAfterCollection(ctx context.Context, orgID, orgName string) error {
	if !e.cfg.Enabled {
		return nil
	}

	log := e.logger.WithScope(logging.ScopeOwnership, logging.WithOrganisation(orgName))

	rules := e.cfg.AutoRules
	if len(rules) == 0 {
		log.Debug("no auto-derivation rules configured, skipping")
		return nil
	}

	totalCreated := 0
	totalRemoved := 0
	totalSkipped := 0

	for _, rule := range rules {
		// If the rule specifies an organisation, skip if it doesn't match.
		if rule.Organisation != "" && rule.Organisation != orgName {
			continue
		}

		// cmdb_attribute rules derive the owner from each node's
		// attributes, so they follow a separate code path.
		if rule.Type == "cmdb_attribute" {
			created, removed, skipped, err := e.evaluateCMDBRule(ctx, orgID, rule, log)
			if err != nil {
				log.Warn(fmt.Sprintf("auto-rule %q: CMDB evaluation failed: %v", rule.Name, err))
				continue
			}
			totalCreated += created
			totalRemoved += removed
			totalSkipped += skipped
			continue
		}

		// Look up the owner by name.
		owner, err := e.db.GetOwnerByName(ctx, rule.Owner)
		if err != nil {
			if errors.Is(err, datastore.ErrNotFound) {
				log.Warn(fmt.Sprintf("auto-rule %q: owner %q not found, skipping", rule.Name, rule.Owner))
				continue
			}
			return fmt.Errorf("auto-rule %q: looking up owner %q: %w", rule.Name, rule.Owner, err)
		}

		// Evaluate the rule to produce matches.
		matches, err := e.evaluateRule(ctx, orgID, rule)
		if err != nil {
			log.Warn(fmt.Sprintf("auto-rule %q: evaluation failed: %v", rule.Name, err))
			continue
		}

		// Build a set of current match keys for stale cleanup.
		currentMatchKeys := make(map[string]bool, len(matches))
		for _, m := range matches {
			currentMatchKeys[m.EntityType+":"+m.EntityKey] = true
		}

		// Create assignments for each match.
		created := 0
		for _, m := range matches {
			_, err := e.db.InsertAssignment(ctx, datastore.InsertAssignmentParams{
				OwnerID:          owner.ID,
				EntityType:       m.EntityType,
				EntityKey:        m.EntityKey,
				OrganisationID:   orgID,
				AssignmentSource: "auto_rule",
				AutoRuleName:     rule.Name,
				Confidence:       "inferred",
				Notes:            fmt.Sprintf("Auto-assigned by rule %q", rule.Name),
			})
			if err != nil {
				if errors.Is(err, datastore.ErrAlreadyExists) {
					totalSkipped++
					continue
				}
				log.Warn(fmt.Sprintf("auto-rule %q: failed to create assignment for %s:%s: %v",
					rule.Name, m.EntityType, m.EntityKey, err))
				continue
			}
			created++
		}
		totalCreated += created

		// Clean up stale assignments from this rule that no longer match.
		removed, err := e.db.DeleteStaleAutoRuleAssignments(ctx, rule.Name, orgID, currentMatchKeys)
		if err != nil {
			log.Warn(fmt.Sprintf("auto-rule %q: stale cleanup failed: %v", rule.Name, err))
		} else {
			totalRemoved += removed
		}
	}

	log.Info(fmt.Sprintf("ownership evaluation complete: %d rules, %d created, %d removed, %d unchanged",
		len(rules), totalCreated, totalRemoved, totalSkipped))
	return nil
}

// evaluateRule dispatches to the correct rule evaluator based on rule type.
// Note: cmdb_attribute rules are handled separately in EvaluateAfterCollection
// because they produce per-match owners rather than a single fixed owner.
func (e *OwnershipEvaluator) evaluateRule(ctx context.Context, orgID string, rule config.OwnershipAutoRule) ([]entityMatch, error) {
	switch rule.Type {
	case "node_attribute":
		return e.evaluateNodeAttributeRule(ctx, orgID, rule)
	case "node_name_pattern":
		return e.evaluateNodeNamePatternRule(ctx, orgID, rule)
	case "policy_match":
		return e.evaluatePolicyMatchRule(ctx, orgID, rule)
	case "cookbook_name_pattern":
		return e.evaluateCookbookNamePatternRule(ctx, orgID, rule)
	case "git_repo_url_pattern":
		return e.evaluateGitRepoURLPatternRule(ctx, rule)
	case "role_match":
		return e.evaluateRoleMatchRule(ctx, orgID, rule)
	case "cmdb_attribute":
		// cmdb_attribute rules are handled separately in EvaluateAfterCollection
		// because they produce per-match owners. This case prevents an
		// "unknown rule type" error if evaluateRule is called directly.
		return nil, fmt.Errorf("cmdb_attribute rules must be evaluated via evaluateCMDBRule, not evaluateRule")
	default:
		return nil, fmt.Errorf("unknown rule type %q", rule.Type)
	}
}

// evaluateCMDBRule handles cmdb_attribute rules end-to-end, including owner
// resolution. Unlike other rules that assign all matches to a single
// pre-configured owner, CMDB rules read the owner name from each node's
// itil.cmdb.<object_type>.<owner_attribute> attribute. This means each
// matched node can map to a different owner.
//
// Returns the number of assignments created, removed (stale), and skipped
// (already existed).
func (e *OwnershipEvaluator) evaluateCMDBRule(ctx context.Context, orgID string, rule config.OwnershipAutoRule, log *logging.ScopedLogger) (created, removed, skipped int, err error) {
	matches, evalErr := e.evaluateCMDBAttributeRule(ctx, orgID, rule)
	if evalErr != nil {
		return 0, 0, 0, evalErr
	}

	// Build a set of current match keys for stale cleanup.
	currentMatchKeys := make(map[string]bool, len(matches))
	for _, m := range matches {
		currentMatchKeys[m.EntityType+":"+m.EntityKey] = true
	}

	// Cache owner lookups — many nodes will share the same owner.
	ownerCache := make(map[string]string) // owner name → owner ID

	for _, m := range matches {
		ownerID, cached := ownerCache[m.OwnerName]
		if !cached {
			owner, lookupErr := e.db.GetOwnerByName(ctx, m.OwnerName)
			if lookupErr != nil {
				if errors.Is(lookupErr, datastore.ErrNotFound) {
					log.Warn(fmt.Sprintf("auto-rule %q: CMDB owner %q (from %s:%s) not found, skipping",
						rule.Name, m.OwnerName, m.EntityType, m.EntityKey))
					ownerCache[m.OwnerName] = "" // negative cache
					continue
				}
				return created, 0, skipped, fmt.Errorf("looking up CMDB owner %q: %w", m.OwnerName, lookupErr)
			}
			ownerID = owner.ID
			ownerCache[m.OwnerName] = ownerID
		}

		if ownerID == "" {
			// Negative cache hit — owner was not found previously.
			continue
		}

		_, insertErr := e.db.InsertAssignment(ctx, datastore.InsertAssignmentParams{
			OwnerID:          ownerID,
			EntityType:       m.EntityType,
			EntityKey:        m.EntityKey,
			OrganisationID:   orgID,
			AssignmentSource: "auto_rule",
			AutoRuleName:     rule.Name,
			Confidence:       "inferred",
			Notes:            fmt.Sprintf("Auto-assigned by CMDB rule %q from itil.cmdb.%s.%s", rule.Name, rule.ObjectType, rule.OwnerAttribute),
		})
		if insertErr != nil {
			if errors.Is(insertErr, datastore.ErrAlreadyExists) {
				skipped++
				continue
			}
			log.Warn(fmt.Sprintf("auto-rule %q: failed to create assignment for %s:%s → %s: %v",
				rule.Name, m.EntityType, m.EntityKey, m.OwnerName, insertErr))
			continue
		}
		created++
	}

	// Clean up stale assignments from this rule that no longer match.
	removedCount, cleanupErr := e.db.DeleteStaleAutoRuleAssignments(ctx, rule.Name, orgID, currentMatchKeys)
	if cleanupErr != nil {
		log.Warn(fmt.Sprintf("auto-rule %q: stale cleanup failed: %v", rule.Name, cleanupErr))
	} else {
		removed = removedCount
	}

	return created, removed, skipped, nil
}

// evaluateCMDBAttributeRule matches nodes by reading the owner value from
// their itil.cmdb.<object_type> attributes stored in custom_attributes.
//
// For each node, the evaluator looks up the key "itil.cmdb.<object_type>"
// in the custom_attributes JSON, then reads the <owner_attribute> field
// from the resulting map. If the field is present and non-empty, the node
// is matched with that owner name.
//
// The object_type determines both the attribute path and the entity type
// of the resulting match:
//
//   - node     → entity_type "node",    entity_key = node name
//   - cookbook  → entity_type "node",    entity_key = node name (the node declares cookbook ownership)
//   - profile  → entity_type "node",    entity_key = node name (the node declares profile ownership)
//   - role     → entity_type "node",    entity_key = node name (the node declares role ownership)
func (e *OwnershipEvaluator) evaluateCMDBAttributeRule(ctx context.Context, orgID string, rule config.OwnershipAutoRule) ([]ownerEntityMatch, error) {
	if rule.ObjectType == "" {
		return nil, fmt.Errorf("object_type is required for cmdb_attribute rule")
	}
	ownerAttr := rule.OwnerAttribute
	if ownerAttr == "" {
		ownerAttr = "owner"
	}

	if e.db == nil {
		return nil, fmt.Errorf("listing nodes: database is nil")
	}

	nodes, err := e.db.ListNodeSnapshotsByOrganisation(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	cmdbKey := "itil.cmdb." + rule.ObjectType
	var matches []ownerEntityMatch

	for _, node := range nodes {
		ownerName := extractCMDBOwner(node.CustomAttributes, cmdbKey, ownerAttr)
		if ownerName == "" {
			continue
		}
		matches = append(matches, ownerEntityMatch{
			entityMatch: entityMatch{
				EntityType: "node",
				EntityKey:  node.NodeName,
			},
			OwnerName: ownerName,
		})
	}
	return matches, nil
}

// extractCMDBOwner reads the owner value from a node's custom attributes
// at the path <cmdbKey>.<ownerAttr>. The custom_attributes JSON is a flat
// map where top-level keys are dot-separated paths (e.g.
// "itil.cmdb.node"), and the value at each key is the subtree returned by
// the Chef partial search. The subtree is expected to be a map containing
// the owner attribute field.
//
// Returns the owner name as a string, or "" if not found or not a string.
func extractCMDBOwner(customAttrs json.RawMessage, cmdbKey, ownerAttr string) string {
	if len(customAttrs) == 0 {
		return ""
	}

	var attrs map[string]interface{}
	if err := json.Unmarshal(customAttrs, &attrs); err != nil {
		return ""
	}

	subtree, ok := attrs[cmdbKey]
	if !ok || subtree == nil {
		return ""
	}

	// The subtree should be a map (the itil.cmdb.<object_type> object).
	subtreeMap, ok := subtree.(map[string]interface{})
	if !ok {
		return ""
	}

	val, ok := subtreeMap[ownerAttr]
	if !ok || val == nil {
		return ""
	}

	ownerStr, ok := val.(string)
	if !ok || ownerStr == "" {
		return ""
	}

	return ownerStr
}

// evaluateNodeAttributeRule matches nodes by a value at a configurable
// attribute path in the custom_attributes JSONB field. The custom_attributes
// field stores a flat map keyed by dot-separated paths.
func (e *OwnershipEvaluator) evaluateNodeAttributeRule(ctx context.Context, orgID string, rule config.OwnershipAutoRule) ([]entityMatch, error) {
	if rule.AttributePath == "" {
		return nil, fmt.Errorf("attribute_path is required for node_attribute rule")
	}
	if rule.MatchValue == "" {
		return nil, fmt.Errorf("match_value is required for node_attribute rule")
	}

	nodes, err := e.db.ListNodeSnapshotsByOrganisation(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var matches []entityMatch
	for _, node := range nodes {
		if matchNodeAttribute(node.CustomAttributes, rule.AttributePath, rule.MatchValue) {
			matches = append(matches, entityMatch{
				EntityType: "node",
				EntityKey:  node.NodeName,
			})
		}
	}
	return matches, nil
}

// matchNodeAttribute checks whether the given custom attributes JSON contains
// a value at the specified dot-separated path that equals matchValue. The
// custom_attributes JSONB is stored as a flat map keyed by dot-separated
// attribute paths.
func matchNodeAttribute(customAttrs json.RawMessage, attrPath, matchValue string) bool {
	if len(customAttrs) == 0 {
		return false
	}

	var attrs map[string]interface{}
	if err := json.Unmarshal(customAttrs, &attrs); err != nil {
		return false
	}

	val, ok := attrs[attrPath]
	if !ok {
		return false
	}

	// Convert the value to string for comparison.
	return fmt.Sprintf("%v", val) == matchValue
}

// evaluateNodeNamePatternRule matches nodes by a regex on node name.
func (e *OwnershipEvaluator) evaluateNodeNamePatternRule(ctx context.Context, orgID string, rule config.OwnershipAutoRule) ([]entityMatch, error) {
	if rule.Pattern == "" {
		return nil, fmt.Errorf("pattern is required for node_name_pattern rule")
	}

	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern %q: %w", rule.Pattern, err)
	}

	nodes, err := e.db.ListNodeSnapshotsByOrganisation(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var matches []entityMatch
	for _, node := range nodes {
		if re.MatchString(node.NodeName) {
			matches = append(matches, entityMatch{
				EntityType: "node",
				EntityKey:  node.NodeName,
			})
		}
	}
	return matches, nil
}

// evaluatePolicyMatchRule matches nodes by policy name. If PolicyName is set,
// an exact match is used. If Pattern is set, a regex match is used.
func (e *OwnershipEvaluator) evaluatePolicyMatchRule(ctx context.Context, orgID string, rule config.OwnershipAutoRule) ([]entityMatch, error) {
	if rule.PolicyName == "" && rule.Pattern == "" {
		return nil, fmt.Errorf("either policy_name or pattern is required for policy_match rule")
	}

	// Compile regex before making any DB calls so that invalid patterns
	// are caught early without unnecessary work.
	var re *regexp.Regexp
	if rule.Pattern != "" && rule.PolicyName == "" {
		var compileErr error
		re, compileErr = regexp.Compile(rule.Pattern)
		if compileErr != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", rule.Pattern, compileErr)
		}
	}

	nodes, err := e.db.ListNodeSnapshotsByOrganisation(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var matches []entityMatch
	for _, node := range nodes {
		if node.PolicyName == "" {
			continue
		}
		matched := false
		if rule.PolicyName != "" {
			// Exact match.
			matched = node.PolicyName == rule.PolicyName
		} else if re != nil {
			// Regex match.
			matched = re.MatchString(node.PolicyName)
		}
		if matched {
			matches = append(matches, entityMatch{
				EntityType: "node",
				EntityKey:  node.NodeName,
			})
		}
	}
	return matches, nil
}

// evaluateCookbookNamePatternRule matches cookbooks by a regex on cookbook
// name. It checks both Chef server cookbooks for the given org and git-
// sourced cookbooks.
func (e *OwnershipEvaluator) evaluateCookbookNamePatternRule(ctx context.Context, orgID string, rule config.OwnershipAutoRule) ([]entityMatch, error) {
	if rule.Pattern == "" {
		return nil, fmt.Errorf("pattern is required for cookbook_name_pattern rule")
	}

	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern %q: %w", rule.Pattern, err)
	}

	// Collect cookbook names from both sources, deduplicating.
	seen := make(map[string]bool)
	var matches []entityMatch

	// Server cookbooks for this org.
	serverCookbooks, err := e.db.ListCookbooksByOrganisation(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing server cookbooks: %w", err)
	}
	for _, cb := range serverCookbooks {
		if re.MatchString(cb.Name) && !seen[cb.Name] {
			seen[cb.Name] = true
			matches = append(matches, entityMatch{
				EntityType: "cookbook",
				EntityKey:  cb.Name,
			})
		}
	}

	// Git-sourced cookbooks (cross-org).
	gitCookbooks, err := e.db.ListGitCookbooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing git cookbooks: %w", err)
	}
	for _, cb := range gitCookbooks {
		if re.MatchString(cb.Name) && !seen[cb.Name] {
			seen[cb.Name] = true
			matches = append(matches, entityMatch{
				EntityType: "cookbook",
				EntityKey:  cb.Name,
			})
		}
	}

	return matches, nil
}

// evaluateGitRepoURLPatternRule matches git repos by a regex on the repo
// URL. It queries distinct git_repo_url values from git-sourced cookbooks.
func (e *OwnershipEvaluator) evaluateGitRepoURLPatternRule(ctx context.Context, rule config.OwnershipAutoRule) ([]entityMatch, error) {
	if rule.Pattern == "" {
		return nil, fmt.Errorf("pattern is required for git_repo_url_pattern rule")
	}

	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern %q: %w", rule.Pattern, err)
	}

	gitCookbooks, err := e.db.ListGitCookbooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing git cookbooks: %w", err)
	}

	// Extract distinct git repo URLs.
	seen := make(map[string]bool)
	var matches []entityMatch
	for _, cb := range gitCookbooks {
		if cb.GitRepoURL == "" || seen[cb.GitRepoURL] {
			continue
		}
		seen[cb.GitRepoURL] = true
		if re.MatchString(cb.GitRepoURL) {
			matches = append(matches, entityMatch{
				EntityType: "git_repo",
				EntityKey:  cb.GitRepoURL,
			})
		}
	}
	return matches, nil
}

// evaluateRoleMatchRule matches roles by name. If Pattern is provided, a
// regex match is used. Otherwise, an exact match on the role name is used
// (falling back to MatchValue for backwards compatibility).
func (e *OwnershipEvaluator) evaluateRoleMatchRule(ctx context.Context, orgID string, rule config.OwnershipAutoRule) ([]entityMatch, error) {
	exactName := rule.MatchValue
	if exactName == "" && rule.Pattern == "" {
		return nil, fmt.Errorf("either match_value or pattern is required for role_match rule")
	}

	// Compile regex before making any DB calls so that invalid patterns
	// are caught early without unnecessary work.
	var re *regexp.Regexp
	if rule.Pattern != "" && exactName == "" {
		var compileErr error
		re, compileErr = regexp.Compile(rule.Pattern)
		if compileErr != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", rule.Pattern, compileErr)
		}
	}

	// Get role dependencies for the org and extract distinct role names.
	deps, err := e.db.ListRoleDependenciesByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing role dependencies: %w", err)
	}

	seen := make(map[string]bool)
	var matches []entityMatch
	for _, dep := range deps {
		roleName := dep.RoleName
		if seen[roleName] {
			continue
		}
		seen[roleName] = true

		matched := false
		if exactName != "" {
			matched = roleName == exactName
		} else if re != nil {
			matched = re.MatchString(roleName)
		}
		if matched {
			matches = append(matches, entityMatch{
				EntityType: "role",
				EntityKey:  roleName,
			})
		}
	}
	return matches, nil
}
