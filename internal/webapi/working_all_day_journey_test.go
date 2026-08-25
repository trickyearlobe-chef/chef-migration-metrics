//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// The journey suite for journeys/working-all-day.md. Run it with
// `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do, so running this recomputes the todo list rather than
// asking anybody to keep one true. Outside the gating suite on purpose: a red
// here is a gap, never a broken build.
//
// This journey is unusual: it binds every screen rather than one, and the half
// that matters most to the person happens in front of them rather than on the
// server. The journey names the two tests that have to be written, and says the
// second is the one that stops coverage rotting. Both are here — the second as
// a real enumeration of the source rather than a placeholder, so it fails the
// commit that introduces a bypass rather than waiting for somebody to look.

// frontendSourceDir locates the frontend source from this package. Returns
// false when it is not there, so the suite reports honestly rather than
// passing because it found nothing to check.
func frontendSourceDir(t *testing.T) (string, bool) {
	t.Helper()
	dir := filepath.Join("..", "..", "frontend", "src")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return dir, true
}

// networkCall matches a direct call to the browser's own fetch. A screen that
// makes one is reaching the network without passing the single place an ended
// session would be noticed.
//
// Leading [^.\w] keeps it off .fetch( and refetch(. Comments are stripped
// before matching, because the word turns up in them and a check that counts
// prose is a check nobody believes.
var networkCall = regexp.MustCompile(`(^|[^.\w])fetch\s*\(`)

