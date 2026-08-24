# Knowing who has to do the work

**As the migration lead, I need every broken thing attached to the team that will fix it, so
that the programme is a set of assignments people can be held to rather than one enormous list
I am personally responsible for.**

The technical answer to "what is stopping us" was never the hard part. The hard part is that a
list of thousands of broken cookbooks belongs to nobody, so it belongs to me, and I cannot fix
them all. Until each one has a name against it there is no programme, only a report.

## What I need

For a person or a team, everything that is theirs and what state it is in — the repositories,
the cookbooks, the machines — so I can send one message that is entirely about their work.

For a piece of work, who owns it, and if nobody does, to see that plainly rather than have it
sit invisibly in a total.

**The unowned pile, as a first-class thing.** Work nobody has been made responsible for is the
single most useful list here, because it is the list that tells me what conversations I still
have to have. It must not be hidden behind a filter that defaults to hiding it.

To cut every view down to one owner's work, and to have that survive into an export, because
what I actually send somebody is a file.

Where nobody has claimed something, a hint at who plausibly should — the people who have
actually been changing it. That is not ownership, but it is where the conversation starts.

## The decisions behind it

**Ownership is attached to the thing, not to the report.** It has to reach whatever anybody
looks at — a list, a detail page, a total, an export — or the numbers disagree with each other
and everything here loses its authority.

**A repository is identified by its name, not by where it is hosted.** Addresses change: a
server is renamed, a project moves between organisations, the protocol changes. If ownership is
keyed on the address then a routine infrastructure change quietly un-owns work that somebody
had claimed. Keying on the address also makes the product disagree with itself when one place
matches on the name and another on the address: a claimed repository shows as unowned in the
list, the owner's own page shows nothing for it, and the count agrees with neither.

**Who has committed to something is evidence, not a verdict.** It suggests who to ask. Making
it an automatic assignment would attribute work to whoever last fixed a typo.

## What proves it

The keying decision is pinned on the readers that must resolve by name: an owner's own
summary [resolves their repositories by
name](internal/datastore/ownership_git_repo_key_functional_test.go#TestFunctional_OwnerGitRepoSummary_ResolvesByRepoName),
and a cookbook [inherits its repository's owner by name
too](internal/datastore/ownership_git_repo_key_functional_test.go#TestFunctional_CookbookInheritsRepoOwnerByName).
Those two are the paths where address-matching would bite, which is why they carry tests
rather than the rule being written down once. Both need a real database and run under their own
build tag rather than in the gating suite.

That an owner filter reaches the export as well as the screen is covered for one view by [the
parity contract](internal/datastore/node_snapshot_export_functional_test.go#TestFunctional_NodeExport_FilterParity),
and the cohort journey it shares that behaviour with is [naming a slice of the
estate](named-cohorts.md).

**Nothing proves an owner's cookbook verdict.** The compatible, incompatible and untested
counts on an owner's page are not asserted anywhere, and they are derived rather than stored —
which makes them the least trustworthy numbers in this journey and the ones most likely to be
quietly wrong. Establish what they actually return before quoting them to anybody.

**Nothing proves the unowned pile is complete.** "Unowned" is the absence of a record, and
absence is exactly what a wrong join produces as well. A repository that is owned but keyed
wrongly, and one that genuinely has no owner, look identical on the screen — which is what made
the address-versus-name fault above invisible for as long as it was.

**The load-bearing assumption:** that every place ownership is read resolves it the same way.
There is no single reader to point at, so this cannot be pinned by one test — it is a property
of not having a fourth reader appear that invents its own rule. If a new view starts reporting
ownership, check it against the two contracts above before believing it.
