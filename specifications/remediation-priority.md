# Deciding what to fix first

**As the engineer doing the remediation, I need the broken things ranked by what it costs me
against what it buys the estate, so I start on the thing that matters instead of the thing
that happens to be at the top of an alphabetical list.**

Finding problems was never the hard part. Any scan produces hundreds. The hard part is that
my time is the scarce resource in this programme, and a flat list of hundreds of faults is
indistinguishable from no list — I will start at the top, get through five, and the five
will be the wrong five.

## What I need to see

For each broken thing: roughly how much work it is, and how much of the estate it unblocks.
Those two together are the ranking. A one-line fix in something everything depends on beats
a fortnight of work in something two machines use, and without both numbers side by side
nobody can see that.

What is actually wrong, in enough detail to start — not "this cookbook has offences" but
which ones, and why each matters for this upgrade rather than in general.

Where a fix can be applied automatically, what it would change, before it changes anything.
I will not run an automatic rewrite across this estate on trust.

Which of it is mine, or my team's, so I can cut the list down to what I am responsible for.

## The decisions behind the ranking

**Not everything a scan reports is a migration problem.** Most of what static analysis
produces is style and modernisation advice that has nothing to do with whether the thing
survives the version change. Presenting all of it equally buries the handful that will
actually break, and after the second false alarm I stop reading. The list must distinguish
"this will break" from "this is worth tidying one day", and it must tell me on what basis
that call was made, because I will be asked to defend it.

**A blocked list I stop trusting is worse than no list.** Once I believe the list is padded,
I work around it, and then I miss the real one. Everything above is in service of that.

**Verify before designing against this:** the ranking assumes each broken thing carries the
verdicts of every source that had an opinion on it, tagged with which source, and a rule
deciding which wins. That structure is what lets a human verdict overrule a scan without
building a second list. It was true when this was written and the tree moves — if it is not
true now, stop and say so rather than quietly ranking some other way.

## How I know it worked

I work down the list from the top, and I do not find myself thinking "why is this here"
before I get to the end of the first page.
