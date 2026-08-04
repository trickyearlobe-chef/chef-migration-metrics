# Naming a slice of the estate so I stop rebuilding it

**As an operator working the estate, I need to name a selection I have built and get it back
later, so that the twenty-role filter I assembled this morning is still there tomorrow.**

The way I actually think about this fleet is in groups — all the Windows machines, all the
RHEL machines, everything sitting under the base roles. Those groups do not exist anywhere
in the tool. To get one I pick twenty roles out of a list by hand, and the moment I close
the page it is gone. So I build it again. Then I build it again slightly differently, and
now two of my numbers disagree and I cannot tell which is right.

## What I need

To build a selection once, give it a name that means something to me, and pick it again from
a list. On the views where I work — machines, cookbooks, repositories.

The same selection to mean the same thing wherever I use it, including when I export it.
A cohort that filters the screen one way and the export another way is worse than not having
it, because I will not notice.

To combine it with whatever else I am filtering by at the time, rather than it replacing my
whole view.

## The decisions behind it

**This adds no new way to query.** It remembers a selection that was already possible to
make by hand. Anything I could not filter by before, I still cannot — the value is entirely
in not rebuilding it.

**A selection the server does not understand must fail loudly, not quietly.** If a saved
cohort carries something the server will not accept, it has to say so. A filter that is
silently dropped returns the unfiltered estate, and an unfiltered result looks exactly like
a legitimate answer — I will read forty thousand machines as "everything matched" and act on
it. This has already caused real faults more than once, and it is the single most important
property here.

## How I know it worked

I open the machines view, choose the cohort by name, and get the same set I got last week —
and when I export it, the file matches what was on the screen.
