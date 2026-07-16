// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"strings"
	"testing"
)

// polyFake is a message-aware test CopClassifier: a cop's classification depends
// on a substring of the offence message, modelling a poly-method cop.
type polyFake struct{}

func (polyFake) Classify(cop string) string { return classBlocker } // cop-name path: over-classifies

func (polyFake) ClassifyOffense(cop, message string) string {
	if cop == "Lint/DeprecatedClassMethods" && strings.Contains(message, "Socket.gethostbyname") {
		return classReview
	}
	return classBlocker
}

// classifyOffensesForComplexity must resolve each offence with its MESSAGE, so a
// poly-method cop's deprecation-only variant scores as Review, not Blocker.
func TestClassifyOffensesForComplexity_MessageAware(t *testing.T) {
	offencesJSON := []byte(`[
		{"cop_name":"Lint/DeprecatedClassMethods","severity":"warning","message":"Socket.gethostbyname is deprecated in favor of Addrinfo.getaddrinfo"},
		{"cop_name":"Lint/DeprecatedClassMethods","severity":"warning","message":"File.exists? is deprecated in favor of File.exist?"}
	]`)
	got := classifyOffensesForComplexity(offencesJSON, polyFake{})
	if len(got) != 2 {
		t.Fatalf("got %d classified offences, want 2", len(got))
	}
	if got[0].Classification != classReview {
		t.Errorf("Socket variant classification = %q, want review (message-aware)", got[0].Classification)
	}
	if got[1].Classification != classBlocker {
		t.Errorf("File.exists? variant classification = %q, want blocker", got[1].Classification)
	}
}
