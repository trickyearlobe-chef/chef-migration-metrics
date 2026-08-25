//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/perf"
)

// The journey suite for journeys/service-diagnosis.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is built;
// a red one is not yet. That makes this the todo list for the journey, and a
// todo list made of tests cannot go stale: nobody has to remember to update it,
// because running it recomputes it.
//
// It is deliberately OUTSIDE the gating suite. Most of a journey is unbuilt for
// most of its life, so a red here is the normal state and must never block a
// build — a red that stops a release gets deleted, and then the list is gone.
//
// Two rules:
//
//   - Assert the real thing, so building the feature turns the test green with
//     no edit. A test that says "not implemented" has to be rewritten by the
//     person it was meant to help.
//   - Name the journey line it comes from, in the journey's words, so the
//     reason outlives whoever wrote it.
//
// This is not where regressions go. Something that used to work and now fails
// is a broken build, not a todo — parking it here hides it among the honest
// gaps, which are indistinguishable from it once they are in the same list.

// ---------------------------------------------------------------------------
// Helpers unique to this journey
// ---------------------------------------------------------------------------

// diagnosisBundle asks for a bundle and returns its files. The handler is
// called directly, as the ordinary suite does, because the sections are what is
// under test here and not the way in.
func diagnosisBundle(t *testing.T, r *Router, query string) map[string][]byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle"+query, nil)
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("asking for a diagnostic bundle answered %d: %s", w.Code, w.Body.String())
	}
	return readZipFiles(t, w.Body.Bytes())
}

// diagnosisSectionsContaining names every file in the bundle whose text holds
// needle. The promise the journey makes is about the whole file the operator
// sends, not about one section of it.
func diagnosisSectionsContaining(files map[string][]byte, needle string) []string {
	var found []string
	for name, data := range files {
		if strings.Contains(string(data), needle) {
			found = append(found, name)
		}
	}
	return found
}

// diagnosisRouterWithConfig is newDiagnosticRouter with a caller-supplied
// configuration, so a canary secret can be planted in it.
func diagnosisRouterWithConfig(cfg *config.Config, store *mockStore) *Router {
	hub := NewEventHub()
	go hub.Run()

	sessions := auth.NewSessionManager(mockSessionStore{}, 8*time.Hour)
	mw := auth.NewMiddleware(sessions)
	localAuth := auth.NewLocalAuthenticator(mockLocalAuthStore{}, 5)

	return NewRouter(store, cfg, hub,
		WithAuth(localAuth, sessions, mw, nil),
		WithPerformance(perf.NewRecorder(300*time.Second, 200, 1000)),
	)
}

// ---------------------------------------------------------------------------
// What I need
// ---------------------------------------------------------------------------

