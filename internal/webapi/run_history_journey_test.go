//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ingest"
)

// The journey suite for journeys/run-history.md. Run it with `make journey`.
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

// runHistoryRouter returns a router with both switches on — the sink that
// receives pushed telemetry and the view that reads it back — so a red here is
// a missing capability rather than a feature that is merely switched off.
func runHistoryRouter(store *mockStore) *Router {
	cfg := testConfig()
	on := true
	cfg.Ingest.Enabled = &on
	cfg.Ingest.ShowRunEvents = &on
	return newTestRouterWithMockAndConfig(store, cfg)
}

// runHistoryGet performs an authenticated read against a router built over store.
func runHistoryGet(store *mockStore, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	runHistoryRouter(store).ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, path, nil)))
	return w
}

// runHistoryPost delivers a body to the telemetry sink.
func runHistoryPost(r *Router, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// runHistoryConverge is one delivered converge, in the shape the Data Feed sends.
const runHistoryConverge = `{"client_run":{"id":"run-1","node_name":"host-01",` +
	`"organization":"org-a","status":"success","end_time":"2026-08-01T10:00:00Z"}}`

// "Machines first, runs second ... A flat list of every converge in the estate
// is noise; the same information grouped by machine is a worklist."
func TestJourney_MachinesFirstRunsSecond(t *testing.T) {
	nodeGrain := false
	store := &mockStore{
		ListConvergeRunNodesFilteredFn: func(context.Context, datastore.ConvergeRunFilter) ([]datastore.ConvergeRunListItem, int, error) {
			nodeGrain = true
			return []datastore.ConvergeRunListItem{{RunID: "run-1", Organisation: "org-a", NodeName: "host-01"}}, 1, nil
		},
	}
	w := runHistoryGet(store, "/api/v1/run-events/nodes")
	if w.Code != http.StatusOK {
		t.Fatalf("asking which machines are unhappy answered %d: %s", w.Code, w.Body.String())
	}
	if !nodeGrain {
		t.Error("the converge history is only readable as a flat list of every run, " +
			"which is noise rather than a worklist")
	}
	if !strings.Contains(w.Body.String(), "host-01") {
		t.Error("the machine-grouped list does not name the machine")
	}
}

// "To filter it the way the problem arrives — this organisation, failures only,
// this version of Chef, this cookbook, since yesterday."
func TestJourney_FiltersTheWayTheProblemArrives(t *testing.T) {
	cases := []struct {
		label string
		query string
		got   func(datastore.ConvergeRunFilter) string
		want  string
	}{
		{"this organisation", "organisation=org-a",
			func(f datastore.ConvergeRunFilter) string { return f.Organisation }, "org-a"},
		{"failures only", "status=failure",
			func(f datastore.ConvergeRunFilter) string { return f.Status }, "failure"},
		{"this version of Chef", "chef_version=19.3.15",
			func(f datastore.ConvergeRunFilter) string { return f.ChefVersion }, "19.3.15"},
		{"this cookbook", "cookbook=base",
			func(f datastore.ConvergeRunFilter) string { return f.Cookbook }, "base"},
		{"since yesterday", "since=2026-08-01T00:00:00Z",
			func(f datastore.ConvergeRunFilter) string { return f.EndTimeFrom.UTC().Format(time.RFC3339) },
			"2026-08-01T00:00:00Z"},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			var seen datastore.ConvergeRunFilter
			store := &mockStore{
				ListConvergeRunNodesFilteredFn: func(_ context.Context, f datastore.ConvergeRunFilter) ([]datastore.ConvergeRunListItem, int, error) {
					seen = f
					return nil, 0, nil
				},
			}
			w := runHistoryGet(store, "/api/v1/run-events/nodes?"+c.query)
			if w.Code != http.StatusOK {
				t.Fatalf("filtering by %s answered %d: %s", c.label, w.Code, w.Body.String())
			}
			if got := c.got(seen); got != c.want {
				t.Errorf("the machine list cannot be narrowed to %s (asked %q, the history was "+
					"asked for %q)", c.label, c.query, got)
			}
		})
	}
}

// "have the machine list answer 'which machines have *any* run matching this',
// rather than only those whose most recent run happens to match."
func TestJourney_AnyRunMatchingNotJustTheLatest(t *testing.T) {
	t.Skip("Needs a real database: the difference between 'any run matched' and 'the " +
		"latest run matched' only shows against stored rows. Held by " +
		"internal/datastore/converge_runs_functional_test.go " +
		"TestFunctional_ConvergeRuns_NodeRollupExistsSemantics, under -tags functional.")
}

