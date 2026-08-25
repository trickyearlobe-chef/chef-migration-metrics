# One person, many names

**As the administrator keeping the ownership record honest, I need the same person recognised
however they were written down, because otherwise one engineer becomes four owners, each
holding a quarter of their work, and none of the four lists is right.**

The names arrive from everywhere. An asset database has a mail address. The commit history has
whatever somebody configured on their laptop. The sign-in system has a corporate username. The
spreadsheet has a display name with a middle initial. Every one of these is the same person, and
every time we treat them as four the work splits and the totals stop meaning anything.

## What I need

To attach the other names a person is known by to the one record for them, so that whichever
form arrives next, it lands on the person and not on a new stranger.

A standing list of people who look like they might already be somebody else, so I can work
through it rather than discovering the problem when somebody says "why does this say I own
sixty repositories".

To merge two records when they are the same person, and have all of the work follow — every
assignment, every alternative name, and the name I merged away, so it never comes back as a new
person on the next import.

To see who changed the ownership record and when. This decides who is accountable for thousands
of things, so a change to it without a name against it is not acceptable.

## The decisions behind it

**A person's sign-in name is the anchor.** It is the one identifier the organisation controls
and does not recycle. Everything else — mail addresses, commit names, display names — hangs off
it as an alternative. Anchoring on anything else means the record drifts as people change teams
or the directory is reconfigured.

**Looking for duplicates has to compare the record names, not only the alternative names.** An
owner created from commit history has no alternative names at all, so a search that only looks
at those cannot see half the catalogue — and the half it cannot see is precisely the half nobody
imported deliberately.

**Merging keeps the name it absorbed.** Dropping it would leave the next import free to create
it again, and the duplicate would reappear on a schedule.

**An audit entry either identifies a thing completely or does not mention one.** A record saying
something was assigned, without saying what, is worse than one that says only that an assignment
happened: it looks like a full account of a change while being unreconstructable.

## What proves it

Duplicate detection is pinned including the case that catches the blind spot: candidates are
[paired by similarity](internal/datastore/owner_duplicates_functional_test.go#TestFunctional_OwnerDuplicates_PairsSimilarOwners),
and matching happens [on the record's own name as well as on an alternative
name](internal/datastore/owner_duplicates_functional_test.go#TestFunctional_OwnerDuplicates_MatchesOnAliasValueToo),
which is what keeps owners created from commit history inside the search.

Merging is pinned on all three of its awkward parts: the work, the alternative names and [the
absorbed name itself all
move](internal/datastore/owner_merge_functional_test.go#TestFunctional_MergeOwners_MovesWorkAliasesAndTheSourceName);
an assignment [the target already holds is
dropped](internal/datastore/owner_merge_functional_test.go#TestFunctional_MergeOwners_AssignmentTheTargetAlreadyHasIsDropped)
rather than duplicated; a name that [is already recorded as an alternative is not an
error](internal/datastore/owner_merge_functional_test.go#TestFunctional_MergeOwners_SourceNameAlreadyAliasedIsNotAnError),
so re-running a merge is safe. Merging a record into itself, or into something that does not
exist, [is
refused](internal/datastore/owner_merge_functional_test.go#TestFunctional_MergeOwners_RejectsUnknownAndSelfMerge).

The audit rules are pinned: [any action is
recorded](internal/datastore/owner_merge_functional_test.go#TestFunctional_AuditLogRecordsAnyAction),
and [half a reference to a thing is
refused](internal/datastore/owner_merge_functional_test.go#TestFunctional_AuditLogRefusesHalfAnEntityReference)
— naming the kind of thing without naming which one is rejected, and both halves or neither are
accepted. Alternative names are [validated before being
stored](internal/datastore/owner_aliases_test.go#TestInsertOwnerAlias_Validation) and [record
where they came from](internal/datastore/owner_aliases_test.go#TestInsertOwnerAliasParams_DefaultSource),
so an automatically derived name can be told from one a person asserted.

Most of the above needs a real database and runs under its own build tag rather than in the
gating suite.

**Nothing proves two records are the same person.** The list is candidates, ranked by looking
similar. Confirming is a human act and always will be — two engineers really can have the same
name.

**Nothing proves a merge can be undone.** It moves work and absorbs a name; there is no test for
reversing it, and the absorbed name is deliberately kept as an alternative rather than restored.
Treat a merge as one-way.

**The load-bearing assumption:** that a person's sign-in name is stable and not reissued. If the
directory ever recycles one, work silently transfers to a different human being, and nothing
here would detect it — every list would simply be confidently wrong. Verify that property holds
in the organisation before relying on the anchor.