// "A single action that collects everything diagnostic into one file the
// operator can download and send. Not a list of instructions — one button,
// because every extra step is a step that gets done wrong or not at all."
//
// One request, one file, and the file arrives as a download rather than as
// something the operator has to save out of a browser window by hand.
func TestJourney_OneActionCollectsEverythingIntoOneFile(t *testing.T) {
	r := newDiagnosticRouter(defaultDiagnosticStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle", nil)
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("there is no single action that produces a bundle (answered %d)", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("the bundle is not one file: content type is %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("the bundle is not offered as a download (disposition %q), so sending it "+
			"becomes a second instruction", cd)
	}

	files := readZipFiles(t, w.Body.Bytes())
	if len(files) < 2 {
		t.Errorf("one request produced %d file(s) — everything diagnostic is not in one place", len(files))
	}
}

// "Enough in it to actually diagnose without a second round trip. Versions,
// configuration with the secrets removed, recent errors, the shape of the data
// — how many of what — and the health of the host."
//
// Each clause checked separately, so a bundle that loses one of them says which.
func TestJourney_EnoughInItToDiagnoseWithoutASecondRoundTrip(t *testing.T) {
	var errorLogFilters []datastore.LogEntryFilter
	store := defaultDiagnosticStore()
	inner := store.ListLogEntriesFn
	store.ListLogEntriesFn = func(ctx context.Context, f datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
		if f.MinSeverity != "" {
			errorLogFilters = append(errorLogFilters, f)
		}
		return inner(ctx, f)
	}

	files := diagnosisBundle(t, newDiagnosticRouter(store), "")

	// Versions.
	info := map[string]any{}
	if data, ok := files["bundle_info.json"]; !ok {
		t.Error("the bundle says nothing about versions")
	} else if err := json.Unmarshal(data, &info); err != nil {
		t.Errorf("the version section cannot be read: %v", err)
	} else {
		if _, ok := info["app_version"]; !ok {
			t.Error("the bundle does not say which version of the software produced it")
		}
		if v, _ := info["go_version"].(string); v == "" {
			t.Error("the bundle does not say what it was built with")
		}
	}

	// Configuration.
	if _, ok := files["config_summary.json"]; !ok {
		t.Error("the bundle does not carry the configuration, so how the service was set up " +
			"needs a second round trip")
	}

	// Recent errors — and errors specifically, not just the whole log.
	if _, ok := files["logs_errors.json"]; !ok {
		t.Error("the bundle does not carry recent errors")
	}
	if len(errorLogFilters) == 0 {
		t.Error("the bundle asks for logs but never asks for the errors on their own, so the " +
			"recent errors have to be found by hand")
	}

	// The shape of the data — how many of what.
	inv := map[string]any{}
	if data, ok := files["inventory_stats.json"]; !ok {
		t.Error("the bundle does not say how many of what there is")
	} else if err := json.Unmarshal(data, &inv); err != nil {
		t.Errorf("the inventory section cannot be read: %v", err)
	} else if _, ok := inv["nodes_by_org"]; !ok {
		t.Error("the inventory section does not carry the counts the shape of the estate is read from")
	}

	// The health of the host, as measurements rather than placeholders.
	health := map[string]any{}
	if data, ok := files["system_health.json"]; !ok {
		t.Error("the bundle does not carry the health of the host")
	} else if err := json.Unmarshal(data, &health); err != nil {
		t.Errorf("the host health section cannot be read: %v", err)
	} else if cpus, _ := health["cpu_count"].(float64); cpus <= 0 {
		t.Errorf("host health reports %v CPUs, so the figures are placeholders rather than "+
			"measurements of the box being diagnosed", health["cpu_count"])
	}
}

// "To read the logs from the interface, filtered, because most questions are
// answered by the last few errors and a screenshot of those is often the whole
// diagnosis."
//
// Also: "Logs are readable through the interface, not only on the host."
// Requiring host access to read a log means the diagnosis cannot happen.
func TestJourney_LogsAreReadableThroughTheInterfaceFiltered(t *testing.T) {
	var got datastore.LogEntryFilter
	store := &mockStore{
		CountLogEntriesFn: func(_ context.Context, _ datastore.LogEntryFilter) (int, error) {
			return 1, nil
		},
		ListLogEntriesFn: func(_ context.Context, f datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
			got = f
			return []datastore.LogEntry{{ID: 1, Severity: "ERROR", Message: "boom"}}, nil
		},
	}
	w := httptest.NewRecorder()
	r := newTestRouterWithMockAndConfig(store, testConfig())
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?min_severity=ERROR&scope=collection", nil)
	r.ServeHTTP(w, withAdminSession(req))

	if w.Code == http.StatusNotFound {
		t.Fatal("logs cannot be read through the interface at all, so a diagnosis needs access " +
			"to the host")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("reading logs through the interface answered %d: %s", w.Code, w.Body.String())
	}
	if got.MinSeverity != "ERROR" {
		t.Errorf("asking for errors only was not honoured (severity filter %q), so the last few "+
			"errors have to be found by scrolling", got.MinSeverity)
	}
	if got.Scope != "collection" {
		t.Errorf("narrowing the logs to one part of the service was not honoured (scope %q)", got.Scope)
	}
}

// "To see where the service is spending its time when the complaint is that it
// is slow, since 'slow' at estate scale and 'slow' in a lab are different
// problems."
func TestJourney_CanSeeWhereTheServiceIsSpendingItsTime(t *testing.T) {
	r := diagnosisRouterWithConfig(testConfig(), defaultDiagnosticStore())
	for _, path := range []string{
		"/api/v1/admin/performance",
		"/api/v1/admin/performance/db",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, path, nil)))
		if w.Code == http.StatusNotFound {
			t.Errorf("there is no way to see where time goes at %s, so 'it is slow' can only be "+
				"answered by guessing", path)
		}
	}
}

