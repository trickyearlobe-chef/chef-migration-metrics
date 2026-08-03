// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tkstatus

import (
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Why a Test Kitchen run failed.
//
// A failed run used to be recorded as nothing more than `passed = false`, so a
// cookbook that will not converge and a lab that could not build a VM to
// converge on were the same fact. Readiness treats any Test Kitchen failure as
// incompatible, overriding a CookStyle pass, so lab failures blocked real
// nodes.
//
// Test Kitchen names the phase that failed in its own output ("Failed to
// complete #create action"), and the phases map onto the distinction that
// matters: converge and verify exercise the cookbook, create and destroy are
// the lab building and tearing down a machine. Only the first pair is evidence
// about a cookbook. That marker is used rather than driver-specific DHCP or
// authentication text, which differs per driver and changes between versions.
// ---------------------------------------------------------------------------

// convergePatterns matches lines showing that Chef actually began converging.
var convergePatterns = regexp.MustCompile(
	`(?i)(Converging \d+ resource|` +
		`\* \S+\[.*\] action |` +
		`Recipe: |` +
		`Starting Chef (Infra )?Client|` +
		`resolving cookbooks)`)

// HasConvergeActivity reports whether the combined output shows Chef began
// converging. It is what separates a machine that never booted from a run that
// reached the cookbook.
func HasConvergeActivity(output string) bool {
	return convergePatterns.MatchString(output)
}

// Failure kinds, as stored in git_kitchen_results.failure_kind.
const (
	// FailureNone is the empty kind: the run did not fail.
	FailureNone = ""
	// FailureConverge — Chef ran and the cookbook did not converge.
	FailureConverge = "converge_failed"
	// FailureVerify — the cookbook converged and its own tests failed.
	FailureVerify = "verify_failed"
	// FailureCreate — the lab never produced a usable machine.
	FailureCreate = "create_failed"
	// FailureDestroy — teardown failed. It leaks a VM and needs attention,
	// but it is not a statement about the cookbook.
	FailureDestroy = "destroy_failed"
	// FailureNetworkTimeout — timed out with no sign Chef ever started;
	// usually a DHCP lease that never arrived.
	FailureNetworkTimeout = "network_timeout"
	// FailureTimeout — timed out after Chef started. Still no verdict.
	FailureTimeout = "timeout"
	// FailureNoConverge — the run ended before Chef started, without naming a
	// phase (a tooling or configuration error).
	FailureNoConverge = "no_converge"
	// FailureUnknown — it failed and we cannot say why. Counts against the
	// cookbook: never silently unblock what we cannot explain.
	FailureUnknown = "unknown"
)

// Test Kitchen's own phase markers.
const (
	markerCreate   = "#create action"
	markerConverge = "#converge action"
	markerVerify   = "#verify action"
	markerDestroy  = "#destroy action"
)

// ClassifyFailure returns why a run failed, from the record that is stored for
// it. It takes the stored fields rather than the live run state so that the
// same rule can be applied to results captured before the kind was recorded.
//
// Order matters: a phase the cookbook took part in wins over one it did not,
// so a converge failure that also failed to tear down is a converge failure.
func ClassifyFailure(output string, passed *bool, timedOut, networkTimeout bool) string {
	if passed != nil && *passed {
		return FailureNone
	}

	switch {
	case networkTimeout || (timedOut && !HasConvergeActivity(output)):
		return FailureNetworkTimeout
	case timedOut:
		return FailureTimeout
	case strings.Contains(output, markerCreate):
		return FailureCreate
	case strings.Contains(output, markerConverge):
		return FailureConverge
	case strings.Contains(output, markerVerify):
		return FailureVerify
	case strings.Contains(output, markerDestroy):
		return FailureDestroy
	case !HasConvergeActivity(output):
		return FailureNoConverge
	case passed == nil:
		// Chef started, but nothing recorded a verdict or a phase.
		return FailureUnknown
	}
	return FailureUnknown
}

// IsLabFailure reports whether a failure kind describes the lab rather than
// the cookbook. These leave the cookbook untested instead of failing it.
func IsLabFailure(kind string) bool {
	switch kind {
	case FailureCreate, FailureDestroy, FailureNetworkTimeout, FailureTimeout, FailureNoConverge:
		return true
	}
	return false
}

// CountsAsCookbookFailure is the rollup rule: a run counts against a cookbook
// when it reached a verdict of "no" and that verdict was about the cookbook.
//
// Anything unrecognised counts, including a kind this build has never heard of
// — an older binary writing rows after a rollback would leave the kind empty,
// and dropping those failures silently would unblock nodes nobody decided to
// unblock. The SQL rollups mirror this by excluding the lab kinds by name.
func CountsAsCookbookFailure(passed *bool, kind string) bool {
	if passed == nil || *passed {
		return false
	}
	return !IsLabFailure(kind)
}
