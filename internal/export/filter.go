// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Filters holds the optional filter criteria applied to export operations.
// These mirror the query-parameter filters available on the node list API
// endpoint and extend to cover cookbook remediation exports.
// All string filters use case-insensitive substring (partial) matching.
type Filters struct {
	Organisation      string `json:"organisation,omitempty"`
	NodeName          string `json:"node_name,omitempty"`
	Environment       string `json:"environment,omitempty"`
	Platform          string `json:"platform,omitempty"`
	ChefVersion       string `json:"chef_version,omitempty"`
	PolicyName        string `json:"policy_name,omitempty"`
	PolicyGroup       string `json:"policy_group,omitempty"`
	Role              string `json:"role,omitempty"`
	Stale             string `json:"stale,omitempty"` // "true", "false", or "" (any)
	TargetChefVersion string `json:"target_chef_version,omitempty"`
	ComplexityLabel   string `json:"complexity_label,omitempty"`
}

// IsEmpty returns true when no filter criteria are set.
func (f Filters) IsEmpty() bool {
	return f.Organisation == "" &&
		f.NodeName == "" &&
		f.Environment == "" &&
		f.Platform == "" &&
		f.ChefVersion == "" &&
		f.PolicyName == "" &&
		f.PolicyGroup == "" &&
		f.Role == "" &&
		f.Stale == "" &&
		f.TargetChefVersion == "" &&
		f.ComplexityLabel == ""
}

// MarshalToJSON serialises the Filters struct as a JSON byte slice suitable
// for storing in the export_jobs.filters JSONB column.
func (f Filters) MarshalToJSON() (json.RawMessage, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("export: marshalling filters: %w", err)
	}
	return json.RawMessage(b), nil
}

// ParseFilters deserialises a Filters struct from a JSON byte slice
// (typically the export_jobs.filters JSONB column).
func ParseFilters(raw json.RawMessage) (Filters, error) {
	var f Filters
	if len(raw) == 0 || string(raw) == "null" {
		return f, nil
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("export: parsing filters: %w", err)
	}
	return f, nil
}

// FilterNodes applies the export filter criteria to a slice of node
// snapshots, returning only nodes that match all specified filters.
// String fields are compared using case-insensitive substring (partial)
// matching so that, for example, filtering by environment "prod" will
// match nodes in "Production", "us-prod-east", etc.
// The Stale filter is left as an exact boolean comparison.
func FilterNodes(nodes []datastore.NodeSnapshot, f Filters) []datastore.NodeSnapshot {
	if f.Environment == "" && f.Platform == "" && f.ChefVersion == "" &&
		f.PolicyName == "" && f.PolicyGroup == "" && f.Role == "" && f.Stale == "" && f.NodeName == "" {
		return nodes
	}

	filtered := make([]datastore.NodeSnapshot, 0, len(nodes))
	for _, n := range nodes {
		if f.NodeName != "" && !strings.Contains(strings.ToLower(n.NodeName), strings.ToLower(f.NodeName)) {
			continue
		}
		if f.Environment != "" && !strings.Contains(strings.ToLower(n.ChefEnvironment), strings.ToLower(f.Environment)) {
			continue
		}
		if f.Platform != "" && !strings.Contains(strings.ToLower(n.Platform), strings.ToLower(f.Platform)) {
			continue
		}
		if f.ChefVersion != "" && !strings.Contains(strings.ToLower(n.ChefVersion), strings.ToLower(f.ChefVersion)) {
			continue
		}
		if f.PolicyName != "" && !strings.Contains(strings.ToLower(n.PolicyName), strings.ToLower(f.PolicyName)) {
			continue
		}
		if f.PolicyGroup != "" && !strings.Contains(strings.ToLower(n.PolicyGroup), strings.ToLower(f.PolicyGroup)) {
			continue
		}
		if f.Role != "" && !nodeHasRole(n, f.Role) {
			continue
		}
		if f.Stale == "true" && !n.IsStale {
			continue
		}
		if f.Stale == "false" && n.IsStale {
			continue
		}
		filtered = append(filtered, n)
	}
	return filtered
}

// FilterOrganisations returns only the organisations whose name contains
// the filter string (case-insensitive substring match). If the organisation
// filter is empty, all organisations are returned.
func FilterOrganisations(orgs []datastore.Organisation, orgName string) []datastore.Organisation {
	if orgName == "" {
		return orgs
	}
	filtered := make([]datastore.Organisation, 0, 1)
	for _, org := range orgs {
		if strings.Contains(strings.ToLower(org.Name), strings.ToLower(orgName)) {
			filtered = append(filtered, org)
		}
	}
	return filtered
}

// FilterComplexities returns only the complexity records matching the given
// target Chef version and optional complexity label. If targetVersion is
// empty, all versions are included. If complexityLabel is empty, all labels
// are included.
func FilterComplexities(complexities []datastore.CookbookComplexity, targetVersion, complexityLabel string) []datastore.CookbookComplexity {
	if targetVersion == "" && complexityLabel == "" {
		return complexities
	}
	filtered := make([]datastore.CookbookComplexity, 0, len(complexities))
	for _, cc := range complexities {
		if targetVersion != "" && cc.TargetChefVersion != targetVersion {
			continue
		}
		if complexityLabel != "" && cc.ComplexityLabel != complexityLabel {
			continue
		}
		filtered = append(filtered, cc)
	}
	return filtered
}

// nodeHasRole checks whether a node snapshot's Roles JSON array contains a
// role whose name matches the given filter using case-insensitive substring
// matching. The Roles field is a JSON array of strings, e.g.
// ["base","webserver"]. We unmarshal the array and check each element
// individually for a partial, case-insensitive match.
func nodeHasRole(n datastore.NodeSnapshot, roleName string) bool {
	if len(n.Roles) == 0 {
		return false
	}
	var roles []string
	if err := json.Unmarshal([]byte(n.Roles), &roles); err != nil {
		return false
	}
	lowerFilter := strings.ToLower(roleName)
	for _, r := range roles {
		if strings.Contains(strings.ToLower(r), lowerFilter) {
			return true
		}
	}
	return false
}