// ---------------------------------------------------------------------------
// Safe to send
// ---------------------------------------------------------------------------

// "For that file to be safe to send. It leaves their organisation and it may
// pass through channels neither of us controls, so it must not carry their
// server names, their organisation names or their credentials."
//
// The organisation-names half. Swept across the whole bundle rather than one
// section, because the file is sent whole.
func TestJourney_TheBundleCarriesNoOrganisationNames(t *testing.T) {
	const realOrg = "example-corp-payments"
	store := defaultDiagnosticStore()
	store.ListOrganisationsFn = func(_ context.Context) ([]datastore.Organisation, error) {
		return []datastore.Organisation{{Name: realOrg}}, nil
	}
	store.ListCollectionRunsFilteredFn = func(_ context.Context, _ datastore.CollectionRunFilter) ([]datastore.CollectionRunWithOrg, error) {
		return []datastore.CollectionRunWithOrg{{
			OrganisationName: realOrg,
			Run:              datastore.CollectionRun{OrganisationName: realOrg, Status: "completed"},
		}}, nil
	}
	store.InventoryStatsFn = func(_ context.Context, _ bool) (datastore.InventoryStatsResult, error) {
		return datastore.InventoryStatsResult{NodesByOrg: map[string]int{realOrg: 5}}, nil
	}
	store.ListLogEntriesFn = func(_ context.Context, _ datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
		return []datastore.LogEntry{{
			ID: 1, Severity: "ERROR", Scope: "collection",
			Message: "collection failed", Organisation: realOrg,
		}}, nil
	}
	r := newDiagnosticRouter(store)

	// The fixture has to be able to find the name, or absence proves nothing.
	withNames := diagnosisBundle(t, r, "?include_identifiers=true")
	if len(diagnosisSectionsContaining(withNames, realOrg)) == 0 {
		t.Fatalf("the fixture proves nothing: the organisation name never reaches the bundle " +
			"even when it is asked for")
	}

	files := diagnosisBundle(t, r, "")
	if leaked := diagnosisSectionsContaining(files, realOrg); len(leaked) > 0 {
		t.Errorf("the bundle carries the real organisation name in %v — it leaves the "+
			"organisation and cannot be recalled", leaked)
	}
}

// "...so it must not carry their server names, their organisation names or
// their credentials."
//
// The credentials half, and the addresses that come with them. The database URL
// carries a password and the address of a machine inside the estate; neither is
// any part of what is wrong with the software.
func TestJourney_TheBundleCarriesNoCredentials(t *testing.T) {
	const password = "canary-database-password"
	const dbHost = "db-server-01.internal.example-corp"

	cfg := testConfig()
	cfg.Datastore.URL = "postgres://cmm:" + password + "@" + dbHost + ":5432/cmm"
	cfg.Organisations = []config.Organisation{{
		Name:          "org-a",
		ChefServerURL: "https://chef-server-01.internal.example-corp/organizations/org-a",
		ClientKeyPath: "/etc/cmm/keys/canary-client.pem",
	}}

	r := diagnosisRouterWithConfig(cfg, defaultDiagnosticStore())
	files := diagnosisBundle(t, r, "")

	for _, secret := range []string{password, dbHost, "canary-client.pem"} {
		if leaked := diagnosisSectionsContaining(files, secret); len(leaked) > 0 {
			t.Errorf("the bundle carries a credential or the address it opens (%q) in %v", secret, leaked)
		}
	}
	// Asking for identifiers is a request for names, never for secrets.
	withNames := diagnosisBundle(t, r, "?include_identifiers=true")
	if leaked := diagnosisSectionsContaining(withNames, password); len(leaked) > 0 {
		t.Errorf("asking for identifying information also hands over a password, in %v", leaked)
	}
}

