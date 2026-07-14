package webapi

import (
	"strings"
	"testing"
)

func TestValidateSavedFilterSelection_AcceptsViewParams(t *testing.T) {
	cases := []struct {
		name    string
		view    string
		filters map[string][]string
	}{
		{
			name: "nodes multi-role cohort",
			view: "nodes",
			filters: map[string][]string{
				"role": {"base", "windows-base", "iis"},
			},
		},
		{
			name: "nodes mixed params",
			view: "nodes",
			filters: map[string][]string{
				"environment":      {"production"},
				"platform":         {"windows"},
				"tags":             {"pci", "dmz"},
				"readiness_filter": {"ready"},
				"organisation":     {"org-a"},
			},
		},
		{
			name:    "roles",
			view:    "roles",
			filters: map[string][]string{"name": {"web"}, "compatibility_status": {"incompatible"}, "tk_status": {"failed"}},
		},
		{
			name:    "cookbooks",
			view:    "cookbooks",
			filters: map[string][]string{"active": {"true"}, "compatibility": {"incompatible"}, "download_status": {"downloaded"}},
		},
		{
			name:    "git-repos",
			view:    "git-repos",
			filters: map[string][]string{"clone_status": {"cloned"}, "has_test_suite": {"yes"}},
		},
		{
			name:    "empty selection is allowed",
			view:    "nodes",
			filters: map[string][]string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSavedFilterSelection(tc.view, tc.filters); err != nil {
				t.Fatalf("expected selection to be valid, got: %v", err)
			}
		})
	}
}

func TestValidateSavedFilterSelection_RejectsUnknownView(t *testing.T) {
	err := validateSavedFilterSelection("dashboard", map[string][]string{})
	if err == nil {
		t.Fatal("expected an unknown view to be rejected")
	}
	if !strings.Contains(err.Error(), "dashboard") {
		t.Errorf("error should name the offending view, got: %v", err)
	}
}

// The vocabulary is owned by the view: a saved filter records a selection in it
// and can never smuggle in a param the view's parser does not accept.
func TestValidateSavedFilterSelection_RejectsUnknownParam(t *testing.T) {
	err := validateSavedFilterSelection("nodes", map[string][]string{
		"role":         {"base"},
		"cluster_name": {"prod"}, // not in the nodes vocabulary
	})
	if err == nil {
		t.Fatal("expected an unknown param to be rejected")
	}
	if !strings.Contains(err.Error(), "cluster_name") {
		t.Errorf("error should name the offending param, got: %v", err)
	}
}

// A param valid on one view is not valid on another — the vocabulary is per-view.
func TestValidateSavedFilterSelection_RejectsParamFromAnotherView(t *testing.T) {
	err := validateSavedFilterSelection("git-repos", map[string][]string{
		"role": {"base"}, // a nodes param
	})
	if err == nil {
		t.Fatal("expected a foreign view's param to be rejected")
	}
}

// Global lens params are a deliberate operator setting, not part of a named
// cohort — applying a saved filter must never move them, so they cannot be
// stored in one.
func TestValidateSavedFilterSelection_RejectsGlobalLensParams(t *testing.T) {
	for _, param := range []string{"target_chef_version", "stale", "stale_tiers"} {
		t.Run(param, func(t *testing.T) {
			err := validateSavedFilterSelection("nodes", map[string][]string{param: {"18.4.12"}})
			if err == nil {
				t.Fatalf("expected the global lens param %q to be rejected", param)
			}
			if !strings.Contains(err.Error(), param) {
				t.Errorf("error should name the offending param, got: %v", err)
			}
		})
	}
}

// Sort and pagination are "how I'm reading them", not "which nodes".
func TestValidateSavedFilterSelection_RejectsViewState(t *testing.T) {
	for _, param := range []string{"sort", "order", "page", "per_page"} {
		t.Run(param, func(t *testing.T) {
			err := validateSavedFilterSelection("nodes", map[string][]string{param: {"name"}})
			if err == nil {
				t.Fatalf("expected the view-state param %q to be rejected", param)
			}
		})
	}
}

func TestValidateSavedFilterSelection_RejectsValuelessParam(t *testing.T) {
	err := validateSavedFilterSelection("nodes", map[string][]string{"role": {}})
	if err == nil {
		t.Fatal("expected a param with no values to be rejected")
	}
}

func TestSavedFilterViews_MatchTheListViews(t *testing.T) {
	want := []string{"nodes", "roles", "cookbooks", "git-repos"}
	for _, view := range want {
		if _, ok := savedFilterVocabulary[view]; !ok {
			t.Errorf("list view %q has no saved-filter vocabulary", view)
		}
	}
	if len(savedFilterVocabulary) != len(want) {
		t.Errorf("vocabulary covers %d views, want %d", len(savedFilterVocabulary), len(want))
	}
}
