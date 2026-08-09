# Not losing it, and getting the next version on

**As the administrator running this service, I need a backup I can actually restore from and an
upgrade that does not lose the estate's history, because the accumulated record is the only part
of this that cannot be recreated.**

Everything the service collects it could collect again. What it cannot recreate is the part people
put in by hand — who owns what, which findings were judged harmless, which cookbooks somebody has
personally watched run — and the history that makes a trend a trend. That is months of human
judgement and it exists in exactly one place.

## What I need

Backups taken on a schedule without anybody remembering, kept for a sensible time, and old ones
removed so they do not fill the disk they are protecting against.

To take one on demand, before I change something.

To know a backup is intact before I rely on it, and to be refused if it is not, rather than
finding out during a restore.

To restore, and to know what the restore will replace.

To upgrade in place without losing anything, including when the upgrade changes the shape of the
stored data.

To understand what a rollback actually involves before I need one, rather than discovering the
answer at the worst moment.

## The decisions behind it

**A backup is verified before it is used, and a corrupt one is refused rather than partially
applied.** A half-applied restore is worse than no restore: it produces a database that looks
populated and is wrong, and the thing you would normally reach for to fix it is the backup you
have just consumed.

**A scheduled backup does not start if one is already running.** Overlapping runs against the same
database compete for the resource they are trying to protect, and the second one is not more
current in any way that matters.

**Every backup records what it is** — when, by whom, of what, and a checksum — because a directory
of files with timestamps is not a set of backups, it is a set of files somebody will guess about.

**Schema changes only ever go forwards.** There is no automated way back. This is a deliberate
choice rather than an omission, and it means a rollback is a restore, not a downgrade: take a copy
before any upgrade, because that copy is the only route back.

**Some schema changes leave a residue an older version reads differently.** Where a change re-keys
existing records, going backwards is not a true inverse — the reversal cannot distinguish a record
it rewrote from one that always looked that way, and anything it removed as redundant is gone. So
a rollback after such a change is a restore from before it, not an undo of it.

## What proves it

Verification before use is pinned by the case that matters: a corrupted backup [is refused, with
the reason](internal/backup/service_test.go#TestService_RunRestore_BadChecksum) rather than
partially applied, and integrity [is checkable on
demand](internal/backup/service_test.go#TestService_VerifyChecksum). The ordinary restore path [is
pinned too](internal/backup/service_test.go#TestService_RunRestore).

Not overlapping is pinned in both places it could go wrong — the scheduler [skips a run while one
is active](internal/backup/scheduler_test.go#TestScheduler_SkipsWhenActive), and the service
[handles concurrent requests to
create](internal/backup/service_test.go#TestService_ConcurrentCreate) — with [whether one is
running being answerable](internal/backup/service_test.go#TestService_IsActive).

Scheduling is pinned including its refusal: a schedule that cannot be parsed [is
rejected](internal/backup/scheduler_test.go#TestScheduler_InvalidCron) rather than silently never
firing, a change to the schedule [is picked
up](internal/backup/scheduler_test.go#TestScheduler_Reschedule_PicksUpNewSchedule) without a
restart, and [backups actually run](internal/backup/scheduler_test.go#TestScheduler_RunsBackup).

Each backup describing itself is pinned by [the manifest round
trip](internal/backup/manifest_test.go#TestWriteAndReadManifest), with an empty directory
[listing as empty rather than
failing](internal/backup/manifest_test.go#TestListManifests_EmptyDir) and a missing one [not
being a crash](internal/backup/manifest_test.go#TestListManifests_NonexistentDir) — a new
installation has neither. Removing old backups is pinned by
[pruning](internal/backup/service_test.go#TestService_Prune), and a failure while creating one [is
recorded rather than swallowed](internal/backup/service_test.go#TestService_CreateFailure).

**Nothing proves a restore produces a working service.** What is asserted is that a corrupt backup
is refused and that the restore mechanism runs. That the restored database is complete, that the
service starts against it, and that the estate looks as it did — none of that is covered. A
restore has been exercised by hand, and that is the only evidence.

**Nothing proves an upgrade preserves the record.** Schema changes are applied going forwards and
that path is exercised, but no test takes a populated database through an upgrade and checks that
the human judgement in it survived. That is the loss this journey exists to prevent, and it is not
defended by anything automated.

**Nothing here proves installation at all.** Getting the software onto a host, and upgrading a
running deployment, is done with packages and has no journey behind it. It is one of the
requirements everybody held self-evident, and it is the first thing anybody does.

**The load-bearing assumption:** that a backup is taken before every upgrade. Every rollback story
above reduces to restoring one, so if that habit is not followed the answer to "how do we go back"
is that we do not. Nothing in the product enforces it.