// "When something failed, the actual error and where it came from, not a
// summary. If I have to go and log into the machine to find out what happened,
// this has saved me nothing."
func TestJourney_TheActualErrorAndWhereItCameFrom(t *testing.T) {
	store := &mockStore{
		ListConvergeRunsForNodeFn: func(context.Context, string, string, int) ([]datastore.ConvergeRunView, error) {
			return []datastore.ConvergeRunView{{
				RunID:  "run-1",
				Status: "failure",
				Error: json.RawMessage(`{"class":"RuntimeError","message":"boom from failcb",` +
					`"backtrace":["/var/chef/cache/cookbooks/failcb/recipes/default.rb:3"]}`),
				FailedResource: json.RawMessage(`{"cookbook_name":"failcb","recipe_name":"default",` +
					`"name":"boom","type":"ruby_block"}`),
			}}, nil
		},
	}
	w := runHistoryGet(store, "/api/v1/run-events/nodes/org-a/host-01")
	if w.Code != http.StatusOK {
		t.Fatalf("reading one machine's runs answered %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"boom from failcb", // the actual error
		"RuntimeError",
		"default.rb:3", // and where it came from
		"failcb",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the failure comes back without %q, so the only way to find out what "+
				"happened is to log into the machine", want)
		}
	}
}

// "Machines we cannot pull from, treated as first-class ... Those servers must
// not be second-class citizens visible in a corner — if a machine is reporting,
// it is part of the estate."
//
// Asserted where it is most likely to be undone by accident: the organisations
// you can filter by come from the converge history, not from the list of
// organisations we collect. The baseline is set first — the collected list is
// empty, so a pass cannot come from the two agreeing.
func TestJourney_MachinesWeCannotPullFromAreFirstClass(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(context.Context) ([]datastore.Organisation, error) {
			return nil, nil
		},
		ListConvergeRunOrganisationsFn: func(context.Context) ([]string, error) {
			return []string{"dmz-org"}, nil
		},
	}
	w := runHistoryGet(store, "/api/v1/filters/run-organisations")
	if w.Code != http.StatusOK {
		t.Fatalf("listing the organisations you can filter by answered %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "dmz-org") {
		t.Error("an organisation that only pushes cannot be filtered for, so the machines " +
			"we cannot reach are invisible in the view built for them")
	}
}

// "So unrecognisable fields degrade to nothing and the run is kept."
func TestJourney_AMalformedFieldDoesNotCostUsTheRecord(t *testing.T) {
	// The case that motivated it: a count arriving as a word, a cookbook list
	// arriving as a number, an error arriving as a string.
	rec := json.RawMessage(`{"client_run":{"id":"run-1","node_name":"host-01","organization":"org-a",` +
		`"status":"success","end_time":"2026-08-01T10:00:00Z","total_resource_count":"lots",` +
		`"cookbooks":42,"error":"not-an-object"}}`)

	run, err := ingest.Normalise(rec)
	if err != nil {
		t.Fatalf("a run with odd field shapes is refused, losing the parts that were fine: %v", err)
	}
	if run == nil {
		t.Fatal("a run with odd field shapes is dropped, losing the parts that were fine")
	}
	if run.RunID != "run-1" || run.Status != "success" || run.EndTime.IsZero() {
		t.Errorf("the identity, outcome and time of the run did not survive: %+v", run)
	}
	if run.TotalResourceCount != 0 || len(run.Cookbooks) != 0 || run.Error != nil {
		t.Errorf("an odd field was kept as something rather than degraded to nothing: %+v", run)
	}
}

// "The identity and outcome of a run are the only fields worth failing on."
//
// The baseline is asserted first: the same record WITH an identifier is
// accepted, so this cannot pass because records are being refused for some
// other reason.
func TestJourney_OnlyIdentityAndOutcomeAreWorthFailingOn(t *testing.T) {
	identified := json.RawMessage(runHistoryConverge)
	if run, err := ingest.Normalise(identified); err != nil || run == nil {
		t.Fatalf("the fixture proves nothing: an identified run is not kept either (run=%v err=%v)", run, err)
	}

	anonymous := json.RawMessage(`{"client_run":{"node_name":"host-01","organization":"org-a",` +
		`"status":"success","end_time":"2026-08-01T10:00:00Z"}}`)
	run, err := ingest.Normalise(anonymous)
	if err == nil && run != nil && run.RunID == "" {
		t.Error("a run with no identifier is kept anyway, stored under an empty key — the one " +
			"field the whole history is keyed on is not worth failing on")
	}
}

