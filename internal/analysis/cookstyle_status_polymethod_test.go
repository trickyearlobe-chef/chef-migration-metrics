// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "testing"

// DeriveCookstyleStatus must resolve each offence with its MESSAGE so a
// poly-method cop's deprecation-only variant does not falsely block. See
// journeys/scan-trust.md.

func polyResolver() *CopClassificationResolver {
	return &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: "19.0"}
}

func TestDeriveCookstyleStatus_PolyCop_DeprecationOnlyIsNeedsReview(t *testing.T) {
	// The only offence is Socket.gethostbyname under Lint/DeprecatedClassMethods —
	// deprecation-only, so the cookbook must NOT be Blocked.
	offenses := []CookstyleOffense{
		{CopName: "Lint/DeprecatedClassMethods", Severity: "warning", Message: "`Socket.gethostbyname` is deprecated in favor of `Addrinfo.getaddrinfo`."},
	}
	if got := DeriveCookstyleStatus(offenses, polyResolver()); got != StatusNeedsReview {
		t.Errorf("status = %q, want needs_review (deprecation-only variant must not block)", got)
	}
}

func TestDeriveCookstyleStatus_PolyCop_RemovedVariantBlocks(t *testing.T) {
	offenses := []CookstyleOffense{
		{CopName: "Lint/DeprecatedClassMethods", Severity: "warning", Message: "`File.exists?` is deprecated in favor of `File.exist?`."},
	}
	if got := DeriveCookstyleStatus(offenses, polyResolver()); got != StatusBlocked {
		t.Errorf("status = %q, want blocked (File.exists? removed → Blocker)", got)
	}
}

func TestDeriveCookstyleStatus_PolyCop_MixedBlocks(t *testing.T) {
	// A removed variant anywhere in the set blocks, even alongside a
	// deprecation-only variant.
	offenses := []CookstyleOffense{
		{CopName: "Lint/DeprecatedClassMethods", Severity: "warning", Message: "`Socket.gethostbyname` is deprecated in favor of `Addrinfo.getaddrinfo`."},
		{CopName: "Lint/DeprecatedClassMethods", Severity: "warning", Message: "`File.exists?` is deprecated in favor of `File.exist?`."},
	}
	if got := DeriveCookstyleStatus(offenses, polyResolver()); got != StatusBlocked {
		t.Errorf("status = %q, want blocked", got)
	}
}
