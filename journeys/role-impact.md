# Finding the one fix that unblocks thousands of machines

**As the engineer running the upgrade, I need to see which roles are blocked and what is
blocking them, so I can spend my effort where one fix moves the most machines instead of
grinding through servers one at a time.**

Roles are how this estate is actually organised. Machines do not get cookbooks directly —
they get a role, which pulls in cookbooks and often other roles. So one broken cookbook
sitting inside a base role that nearly everything includes is not one problem, it is the
whole fleet's problem, and it is also the single cheapest thing I will fix all month.

Working from the machine list alone hides this completely. Forty thousand blocked servers
can be four cookbooks.

## What I need to see

Which roles are ready and which are not, knowing that a role is only as good as its worst
dependency — one bad cookbook anywhere in the chain blocks it.

For a role I care about: what it pulls in, including what it inherits from roles nested
inside it, because that chain is where the surprises live and nobody holds it in their
head. Which of those are the ones actually blocking it.

How much rides on it — how many machines, and which parts of the estate. That is what tells
me whether this is a fix worth doing first or a corner case.

**A picture, but only where a picture helps.** The chain for one role or one machine is a
few dozen things and seeing it laid out makes the shape obvious. The same drawing for the
entire estate is thousands of things and is unreadable — it looks impressive and tells me
nothing. Small and scoped, or not at all.

## How I know it worked

I can name the handful of fixes that between them unblock most of the estate, and say how
many machines each one frees, without building it by hand from a list of servers.

## What proves it

The chain being followed all the way down — the part nobody holds in their head — is pinned
by [nested role
expansion](internal/nodekitchen/runlist_test.go#TestExpandRunList_NestedRoles), so what a role
inherits from roles inside it is counted rather than stopping at the first level. A role that
includes itself, directly or round a longer loop, [does not recurse
forever](internal/nodekitchen/runlist_test.go#TestExpandRunList_CycleDetection). That a
blocked cookbook cannot come back as compatible anywhere in that chain is pinned by [the
compatibility
contract](internal/analysis/readiness_test.go#TestCheckCookbookCompatibility_BlockedAlwaysIncompatible).

**One role we cannot read loses the whole chain.** A role that is referenced but not found
[fails the expansion outright](internal/nodekitchen/runlist_test.go#TestExpandRunList_MissingRole)
rather than resolving what it can and saying which part is missing. For a journey whose whole
value is seeing the chain, one unreadable role means no answer instead of a partial one — and a
partial answer with a gap named in it would be more use here.

**Nothing proves the counting.** How many machines a role carries, and therefore the ranking of
which fix frees the most, is not asserted anywhere. Nor is "a role is only as good as its worst
dependency" pinned as a roll-up rule; what is pinned is that the chain is fully walked and that
a blocked cookbook stays blocked, which is the input that rule needs rather than the rule
itself.

**Nothing proves the judgement about the picture** — that a scoped chain is worth drawing and
the whole estate is not. That is a decision to keep, not a property to test.
