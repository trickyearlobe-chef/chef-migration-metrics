//go:build unproven

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// Unproven journey properties — run with `make unproven`, NOT part of `make ci`.
//
// A journey says what somebody needs. Where nothing in the code answers that
// need, prose saying so rots: it is written once, nobody re-reads it, and it
// stays on the page long after the gap is closed or long after a second gap
// opens beside it. A failing test does not rot. It fails until somebody makes
// it pass, and then it stops being a claim and becomes a guarantee.
//
// So these are RED ON PURPOSE. Red here means "the journey asks for this and
// nothing provides it yet" — not "the build is broken". That is exactly why
// they sit behind their own build tag and out of the gating suite: a red that
// blocks a release teaches people to delete reds.
//
// Two rules for anything added here:
//
//   - It must assert the real thing, so that implementing the feature turns it
//     green with no edit to the test. A test that says "not implemented" proves
//     nothing and has to be rewritten by the very person it was meant to help.
//   - It must name the journey need it comes from, in the words the journey
//     uses, so the reason survives the person who wrote it.
//
// When one goes green, move it into the ordinary suite and take the matching
// "nothing proves" paragraph out of the journey.

// TestUnproven_ARunSaysHowLongItTook — journeys/ownership-intake.md: "That
// decision turns on having watched it run once, including how long it took — a
// job that takes forty minutes is a different proposition from one that takes
// four."
//
// The summary reports how many rows it read, filtered and wrote. It does not
// report elapsed time, so the fact the scheduling decision turns on is the one
// fact the run does not hand back.
func TestUnproven_ARunSaysHowLongItTook(t *testing.T) {
	encoded, err := json.Marshal(ImportRunSummary{RowCount: 10})
	if err != nil {
		t.Fatalf("marshalling a run summary: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decoding a run summary: %v", err)
	}

	for key := range fields {
		if strings.Contains(strings.ToLower(key), "duration") ||
			strings.Contains(strings.ToLower(key), "elapsed") {
			return // somebody has closed this gap
		}
	}
	t.Fatalf("a completed run does not report how long it took, which is what "+
		"the decision to schedule it turns on; it reports only %v", keysOf(fields))
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