// "But a record we cannot parse at all is stored nowhere, not half-stored.
// Partial rows are worse than absence, because they read as data."
func TestJourney_ARecordWeCannotParseIsStoredNowhere(t *testing.T) {
	stored := 0
	store := &mockStore{
		BulkUpsertConvergeRunsFn: func(_ context.Context, runs []ingest.ConvergeRun) (int, error) {
			stored += len(runs)
			return len(runs), nil
		},
	}
	cfg := testConfig()
	on := true
	cfg.Ingest.Enabled = &on
	r := newTestRouterWithMockAndConfig(store, cfg)

	// A good record followed by a truncated one: the body cannot be parsed, so
	// nothing from it may reach the history.
	runHistoryPost(r, runHistoryConverge+"\n{\"client_run\":{\"id\":\"run-2\"")
	if stored != 0 {
		t.Errorf("%d run(s) from an unparseable delivery were written, so the history holds "+
			"partial rows that read as data", stored)
	}
}

// "Kept for a bounded time, then purged. This is high-volume and it is history,
// not the current state of anything."
func TestJourney_KeptForABoundedTimeThenPurged(t *testing.T) {
	purged := make(chan time.Time, 1)
	stop := ingest.StartRetentionTicker(
		runHistoryPurgeStore{purged: purged},
		func() int { return 2 },
		time.Hour,
		nil,
	)
	defer stop()

	select {
	case cutoff := <-purged:
		if !cutoff.Before(time.Now().UTC()) {
			t.Errorf("the purge cutoff is %s, which is not in the past — nothing would ever "+
				"be reclaimed", cutoff)
		}
	case <-time.After(5 * time.Second):
		t.Error("nothing purges old converge history, so a high-volume record of what " +
			"happened grows without bound")
	}
}

// runHistoryPurgeStore records the cutoff the retention ticker asks to purge to.
type runHistoryPurgeStore struct{ purged chan time.Time }

func (s runHistoryPurgeStore) PurgeConvergeRunPartitions(_ context.Context, olderThan time.Time) (int, error) {
	select {
	case s.purged <- olderThan:
	default:
	}
	return 0, nil
}

// "Off unless somebody turns it on ... Until it is enabled it is not merely
// idle — it is absent."
func TestJourney_OffUnlessSomebodyTurnsItOn(t *testing.T) {
	// Baseline: with the switch on, the endpoint answers. Without this the test
	// would pass just as well against a product that has no sink at all.
	on := true
	enabled := testConfig()
	enabled.Ingest.Enabled = &on
	if w := runHistoryPost(newTestRouterWithMockAndConfig(&mockStore{}, enabled), runHistoryConverge); w.Code == http.StatusNotFound {
		t.Fatalf("the fixture proves nothing: the sink is absent even when switched on")
	}

	// Default config: nobody has turned it on.
	if w := runHistoryPost(newTestRouterWithMockAndConfig(&mockStore{}, testConfig()), runHistoryConverge); w.Code != http.StatusNotFound {
		t.Errorf("an endpoint nobody decided to have is listening (answered %d) — it must be "+
			"absent, not merely idle", w.Code)
	}
}

// "Runs that fail while working out which cookbooks to use — an unresolvable
// dependency, a cookbook that is not there — arrive without the error detail
// that a failed converge carries."
func TestJourney_DepsolveFailuresArriveWithoutTheirError(t *testing.T) {
	t.Skip("A measured gap nothing here can fix: it is a property of what the sending " +
		"system chooses to include, so no test on our side can assert it away.")
}

// "Nothing proves the volume is survivable. A cap exists and drops beyond it,
// and retention purges; whether the settings are right for an estate this size
// is an operational question that only real traffic answers."
func TestJourney_TheVolumeIsSurvivable(t *testing.T) {
	t.Skip("The journey says nothing proves this. Whether the cap and the retention " +
		"window are right for an estate this size is answered by real traffic, not here.")
}

// "The load-bearing assumption: that the identifier the sending system puts on a
// run is stable, and unique enough to recognise the same run arriving twice."
func TestJourney_TheRunIdentifierIsStableAndUnique(t *testing.T) {
	t.Skip("A property of the sending system, not of this code: nothing on our side can " +
		"assert that a run identifier is stable. That duplicate deliveries of the SAME " +
		"identifier collapse is held by internal/datastore/converge_runs_functional_test.go " +
		"TestFunctional_ConvergeRuns_UpsertDedupAndRetention, under -tags functional.")
}