// "...it must not carry their server names..."
//
// The journey names this itself as the gap that matters: "Machine names,
// addresses, repository addresses and user names are not covered, and the
// promise the journey makes is about all of them."
//
// The baseline is asserted first — with identifiers asked for, the machine name
// is in the bundle — so a pass here cannot come from the name being unfindable.
func TestJourney_TheBundleCarriesNoServerNames(t *testing.T) {
	host, err := os.Hostname()
	if err != nil || len(host) < 4 {
		t.Skipf("no machine name to look for on this host (%q, %v), so nothing can be proved here", host, err)
	}

	r := newDiagnosticRouter(defaultDiagnosticStore())

	withNames := diagnosisBundle(t, r, "?include_identifiers=true")
	if len(diagnosisSectionsContaining(withNames, host)) == 0 {
		t.Skipf("the fixture proves nothing: the machine name %q is nowhere in the bundle even "+
			"when identifiers are asked for, so its absence by default says nothing", host)
	}

	files := diagnosisBundle(t, r, "")
	if leaked := diagnosisSectionsContaining(files, host); len(leaked) > 0 {
		t.Errorf("with nothing asked for, the bundle carries the name of the machine it ran on "+
			"in %v — a name from inside the estate that leaves it and cannot be recalled", leaked)
	}
}

// "The bundle is anonymised by default and identifying information is opt-in.
// The safe thing has to be the default, because the operator sending it is not
// the person who will be blamed if it carries something it should not. Somebody
// who genuinely needs real names can ask for them deliberately."
//
// Both directions. Anonymous-by-default is only half of it: if asking cannot
// produce the real names either, the opt-in does not exist and the default is
// not a choice.
func TestJourney_AnonymisedByDefaultAndIdentifiersAreOptIn(t *testing.T) {
	const realOrg = "example-corp-payments"
	store := defaultDiagnosticStore()
	store.ListOrganisationsFn = func(_ context.Context) ([]datastore.Organisation, error) {
		return []datastore.Organisation{{Name: realOrg}}, nil
	}
	r := newDiagnosticRouter(store)

	orgs := string(diagnosisBundle(t, r, "")["organisations.json"])
	if strings.Contains(orgs, realOrg) {
		t.Error("a bundle asked for with nothing set carries real names, so the operator has to " +
			"know to turn them off")
	}

	asked := string(diagnosisBundle(t, r, "?include_identifiers=true")["organisations.json"])
	if !strings.Contains(asked, realOrg) {
		t.Error("real names cannot be asked for deliberately, so somebody who needs them has no " +
			"way to get them")
	}
}

