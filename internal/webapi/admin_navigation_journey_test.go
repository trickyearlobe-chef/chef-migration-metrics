//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The journey suite for journeys/admin-navigation.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do — so running this recomputes the todo list rather than
// asking anybody to keep one true.
//
// Most of it is red, which is correct: the complaint is that the administration
// screens are filed by how the program is built rather than by what somebody
// came to do, and nothing has been moved yet.
//
// These read the interface's own routing table rather than driving a browser.
// What a person sees is decided there, and a suite that needed a running
// browser would be skipped instead of run.

// routingTable is the interface's route list, read as text.
func routingTable(t *testing.T) string {
	t.Helper()
	// Up from internal/webapi to the repository root.
	path := filepath.Join("..", "..", "frontend", "src", "App.tsx")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read the interface's routing table, so none of this can be "+
			"answered: %v", err)
	}
	return string(b)
}

var routePattern = regexp.MustCompile(`path="([^"]+)"`)

// redirectPattern finds a route that sends somebody somewhere else, and the
// place it sends them.
var redirectPattern = regexp.MustCompile(`path="([^"]+)"\s*element=\{<Navigate to="([^"?]+)`)

// "The name I click to be the name of the place I arrive at."
//
// A menu entry named after the analysis tools that lands on a screen named
// after one tool is the case this is written for. Redirects are how that
// happens: the address keeps the old name and the screen has a new one.
func TestJourney_TheNameIClickIsWhereIArrive(t *testing.T) {
	table := routingTable(t)
	assertTableParses(t, table)

	var mismatched []string
	for _, m := range redirectPattern.FindAllStringSubmatch(table, -1) {
		from, to := m[1], m[2]
		// The catch-all is not something anybody clicks: it is where a mistyped
		// address lands, and sending that to the front page is correct.
		if from == "*" {
			continue
		}
		// A redirect to a tab of the same screen is not a rename — it is the
		// same place, reached from an older address.
		if strings.HasPrefix(to, from) {
			continue
		}
		if lastSegment(from) != lastSegment(to) {
			mismatched = append(mismatched, from+" → "+to)
		}
	}

	if len(mismatched) > 0 {
		t.Errorf("%d addresses land somewhere with a different name (%s) — somebody who "+
			"clicked one thing is looking at another, and has to work out for themselves "+
			"whether they arrived anywhere near what they wanted",
			len(mismatched), strings.Join(mismatched, ", "))
	}
}

// "Everything I am allowed to change reachable by clicking. Not by typing an
// address, and not by knowing that one screen keeps another screen's settings
// on a tab."
func TestJourney_EverySettingIsReachableByClicking(t *testing.T) {
	t.Skip("nothing here can tell which screens the menu offers: the menu is built in the " +
		"interface's own components and this suite reads only the routing table. Answering " +
		"it honestly means the menu naming its entries somewhere both it and a test can " +
		"read — which is itself part of the work this journey describes")
}

// "An address that changes has to keep working, because the one thing I do
// write down is a link."
//
// Green, and the reason moving things is safe to attempt: the interface already
// redirects old addresses rather than dropping them. This is the one part of
// the journey that is built, and it is what the rest depends on.
func TestJourney_AnAddressThatMovedStillWorks(t *testing.T) {
	table := routingTable(t)
	assertTableParses(t, table)

	redirects := redirectPattern.FindAllStringSubmatch(table, -1)
	if len(redirects) == 0 {
		t.Error("no address that moved is kept working — a rename now breaks every link " +
			"anybody wrote down, which makes the reorganisation this journey asks for cost " +
			"more than it is worth")
	}
}

// "One home per setting, so I am never deciding which of two screens is the
// real one."
func TestJourney_EachSettingHasOneHome(t *testing.T) {
	t.Skip("no test tells whether two screens present the same setting: it needs a reading " +
		"of what each screen renders, not of where the addresses go. Two screens both " +
		"offering the analysis tool settings is the known case, found by hand")
}

// "Settings grouped by what they change, not by which part of the program reads
// them."
func TestJourney_SettingsAreGroupedByWhatTheyChange(t *testing.T) {
	t.Skip("a judgement, not a fact: whether a setting is filed where somebody would look " +
		"for it cannot be asserted. The journey says the check is a person who has never " +
		"seen the service being asked to find one")
}

// lastSegment is the final part of an address, which is the part a person reads
// as its name.
func lastSegment(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return parts[len(parts)-1]
}

// assertTableParses is the baseline every address check needs first. The
// routing table is read as text, so a change to how it is written could stop
// these patterns matching — and a pattern that matches nothing reports no
// mismatched names and no lost addresses, which reads exactly like success.
func assertTableParses(t *testing.T, table string) {
	t.Helper()
	if got := len(routePattern.FindAllStringSubmatch(table, -1)); got < 20 {
		t.Fatalf("only %d addresses found in the interface's routing table, which is far too "+
			"few — the table is no longer being read, and every check below would report "+
			"success by finding nothing", got)
	}
}
