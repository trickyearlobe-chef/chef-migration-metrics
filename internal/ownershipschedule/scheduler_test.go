// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipschedule

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

type fakeStore struct {
	mu        sync.Mutex
	imports   []datastore.ImportMapping
	listErr   error
	recorded  []recordedRun
	recordErr error
}

type recordedRun struct {
	id             int64
	status, detail string
}

func (f *fakeStore) ListScheduledImports(context.Context) ([]datastore.ImportMapping, error) {
	return f.imports, f.listErr
}

func (f *fakeStore) RecordImportRun(_ context.Context, id int64, status, detail string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = append(f.recorded, recordedRun{id, status, detail})
	return f.recordErr
}

func nightly(id int64, name, cron string) datastore.ImportMapping {
	return datastore.ImportMapping{
		ID: id, Name: name, SourceKind: "database",
		Schedule: cron, ScheduleEnabled: true,
		DBConnection: "cmdb", DBQuery: "SELECT 1",
	}
}

// at builds a UTC time on a fixed day, so the cron arithmetic in these tests
// does not depend on when they are run.
func at(hour, minute int) time.Time {
	return time.Date(2026, 8, 6, hour, minute, 0, 0, time.UTC)
}

func TestRunDue_RunsAnImportWhoseCronHasComeRound(t *testing.T) {
	store := &fakeStore{imports: []datastore.ImportMapping{nightly(1, "cmdb-nightly", "0 2 * * *")}}
	var ran []string
	s := New(store, func(_ context.Context, m datastore.ImportMapping) (string, error) {
		ran = append(ran, m.Name)
		return "2 rows", nil
	}, nil)

	s.RunDue(context.Background(), at(1, 59), at(2, 0))

	if len(ran) != 1 || ran[0] != "cmdb-nightly" {
		t.Fatalf("ran = %v, want the nightly import", ran)
	}
	if len(store.recorded) != 1 {
		t.Fatalf("recorded %d runs, want 1", len(store.recorded))
	}
	if store.recorded[0].status != StatusSucceeded {
		t.Errorf("status = %q, want %q", store.recorded[0].status, StatusSucceeded)
	}
	if store.recorded[0].detail != "2 rows" {
		t.Errorf("detail = %q, want the run summary", store.recorded[0].detail)
	}
}

func TestRunDue_LeavesAnImportAloneUntilItsTime(t *testing.T) {
	store := &fakeStore{imports: []datastore.ImportMapping{nightly(1, "cmdb-nightly", "0 2 * * *")}}
	ran := 0
	s := New(store, func(context.Context, datastore.ImportMapping) (string, error) {
		ran++
		return "", nil
	}, nil)

	s.RunDue(context.Background(), at(9, 0), at(9, 1))

	if ran != 0 {
		t.Errorf("ran %d times at 09:01 for an 02:00 schedule, want 0", ran)
	}
	if len(store.recorded) != 0 {
		t.Errorf("recorded a run that never happened: %+v", store.recorded)
	}
}

// A tick that arrives late — the process was busy, or asleep — must still run
// the import once. Skipping it silently is how a nightly import turns into a
// weekly one without anybody noticing.
func TestRunDue_CatchesUpAfterAMissedTick(t *testing.T) {
	store := &fakeStore{imports: []datastore.ImportMapping{nightly(1, "cmdb-nightly", "0 2 * * *")}}
	ran := 0
	s := New(store, func(context.Context, datastore.ImportMapping) (string, error) {
		ran++
		return "", nil
	}, nil)

	// Last check at 01:30, next at 03:00: 02:00 fell in the gap.
	s.RunDue(context.Background(), at(1, 30), at(3, 0))

	if ran != 1 {
		t.Errorf("ran %d times, want 1 catch-up run", ran)
	}
}

// Catching up runs the import once, not once per missed occurrence. An hourly
// import after a day's outage must not fire 24 times in a row.
func TestRunDue_CatchesUpOnceRatherThanPerMissedOccurrence(t *testing.T) {
	store := &fakeStore{imports: []datastore.ImportMapping{nightly(1, "hourly", "0 * * * *")}}
	ran := 0
	s := New(store, func(context.Context, datastore.ImportMapping) (string, error) {
		ran++
		return "", nil
	}, nil)

	s.RunDue(context.Background(), at(1, 0), at(23, 0))

	if ran != 1 {
		t.Errorf("ran %d times after a 22-hour gap, want 1", ran)
	}
}

// A failing import has to leave a mark. Otherwise "it is scheduled" and "it is
// working" are the same claim, and a broken connection is invisible until
// somebody asks why ownership is stale.
func TestRunDue_RecordsAFailureRatherThanSwallowingIt(t *testing.T) {
	store := &fakeStore{imports: []datastore.ImportMapping{nightly(1, "cmdb-nightly", "0 2 * * *")}}
	s := New(store, func(context.Context, datastore.ImportMapping) (string, error) {
		return "", errors.New("could not read the credential")
	}, nil)

	s.RunDue(context.Background(), at(1, 59), at(2, 0))

	if len(store.recorded) != 1 {
		t.Fatalf("recorded %d runs, want 1", len(store.recorded))
	}
	if store.recorded[0].status != StatusFailed {
		t.Errorf("status = %q, want %q", store.recorded[0].status, StatusFailed)
	}
	if store.recorded[0].detail == "" {
		t.Error("a failed run recorded no reason, so nothing on screen can say why")
	}
}

// An unparseable expression cannot reach the database through the API, but a
// hand-edited row or a future format change could. It must disable that one
// import, not stop every other schedule.
func TestRunDue_AnUnparseableExpressionDoesNotStopTheOthers(t *testing.T) {
	store := &fakeStore{imports: []datastore.ImportMapping{
		nightly(1, "broken", "not a cron"),
		nightly(2, "good", "0 2 * * *"),
	}}
	var ran []string
	s := New(store, func(_ context.Context, m datastore.ImportMapping) (string, error) {
		ran = append(ran, m.Name)
		return "", nil
	}, nil)

	s.RunDue(context.Background(), at(1, 59), at(2, 0))

	if len(ran) != 1 || ran[0] != "good" {
		t.Errorf("ran = %v, want only the import with a valid expression", ran)
	}
	var brokenRecorded bool
	for _, rec := range store.recorded {
		if rec.id == 1 && rec.status == StatusFailed {
			brokenRecorded = true
		}
	}
	if !brokenRecorded {
		t.Error("the unreadable schedule was skipped silently, so nothing says why it never runs")
	}
}

// One import failing must not stop the next one being tried.
func TestRunDue_OneFailingImportDoesNotBlockTheRest(t *testing.T) {
	store := &fakeStore{imports: []datastore.ImportMapping{
		nightly(1, "first", "0 2 * * *"),
		nightly(2, "second", "0 2 * * *"),
	}}
	var ran []string
	s := New(store, func(_ context.Context, m datastore.ImportMapping) (string, error) {
		ran = append(ran, m.Name)
		if m.Name == "first" {
			return "", errors.New("connection refused")
		}
		return "ok", nil
	}, nil)

	s.RunDue(context.Background(), at(1, 59), at(2, 0))

	if len(ran) != 2 {
		t.Errorf("ran = %v, want both imports attempted", ran)
	}
}