// "The bundle requires authentication, so it is not a way for anybody to
// extract the estate's shape."
func TestJourney_TheBundleIsNotAWayToExtractTheEstatesShape(t *testing.T) {
	// Nobody signed in.
	r := diagnosisRouterWithConfig(testConfig(), defaultDiagnosticStore())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle", nil))
	if w.Code == http.StatusOK {
		t.Error("anybody who can reach the service can download the shape of the estate")
	}

	// Authentication not configured at all: refuse rather than serve openly.
	noAuth := newDiagnosticRouterNoAuth(defaultDiagnosticStore())
	w2 := httptest.NewRecorder()
	noAuth.handleDiagnosticBundle(w2, httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle", nil))
	if w2.Code == http.StatusOK {
		t.Error("with no authentication configured the bundle is served to anyone who asks")
	}
}

// ---------------------------------------------------------------------------
// Still useful when something is broken
// ---------------------------------------------------------------------------

// "A missing part of the bundle does not fail the bundle. If one source of
// information is unavailable — that is often the very fault being diagnosed —
// the rest is still collected. A diagnostic tool that refuses to produce
// anything when something is broken is useless precisely when it is needed."
func TestJourney_AMissingPartDoesNotFailTheBundle(t *testing.T) {
	store := defaultDiagnosticStore()
	store.ListAppliedMigrationsFn = func(_ context.Context) ([]datastore.AppliedMigration, error) {
		return nil, context.DeadlineExceeded
	}
	store.InventoryStatsFn = func(_ context.Context, _ bool) (datastore.InventoryStatsResult, error) {
		return datastore.InventoryStatsResult{}, context.DeadlineExceeded
	}

	files := diagnosisBundle(t, newDiagnosticRouter(store), "")

	for _, name := range []string{"config_summary.json", "system_health.json", "logs_errors.json"} {
		if _, ok := files[name]; !ok {
			t.Errorf("one broken source cost the bundle %s as well", name)
		}
	}
	// And it has to say what it could not collect, or the gap reads as "nothing
	// wrong here".
	data, ok := files["errors.json"]
	if !ok {
		t.Fatal("the bundle does not say which parts are missing, so a gap looks like an answer")
	}
	var errs map[string]string
	if err := json.Unmarshal(data, &errs); err != nil {
		t.Fatalf("what the bundle could not collect cannot be read: %v", err)
	}
	for _, source := range []string{"migrations", "inventory_stats"} {
		if _, ok := errs[source]; !ok {
			t.Errorf("the bundle does not record that %s was unavailable; got %v", source, errs)
		}
	}
}

// "The estate summary inside it is pinned including the empty case, so a new
// installation produces a valid bundle rather than an error."
func TestJourney_ANewInstallationProducesAValidBundle(t *testing.T) {
	files := diagnosisBundle(t, newDiagnosticRouter(&mockStore{}), "")

	if len(files) == 0 {
		t.Fatal("an installation with nothing collected yet produces an empty bundle")
	}
	for name, data := range files {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var any any
		if err := json.Unmarshal(data, &any); err != nil {
			t.Errorf("%s is not readable on a new installation: %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// What the journey says nothing can prove
// ---------------------------------------------------------------------------

// "Nothing proves the bundle is free of identifying information. ... Anybody
// adding a new section to the bundle can put identifying data in it and every
// test here will still pass."
//
// "The load-bearing assumption: that anonymisation is applied where the bundle
// is assembled rather than by each section. If each section is responsible for
// cleaning itself, then the guarantee is only as good as the newest section."
func TestJourney_AnonymisationIsAppliedWhereTheBundleIsAssembled(t *testing.T) {
	t.Skip("Not assertable from outside. Each section anonymises itself today, so a section " +
		"added tomorrow is clean only if its author remembers; the tests above name the " +
		"identifiers they know about and cannot name one nobody has written yet. Proving it " +
		"needs the redaction to sit on the single path every section is written through, " +
		"which is a change to the code and not to this test.")
}

// "Nothing proves no secret reaches a log. Stated in credentials that never
// leave the box in the clear and repeated here because the log path is where it
// would happen."
func TestJourney_NoSecretReachesALog(t *testing.T) {
	t.Skip("Belongs to the credentials journey and is unproven there too. Recorded here so " +
		"the line is not silently unaccounted for: what reaches a log is decided at every " +
		"call site that writes one, and no test can enumerate them.")
}

// "Assume what we log is widely readable. This deployment ships its logs to a
// shared system that many people in the organisation can read."
func TestJourney_WhatIsLoggedIsTreatedAsWidelyReadable(t *testing.T) {
	t.Skip("A rule for whoever writes the next log line, not a behaviour of the code. Nothing " +
		"can decide whether a message is safe for a wide audience; the closest thing to a " +
		"check is the secret test above, which is also unproven.")
}

// "Nothing proves the bundle is sufficient. Whether it actually answers a real
// support question without a second round trip is only established by using it
// in anger, and it is the thing the whole journey is for."
func TestJourney_TheBundleIsSufficient(t *testing.T) {
	t.Skip("Only answered by a real support question. The test above pins what is in the " +
		"bundle; whether that is enough is a judgement made after using it, and stays open " +
		"until somebody diagnoses a fault from one without asking for more.")
}

// "...log retention is pinned so that reading logs from the interface has
// something to read and does not grow without bound."
func TestJourney_LogsDoNotGrowWithoutBound(t *testing.T) {
	t.Skip("Held in the datastore, where retention lives: " +
		"internal/datastore/datastore_test.go#TestPurgeLogEntriesOlderThanDays_InvalidRetention " +
		"and internal/datastore/log_entries_partition_functional_test.go" +
		"#TestLogEntryPartitions_PurgeDropsOnlyFullyExpiredDays. Recorded here so the line is " +
		"not silently unaccounted for — there is nothing on this side to assert.")
}
