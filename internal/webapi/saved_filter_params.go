package webapi

import "fmt"

// savedFilterVocabulary is the set of query params each list view's filter
// parser accepts, minus the params a saved filter must never carry (see
// savedFilterExcluded). A saved filter records a *selection* in a view's
// vocabulary; it never redefines it. Keep each entry in step with the view's
// <x>FilterFromValues function:
//
//	nodes     — nodeSnapshotFilterFromValues (handle_nodes.go)
//	roles     — roleFilterFromValues         (handle_roles.go)
//	cookbooks — cookbookFilterFromValues     (handle_cookbooks.go)
//	git-repos — gitRepoFilterFromValues      (handle_git_repos.go)
//
// "organisation" is parsed off the request by resolveOrganisationFilter rather
// than by the per-view parser, but it is a filter of the view all the same.
var savedFilterVocabulary = map[string]map[string]bool{
	"nodes": setOf(
		"node_name", "environment", "platform", "chef_version", "policy_name",
		"policy_group", "role", "tags", "organisation", "readiness_filter",
		"cookstyle_status", "kitchen_status", "migration_state",
		"target_converge_status", "target_version", "ready_to_activate",
	),
	"roles": setOf(
		"name", "organisation", "compatibility_status", "tk_status",
	),
	"cookbooks": setOf(
		"name", "organisation", "active", "compatibility", "download_status",
		"cookstyle_status", "tk_status",
	),
	"git-repos": setOf(
		"name", "compatibility", "cookstyle_status", "tk_status", "clone_status",
		"has_test_suite",
	),
}

// savedFilterExcluded lists params a view's parser does accept but a saved
// filter may not carry, with the reason to report on rejection. The global lens
// (target version, staleness) is set deliberately by the operator and applying a
// saved filter must not move it; sort and pagination are how the operator reads
// the list, not which records it holds.
var savedFilterExcluded = map[string]string{
	"target_chef_version": "a global filter, not part of a saved cohort",
	"stale":               "a global filter, not part of a saved cohort",
	"stale_tiers":         "a global filter, not part of a saved cohort",
	"sort":                "view state, not a filter",
	"order":               "view state, not a filter",
	"page":                "view state, not a filter",
	"per_page":            "view state, not a filter",
}

// maxSavedFilterNameLen bounds an operator-supplied saved-filter name.
const maxSavedFilterNameLen = 200

// validateSavedFilterSelection checks a saved filter's payload against the
// target view's filter vocabulary. Unknown params are rejected at save time
// rather than silently stored, so a saved filter cannot carry a filter the view
// does not support.
func validateSavedFilterSelection(view string, filters map[string][]string) error {
	vocabulary, ok := savedFilterVocabulary[view]
	if !ok {
		return fmt.Errorf("unknown view %q — saved filters exist for: nodes, roles, cookbooks, git-repos", view)
	}

	for param, values := range filters {
		if reason, excluded := savedFilterExcluded[param]; excluded {
			return fmt.Errorf("filter param %q cannot be saved: it is %s", param, reason)
		}
		if !vocabulary[param] {
			return fmt.Errorf("filter param %q is not accepted by the %s view", param, view)
		}
		if len(values) == 0 {
			return fmt.Errorf("filter param %q has no values", param)
		}
	}

	return nil
}

func setOf(keys ...string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set
}
