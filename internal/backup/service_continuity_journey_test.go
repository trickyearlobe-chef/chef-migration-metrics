//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The journey suite for journeys/service-continuity.md. Run it with
// `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do, so running this recomputes the todo list rather than
// asking anybody to keep one true. Outside the gating suite on purpose: a red
// here is a gap, never a broken build.
//
// The suite lives in this package because taking, verifying, pruning and
// restoring a backup are all here. What the journey is actually about — that
// months of human judgement survive an upgrade — is a property of a populated
// database going through a schema change, which nothing in this package can
// reach. That line is recorded as a skip rather than left out.

// continuityJourneyBackup takes one backup and returns its manifest, which is
// what an administrator has in hand before they change something.
func continuityJourneyBackup(t *testing.T, svc *Service, by string) Manifest {
	t.Helper()
	m, err := svc.Create(context.Background(), by)
	if err != nil {
		t.Fatalf("taking a backup: %v", err)
	}
	svc.RunCreate(context.Background(), &m)
	return m
}

func continuityJourneyService(t *testing.T) *Service {
	t.Helper()
	return newTestService(t, &mockExecutor{
		pgDumpVersion:   "pg_dump (PostgreSQL) 17.2",
		pgServerVersion: "17.2",
	})
}

// "To take one on demand, before I change something."
//
// The whole rollback story reduces to this, so it is asked first: an
// administrator about to change something can get a backup, and it completes.
func TestJourney_ICanTakeABackupBeforeIChangeSomething(t *testing.T) {
	svc := continuityJourneyService(t)
	m := continuityJourneyBackup(t, svc, "admin")

	if m.Status != StatusSucceeded {
		t.Fatalf("a backup taken on demand finished as %q rather than completed, so the "+
			"administrator who took it before an upgrade does not have one", m.Status)
	}
	if m.SHA256 == "" {
		t.Error("a completed backup carries no checksum, so there is no way to tell later " +
			"whether it is intact")
	}
}

// "Every backup records what it is — when, by whom, of what, and a checksum —
// because a directory of files with timestamps is not a set of backups, it is a
// set of files somebody will guess about."
func TestJourney_EveryBackupSaysWhatItIs(t *testing.T) {
	svc := continuityJourneyService(t)
	m := continuityJourneyBackup(t, svc, "the-administrator")

	if m.CreatedAt.IsZero() {
		t.Error("a backup does not record when it was taken")
	}
	if m.InitiatedBy == "" {
		t.Error("a backup does not record who took it, so an unexpected one cannot be " +
			"traced to a person or to the schedule")
	}
	if m.AppVersion == "" || m.SchemaVersion == 0 {
		t.Error("a backup does not record which version of the software and which shape of " +
			"the stored data it holds, so nobody can tell whether it can be restored into " +
			"what is running now")
	}
}

// "To know a backup is intact before I rely on it, and to be refused if it is
// not, rather than finding out during a restore."
//
// "A half-applied restore is worse than no restore: it produces a database that
// looks populated and is wrong, and the thing you would normally reach for to
// fix it is the backup you have just consumed."
func TestJourney_ACorruptBackupIsRefusedRatherThanPartlyApplied(t *testing.T) {
	svc := continuityJourneyService(t)
	m := continuityJourneyBackup(t, svc, "admin")

	// Something got at the file after it was taken — a truncated copy, a bad
	// disk, an interrupted transfer.
	path := filepath.Join(svc.Dir(), m.Filename)
	if err := os.WriteFile(path, []byte("this is not the backup that was taken"), 0600); err != nil {
		t.Fatalf("corrupting the backup file: %v", err)
	}

	if err := svc.VerifyChecksum(m.ID); err == nil {
		t.Error("a backup that no longer matches what was taken verifies as intact, so an " +
			"administrator relies on it and finds out during the restore")
	}

	if err := svc.RunRestore(context.Background(), m.ID); err == nil {
		t.Error("a corrupt backup was restored rather than refused, which leaves a database " +
			"that looks populated and is wrong, with the backup already consumed")
	}
}

// "Backups taken on a schedule without anybody remembering, kept for a sensible
// time, and old ones removed so they do not fill the disk they are protecting
// against."
func TestJourney_OldBackupsAreRemovedSoTheyDoNotFillTheDisk(t *testing.T) {
	svc := continuityJourneyService(t)

	// The fixture keeps three generations. Take more than that.
	for i := 0; i < 5; i++ {
		continuityJourneyBackup(t, svc, "schedule")
	}

	if _, err := svc.Prune(); err != nil {
		t.Fatalf("removing old backups: %v", err)
	}

	remaining, err := svc.List()
	if err != nil {
		t.Fatalf("listing backups: %v", err)
	}
	if len(remaining) > 3 {
		t.Errorf("%d backups are kept where three generations were asked for, so the "+
			"backups fill the disk they exist to protect against", len(remaining))
	}
	if len(remaining) == 0 {
		t.Error("pruning removed every backup, so protecting the disk has been implemented " +
			"by having nothing to restore from")
	}
}

