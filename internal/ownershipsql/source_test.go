// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A read that dies part-way says how far it got.
//
// How much arrived before the failure is what says whether a source of that
// size can be read at all, and whether a filter has helped. The count is
// already kept.
func TestAReadThatDiesPartWaySaysHowFarItGot(t *testing.T) {
	// Baseline: with no rows read, there is nothing to report and the failure
	// is left exactly as it was — so what is asserted below is the count being
	// added, not a phrase being appended to everything.
	atStart := &sqlSource{err: errors.New("killed")}
	if strings.Contains(atStart.Err().Error(), "row") {
		t.Fatal("the fixture proves nothing: a failure before any row was read already " +
			"mentions rows")
	}

	partWay := &sqlSource{
		err:     errors.New("killed"),
		number:  412_317,
		started: time.Now().Add(-25 * time.Second),
	}
	got := partWay.Err().Error()
	if !strings.Contains(got, "412317") && !strings.Contains(got, "412,317") {
		t.Errorf("the failure does not say how far the read got, so nobody can tell whether "+
			"it managed five thousand rows or four hundred thousand: %s", got)
	}
	// With the time beside it the two are a throughput, which is what decides
	// whether a source of a given size can be read across a given link.
	if !strings.Contains(got, "25s") {
		t.Errorf("the failure does not say how long the read lasted, so the count is a "+
			"number without a rate: %s", got)
	}
	if !strings.Contains(got, "killed") {
		t.Errorf("saying how far it got lost what actually went wrong: %s", got)
	}
}
