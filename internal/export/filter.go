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
type Filters struct {
	Organisation      string `json:"organisation,omitempty"`
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
// This is the shared filtering logic equivalent to the filterNodes()
// function in the webapi package, extracted here so that both the API
// handlers and export generators can reuse it.
func FilterNodes(nodes []datastore.NodeSnapshot, f Filters) []datastore.NodeSnapshot {
	if f.Environment == "" && f.Platform == "" && f.ChefVersion == "" &&
		f.PolicyName == "" && f.PolicyGroup == "" && f.Role == "" && f.Stale == "" {
		return nodes
	}

	filtered := make([]datastore.NodeSnapshot, 0, len(nodes))
	for _, n := range nodes {
		if f.Environment != "" && n.ChefEnvironment != f.Environment {
			continue
		}
		if f.Platform != "" && n.Platform != f.Platform {
			continue
		}
		if f.ChefVersion != "" && n.ChefVersion != f.ChefVersion {
			continue
		}
		if f.PolicyName != "" && n.PolicyName != f.PolicyName {
			continue
		}
		if f.PolicyGroup != "" && n.PolicyGroup != f.PolicyGroup {
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

// FilterOrganisations returns only the organisations whose name matches the
// filter. If the organisation filter is empty, all organisations are returned.
func FilterOrganisations(orgs []datastore.Organisation, orgName string) []datastore.Organisation {
	if orgName == "" {
		return orgs
	}
	filtered := make([]datastore.Organisation, 0, 1)
	for _, org := range orgs {
		if org.Name == orgName {
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

// nodeHasRole checks whether a node snapshot's Roles JSON contains the given
// role name. The Roles field is a JSON array of strings, e.g.
// ["base","webserver"]. We use a quick substring check on the raw JSON —
// the role name will appear as a quoted string element.
func nodeHasRole(n datastore.NodeSnapshot, roleName string) bool {
	if len(n.Roles) == 0 {
		return false
	}
	return strings.Contains(string(n.Roles), fmt.Sprintf("%q", roleName))
}