// "A scheduled backup does not start if one is already running. Overlapping
// runs against the same database compete for the resource they are trying to
// protect."
func TestJourney_TwoBackupsDoNotRunAtOnce(t *testing.T) {
	svc := continuityJourneyService(t)

	m, err := svc.Create(context.Background(), "the-first")
	if err != nil {
		t.Fatalf("taking the first backup: %v", err)
	}

	if !svc.IsActive() {
		t.Fatal("a backup that has been started does not report as running, so the " +
			"schedule cannot tell whether to skip and two will compete for the database")
	}

	if _, err := svc.Create(context.Background(), "the-second"); err == nil {
		t.Error("a second backup started while the first was still running, so two dumps " +
			"compete for the database they are trying to protect")
	}

	svc.RunCreate(context.Background(), &m)
	if svc.IsActive() {
		t.Error("a finished backup still reports as running, so every later scheduled run " +
			"is skipped and backups quietly stop")
	}
}

// "To restore, and to know what the restore will replace."
func TestJourney_IKnowWhatARestoreWillReplace(t *testing.T) {
	svc := continuityJourneyService(t)
	m := continuityJourneyBackup(t, svc, "admin")

	got, err := svc.Get(m.ID)
	if err != nil {
		t.Fatalf("reading back what a restore would use: %v", err)
	}
	if got.CreatedAt.IsZero() || got.AppVersion == "" {
		t.Error("what a restore is about to apply cannot be read before applying it, so " +
			"the administrator is replacing the estate's record with something unlabelled")
	}
}

// "Schema changes only ever go forwards. There is no automated way back. This
// is a deliberate choice rather than an omission, and it means a rollback is a
// restore, not a downgrade."
//
// The decision is deliberate, so what is asserted is that nothing has quietly
// grown a way back: a backup records the shape of the data it holds, which is
// what makes "restore from before the change" a thing an administrator can
// actually identify.
func TestJourney_ARollbackIsARestoreNotADowngrade(t *testing.T) {
	svc := continuityJourneyService(t)
	m := continuityJourneyBackup(t, svc, "admin")

	if m.SchemaVersion == 0 {
		t.Fatal("a backup does not say which shape of the stored data it holds, so an " +
			"administrator cannot tell which backup predates the change they are undoing")
	}
}

// "To upgrade in place without losing anything, including when the upgrade
// changes the shape of the stored data."
//
// "Nothing proves an upgrade preserves the record. Schema changes are applied
// going forwards and that path is exercised, but no test takes a populated
// database through an upgrade and checks that the human judgement in it
// survived."
func TestJourney_AnUpgradePreservesTheRecord(t *testing.T) {
	t.Skip("taking a populated database through a schema change and checking that the " +
		"ownership, the judgements and the history survived needs a real database and a " +
		"real upgrade; this is the loss the journey exists to prevent and nothing " +
		"automated defends it")
}

// "Nothing proves a restore produces a working service. What is asserted is
// that a corrupt backup is refused and that the restore mechanism runs. That
// the restored database is complete, that the service starts against it, and
// that the estate looks as it did — none of that is covered."
func TestJourney_ARestoreProducesAWorkingService(t *testing.T) {
	t.Skip("whether a restored database is complete and the service starts against it is " +
		"answered by doing it; a restore has been exercised by hand and that is the only " +
		"evidence")
}

// "Nothing here proves installation at all. Getting the software onto a host,
// and upgrading a running deployment, is done with packages and has no journey
// behind it."
func TestJourney_TheSoftwareCanBeInstalledAndUpgraded(t *testing.T) {
	t.Skip("installation and in-place upgrade are done with packages and have no journey " +
		"behind them; it is the first thing anybody does and nothing here covers it")
}

// "The load-bearing assumption: that a backup is taken before every upgrade.
// Every rollback story above reduces to restoring one, so if that habit is not
// followed the answer to 'how do we go back' is that we do not. Nothing in the
// product enforces it."
func TestJourney_ABackupIsTakenBeforeEveryUpgrade(t *testing.T) {
	t.Skip("nothing in the product requires a backup before an upgrade; it is a habit, " +
		"and every rollback story depends on it being kept")
}