var (
	lineComment  = regexp.MustCompile(`//[^\n]*`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// withoutComments removes what a reader wrote from what the code does.
func withoutComments(src []byte) []byte {
	return lineComment.ReplaceAll(blockComment.ReplaceAll(src, nil), nil)
}

// "To sign in once in the morning and still be working in the afternoon. A
// lunch break is not a reason to sign in again, and neither is a long meeting."
//
// The server half: how long a session lasts is something somebody sets, so a
// working day can be made to fit rather than being whatever was compiled in.
// "Time away is not the same as time elapsed. A working day with a lunch break
// in it is the normal case, not an edge case."
func TestJourney_HowLongIStaySignedInIsSomethingSomebodyChose(t *testing.T) {
	cfg := &config.Config{}
	cfg.ApplyDefaults()

	if cfg.Auth.SessionExpiry == "" {
		t.Fatal("how long a session lasts is not set to anything, so a working day cannot " +
			"be made to fit and nobody can change it")
	}

	lifetime, err := time.ParseDuration(cfg.Auth.SessionExpiry)
	if err != nil {
		t.Fatalf("how long a session lasts is recorded as %q, which is not a length of "+
			"time: %v", cfg.Auth.SessionExpiry, err)
	}
	if lifetime < 8*time.Hour {
		t.Errorf("a session lasts %s by default, so somebody who signs in after breakfast "+
			"is signed out before the end of the day — a lunch break and a long meeting "+
			"are the normal case, not an edge case", lifetime)
	}
}

// "If my session does end, to be told — plainly, at the moment it matters — and
// put back where I was afterwards. Not dropped on the front page having lost
// the selection I spent ten minutes building."
//
// "An ended session is detected in one place, not in each screen." The server
// side of that is the middleware every request already passes through, and it
// has to answer an ended session distinguishably — a screen cannot tell a
// person their session ended if the answer looks like anything else.
func TestJourney_AnEndedSessionIsAnsweredDistinguishably(t *testing.T) {
	// A router with authentication actually wired. The bare journey router
	// answers without checking, so asking it would report an ended session as
	// working.
	router, _ := credentialScopeFixture(t, "admin", false)
	w := credentialRequest(t, router, "this-session-does-not-exist",
		http.MethodGet, "/api/v1/nodes", "")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("a request carrying a session that is no longer valid was answered %d "+
			"rather than 401, so a screen cannot tell an ended session from any other "+
			"failure and cannot say plainly what happened", w.Code)
	}
}

// "**No part of the application reaches the network except through that
// place.** This is enumerable from the source rather than reviewed by hand, so
// it fails the commit that introduces a bypass."
//
// The journey calls this the one that stops coverage rotting: without it, the
// single-place rule holds only for the screens somebody remembered to check,
// and the newest screen is always the one nobody checked.
//
// Red today. It goes green when the bypasses below are routed through the
// client, and it stays green by failing whoever adds the next one.
func TestJourney_NothingReachesTheNetworkExceptThroughOnePlace(t *testing.T) {
	dir, ok := frontendSourceDir(t)
	if !ok {
		t.Skip("the frontend source is not present beside this package, so the bypasses " +
			"cannot be enumerated from here")
	}

	// The front door itself. Everything else has to go through it.
	frontDoor := filepath.Join("api", "client.ts")

	var bypasses []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".tsx") {
			return nil
		}
		// Tests stand up their own fetch on purpose.
		if strings.Contains(name, ".test.") || strings.Contains(path, "__tests__") {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if rel == frontDoor {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if networkCall.Match(withoutComments(body)) {
			bypasses = append(bypasses, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the frontend source: %v", err)
	}

	if len(bypasses) > 0 {
		sort.Strings(bypasses)
		t.Errorf("%d place(s) reach the network without passing the one place an ended "+
			"session is noticed:\n  %s\nEach is a screen that keeps drawing its last "+
			"answer after the session behind it has gone",
			len(bypasses), strings.Join(bypasses, "\n  "))
	}
}

// "**And it is presented structurally**, so a screen cannot forget to. If
// showing the ended-session state is something each page opts into, that is the
// same staleness problem wearing a different hat."
func TestJourney_TheEndedSessionStateIsPresentedStructurally(t *testing.T) {
	t.Skip("whether an ended session becomes an application-wide condition, rather than " +
		"something each screen opts into, is a property of the frontend's own structure; " +
		"it is unimplemented and this suite cannot assert it from the server side")
}

// "Never to be shown something that is no longer true. A screen that stops
// updating looks exactly like a screen where nothing is happening."
//
// "**Nothing shows stale data as though it were current.** This binds every
// screen, not one of them: a saved selection the server cannot understand must
// not silently return the whole estate, a machine we have not heard from must
// not read as healthy, a screen whose session has ended must not keep
// displaying its last answer."
//
// The first of those three is held on the server and is asked here, because it
// is the one that fails silently in the direction nobody notices: an unfiltered
// answer looks exactly like a legitimate one.
func TestJourney_ASelectionTheServerCannotUnderstandIsRefused(t *testing.T) {
	err := validateSavedFilterSelection("nodes", map[string][]string{
		"a_parameter_nothing_serves": {"value"},
	})
	if err == nil {
		t.Error("a saved selection carrying something the server does not understand was " +
			"accepted, so it silently returns the unfiltered estate and a hundred and " +
			"fifty thousand machines read as 'everything matched'")
	}
}

// "To be able to tell the difference between 'there is nothing to show', 'we
// cannot reach it', and 'this is old'. Those are three different situations and
// I act differently on each."
func TestJourney_NothingToShowIsNotTheSameAsCannotReachIt(t *testing.T) {
	// An address nobody serves must say so, rather than answering as though it
	// exists and had nothing for you.
	w := httptest.NewRecorder()
	agentJourneyRouter().ServeHTTP(w,
		withAdminSession(httptest.NewRequest(http.MethodGet, "/api/v1/nothing-serves-this", nil)))

	if w.Code != http.StatusNotFound {
		t.Errorf("an address nobody serves answered %d, so a screen cannot tell "+
			"'there is nothing here' from 'this does not exist'", w.Code)
	}
}

// "**Whether the tool bypasses its own front door is itself checked.** The
// above only holds while every request really does go through one place. That
// is not a thing to assert once and trust — it is a thing to test, because it
// drifts silently and the drift is invisible until somebody hits it."
//
// Held by TestJourney_NothingReachesTheNetworkExceptThroughOnePlace, which
// enumerates rather than asserts. Kept as its own line because the journey
// states it as a decision, and a decision with no test beside it is the kind
// that gets re-litigated.
func TestJourney_TheCheckOnBypassesIsItselfEnumerated(t *testing.T) {
	if _, ok := frontendSourceDir(t); !ok {
		t.Error("the bypass check cannot reach the frontend source, so it would pass by " +
			"finding nothing rather than by there being nothing to find")
	}
}

// "**Nothing proves the lunch break.** That a session survives a normal working
// day with normal gaps in it is a property of a real day, and nobody has sat in
// front of it for eight hours."
func TestJourney_ASessionSurvivesANormalWorkingDay(t *testing.T) {
	t.Skip("that a session survives a working day with a lunch break and a long meeting " +
		"in it is a property of a real day; nobody has sat in front of it for eight hours")
}

// "**The load-bearing assumption:** that a person can be returned to what they
// were doing after signing in again."
func TestJourney_IAmPutBackWhereIWas(t *testing.T) {
	t.Skip("returning a person to what they were looking at after they sign in again is " +
		"unimplemented; if what they were looking at cannot be described well enough to " +
		"go back to, the honest promise shrinks to telling them clearly and starting again")
}
