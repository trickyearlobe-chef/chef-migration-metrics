//go:build debt

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"testing"
)

// Tech debt that can be held by a test, from plans/todo-tech-debt.md. Run it
// with `make debt`. Red means still outstanding; nothing here gates a release.

// The settings sections do not all answer the same shape.
//
// Reading most of them answers the section itself; reading the analysis tools
// answers it wrapped in a `value`. The frontend's config helpers special-case
// the difference, and the description has to carry two shapes for one idea. The
// fix is to make them agree — which is a breaking change for whoever is reading
// the odd one out, so it is recorded rather than done quietly.
func TestDebt_EverySettingsSectionAnswersTheSameShape(t *testing.T) {
	doc := newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).openAPIDocument()

	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)

	// Resolved, not taken as it comes: a described answer is usually a
	// reference, and reading properties straight off one finds nothing and
	// passes — which is this test going green while the thing it names is
	// still true. It did, once.
	wrapped := resolveSchema(
		responseSchemaIn(doc, "GET", "/api/v1/admin/config/analysis-tools"), schemas)
	if wrapped == nil {
		t.Skip("the analysis-tools section no longer describes what it answers, so this " +
			"cannot tell whether the shapes agree")
	}

	// The baseline: another section, which answers the section itself.
	plain := resolveSchema(
		responseSchemaIn(doc, "GET", "/api/v1/admin/config/collection"), schemas)
	if plain == nil {
		t.Skip("no section to compare against")
	}
	if _, oddOneOut := plain["properties"].(map[string]any)["value"]; oddOneOut {
		t.Fatal("the section being compared against is itself wrapped, so this test is " +
			"comparing two of the same thing and proves nothing")
	}

	props, _ := wrapped["properties"].(map[string]any)
	if _, wrappedInValue := props["value"]; wrappedInValue {
		t.Error("reading the analysis-tools settings answers the section wrapped in a " +
			"`value`, where every other section answers the section itself — so a caller " +
			"reading settings has to know which of the two shapes this one is")
	}
}
