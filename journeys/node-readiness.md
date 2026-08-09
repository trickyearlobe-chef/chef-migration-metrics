# Knowing which machines can move, and which are not really there

**As the engineer running the upgrade, I need to see which servers are ready to move and
exactly what is stopping the ones that are not, so I can work a list instead of
investigating machines one at a time.**

There are tens of thousands of them. Anything that makes me open a machine to find out why
it is blocked does not scale — I will not do it forty thousand times, so I will stop
looking, and the ones that matter get lost in the ones that do not.

## What I need to see

For every server, whether it is ready, and if not, which check failed — is it disk space,
is it the cookbook analysis, is it the converge testing, or more than one. I need that on
the list itself, at a glance, without opening anything. Colour alone is not enough; I have
colleagues who cannot rely on it and I read these lists tired.

When I do open a machine, the specific reasons — which cookbooks are blocking it, how much
disk it actually has against how much it needs — and what it pulls in, so I can see whether
the problem is this machine or something it inherits.

## The machines that are not really there

A server that has not reported for a week and a server that has not reported for six months
are not the same problem, and treating them the same wasted my time for months. The first
is a reboot or a patch window and will come back on its own. The second is decommissioned,
or lost, or nobody has owned it for a year — and chasing it is work with no outcome.

So I need those told apart: reporting normally, quiet for a while, and gone. What I do
about each is different, and I need to be able to set the whole "gone" pile aside without
losing it.

**A machine we cannot see must not be reported as a machine that is fine.** If we have no
recent data, say so — do not let absence of evidence render as a pass.

## How I know it worked

I can go from "forty thousand servers" to "these are the ones I have to deal with this
week, and here is why" without opening a single machine, and the pile I set aside as gone
does not come back to haunt me next month.

## What proves it

Telling the three states apart is pinned by [the tier
contract](internal/staleness/staleness_test.go#TestComputeTier). That a blocked machine can
never come back as ready is pinned by [the compatibility
contract](internal/analysis/readiness_test.go#TestCheckCookbookCompatibility_BlockedAlwaysIncompatible).

"Colour alone is not enough" is checked only through a stand-in: [the status
icons](frontend/src/components/CheckStatusIcons.test.tsx) are asserted to carry a spoken
label and a distinct overlay shape, not just a colour. That is evidence the information
survives without colour, not proof it is readable.

**Nothing proves the part that matters most** — that the list is usable at a glance across
tens of thousands of rows without opening anything. That is a judgement made by looking at
it, and no assertion stands in for it.
