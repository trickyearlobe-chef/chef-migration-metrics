# Finding the setting I came for

**As the systems administrator who looks after this service a few times a month, I need to find
the setting I came to change by reading the menu, because I do not know how this application is
built and I should not have to.**

I am not in here every day. I come because something is wrong or somebody asked for a change,
and I leave again. Everything I learn about where things live, I have to learn again next time.

## What is wrong with it today

It is organised around how it was built rather than what I came to do. Reading the menu does
not tell me where anything is, so I hunt.

- A menu entry named after the analysis tools takes me to a screen named after one tool.
- Where the Chef tools live is set on a tab belonging to the machine-testing screen, while the
  tool that setting is for has a screen of its own.
- How much work runs at once — collection, fetching, scanning, the lot — is filed under
  machine testing, which is the one thing most of it has nothing to do with.
- Two different screens both present themselves as the place for analysis tool settings.

None of that is wrong from the inside. All of it is wrong from where I am standing.

## What I need

- The name I click to be the name of the place I arrive at.
- Settings grouped by what they change, not by which part of the program reads them.
- Everything I am allowed to change reachable by clicking. Not by typing an address, and not by
  knowing that one screen keeps another screen's settings on a tab.
- One home per setting, so I am never deciding which of two screens is the real one.
- To find it again next month without rediscovering the layout.

## The decisions behind it

**Renaming and moving things is allowed, and is most of the work.** The layout is the thing
being fixed. An address that changes has to keep working, because the one thing I do write down
is a link.

**Grouping is by the task, not the subsystem.** Which component reads a setting is an
implementation detail and a bad filing system: it changes when the code changes, and it means
nothing to me.

**This is not about adding settings.** Everything I need is already here somewhere. The
complaint is that I cannot find it.

## How I would know it worked

Somebody who has never seen this service is asked to change where the Chef tools live. They
find it by reading the menu, first guess, without asking anybody and without typing an address.

## What proves it

**Almost nothing, and one of these is worse than red.** The suite is the list, starting with
[a menu entry landing on a screen with a different
name](internal/webapi/admin_navigation_journey_test.go#TestJourney_TheNameIClickIsWhereIArrive)
and [settings reachable only by knowing another screen keeps
them](internal/webapi/admin_navigation_journey_test.go#TestJourney_EverySettingIsReachableByClicking).

Held already, and the reason a rename is safe to attempt: [an address that moved still
works](internal/webapi/admin_navigation_journey_test.go#TestJourney_AnAddressThatMovedStillWorks).

**Nothing proves the grouping is any good.** Whether a setting is filed where somebody would
look for it is a judgement, and no test makes it. The check is the person who has never seen
it, and that has to be run by asking one.

**Nothing proves I can find it again next month**, which is the whole complaint. What can be
tested is that names match and nothing is hidden; that they are the *right* names cannot be.
