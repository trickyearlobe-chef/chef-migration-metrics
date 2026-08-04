# Knowing where the estate stands

**As the migration lead, I need to know how far through the upgrade we are and whether it
is still moving, so I can answer the programme board honestly and see a stall before it
becomes a slipped date.**

Tens of thousands of servers have to move to a new version of Chef. The question I am
asked, repeatedly and by people who will not read a spreadsheet, is "how far through are
we?" Until this existed I could not answer it, and neither could anyone else.

## What I need to see

The fleet split by major version first — how much is on 16, on 17, on 18 — because that is
the shape of the problem and the shape of the conversation. Minor versions matter when I am
chasing a specific rollout, so I need to open a major version up and see them, not have
them all thrown at me at once.

**Two different questions that must not be blurred together.** "Can our cookbooks survive
the target version?" is analysis — it is a prediction, made against one target version.
"Is the new version actually running out there and converging?" is observation — it is what
the machines are really doing, and several versions can be in flight at once. Presenting
one as the other would make a stalled rollout look like a solved problem.

Progress over time, so I can see whether the last fortnight moved anything.

## What has to be true

**The charts must not flatter us.** Every axis starts at zero and no axis is truncated. I
screenshot these for a board pack, and a chart that exaggerates a trend is worse than no
chart, because it spends credibility I need later when the news is bad.

There is one target version at a time. Not a matrix, not a version I pick per chart — one,
set centrally, shown so I know which one I am looking at.

## How I know it worked

I can answer "how far through are we, and did it move this month" from one screen, without
exporting anything, and I am willing to put the number in front of the board without
caveating it.
