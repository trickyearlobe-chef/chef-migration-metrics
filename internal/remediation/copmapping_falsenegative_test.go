// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"strings"
	"testing"
)

// These cover cops that flag a class/helper/method/constant absent or broken on
// CC19.3.15. Each must carry a RemovedIn ≤ 19 so it resolves as a Blocker rather
// than a Review. Validation harness: scripts/cop-validation/README.md.

func TestFalseNegativeSweep_NewBlockersHaveRemovedIn(t *testing.T) {
	// cop → expected RemovedIn (majors that the curation linter cross-checks
	// against the shipped cop description; see cop_curation_lint.go).
	want := map[string]string{
		// Ruby-API removals (Lint dept, break on the target's bundled Ruby 3.4).
		"Lint/BigDecimalNew":       "19.0",
		"Lint/UnifiedInteger":      "19.0",
		"Lint/DeprecatedConstants": "19.0",
		// Chef removed classes/helpers/methods.
		"Chef/Deprecations/UsesChefRESTHelpers":       "13.0",
		"Chef/Deprecations/ChefShellout":              "13.0",
		"Chef/Deprecations/UsesDeprecatedMixins":      "14.0",
		"Chef/Deprecations/ResourceUsesDslNameMethod": "13.0",
		"Chef/Deprecations/NodeSetWithoutLevel":       "11.0",
		"Chef/Deprecations/PartialSearchClassUsage":   "19.0",
		"Chef/Deprecations/PartialSearchHelperUsage":  "19.0",
		"Chef/Deprecations/EpicFail":                  "19.0",
	}
	for cop, removedIn := range want {
		t.Run(cop, func(t *testing.T) {
			m := LookupCop(cop)
			if m == nil {
				t.Fatalf("cop %q has no mapping — false-negative fix missing", cop)
			}
			if m.RemovedIn != removedIn {
				t.Errorf("RemovedIn = %q, want %q (drives verified-removal → Blocker)", m.RemovedIn, removedIn)
			}
		})
	}
}

// DeprecatedConstants is poly: the removed constants (NIL/Random::DEFAULT/etc.)
// resolve to the base Blocker mapping, while Net::HTTPServerException — still
// present on Ruby 3.4 as an alias — is carved out to Review (no RemovedIn).
func TestFalseNegativeSweep_DeprecatedConstantsPoly(t *testing.T) {
	base := LookupCop("Lint/DeprecatedConstants")
	if base == nil || base.RemovedIn != "19.0" {
		t.Fatalf("base DeprecatedConstants mapping = %+v, want RemovedIn 19.0", base)
	}

	// Removed constants → no variant match → base Blocker (RemovedIn 19.0).
	for _, msg := range []string{
		"Use `nil` instead of `NIL`, deprecated since Ruby 2.4.",
		"Use `true` instead of `TRUE`, deprecated since Ruby 2.4.",
		"Do not use `Random::DEFAULT`.",
		"Use `Struct::Group` replacement.",
	} {
		m := LookupCopForOffense("Lint/DeprecatedConstants", msg)
		if m == nil || m.RemovedIn != "19.0" {
			t.Errorf("removed-constant message %q → RemovedIn %q, want 19.0 (base Blocker)", msg, remIn(m))
		}
	}

	// Net::HTTPServerException → Review variant (present on Ruby 3.4, no RemovedIn).
	net := LookupCopForOffense("Lint/DeprecatedConstants",
		"Use `Net::HTTPClientException` instead of `Net::HTTPServerException`.")
	if net == nil {
		t.Fatal("expected a variant mapping for Net::HTTPServerException")
	}
	if net.RemovedIn != "" {
		t.Errorf("Net::HTTPServerException RemovedIn = %q, want empty (present → Review)", net.RemovedIn)
	}
	if !strings.Contains(net.ReplacementPattern, "Net::HTTPClientException") {
		t.Errorf("Net variant guidance %q does not mention Net::HTTPClientException", net.ReplacementPattern)
	}
	if tok := OffenseVariantToken("Lint/DeprecatedConstants",
		"Use `Net::HTTPClientException` instead of `Net::HTTPServerException`."); tok != "Net::HTTPServerException" {
		t.Errorf("group token = %q, want Net::HTTPServerException", tok)
	}
}

func remIn(m *CopMapping) string {
	if m == nil {
		return "<nil>"
	}
	return m.RemovedIn
}
