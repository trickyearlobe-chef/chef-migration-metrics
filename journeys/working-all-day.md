# Working all day without the tool getting in the way

**As anybody using this, I want to log on and get on with my work for several hours straight,
with gaps for lunch and meetings, without having to think about my session or wonder whether
what I am looking at is still true.**

This is not a feature. It is the difference between a tool I keep open all day and one I avoid.
Everything here is invisible when it works, which is why it never gets written down and why it
was missing.

## What I need

To sign in once in the morning and still be working in the afternoon. A lunch break is not a
reason to sign in again, and neither is a long meeting.

If my session does end, to be told — plainly, at the moment it matters — and put back where I
was afterwards. Not dropped on the front page having lost the selection I spent ten minutes
building.

**Never to be shown something that is no longer true.** This is the one that actually costs me.
A screen that stops updating looks exactly like a screen where nothing is happening. A chart
drawn from data that arrived before my session ended looks exactly like a current chart. If I
take a number from a stale screen into a meeting, the tool has done me more harm than if it had
shown me nothing at all.

To be able to tell the difference between "there is nothing to show", "we cannot reach it", and
"this is old". Those are three different situations and I act differently on each.

## The decisions behind it

**Nothing shows stale data as though it were current.** This binds every screen, not one of
them. The same rule has already been written down three times in this product from three
different directions — a saved selection the server cannot understand must not silently return
the whole estate, a machine we have not heard from must not read as healthy, and now a screen
whose session has ended must not keep displaying its last answer. It is one rule and it belongs
here rather than being restated in each journey that trips over it.

**An ended session is detected in one place, not in each screen.** If every screen has to notice
independently, then coverage is only as good as the newest screen, and the newest screen is
always the one nobody checked. There is a single point every request already passes through, and
that is where this belongs.

**And it is presented structurally**, so a screen cannot forget to. If showing the ended-session
state is something each page opts into, that is the same staleness problem wearing a different
hat.

**Whether the tool bypasses its own front door is itself checked.** The above only holds while
every request really does go through one place. That is not a thing to assert once and trust —
it is a thing to test, because it drifts silently and the drift is invisible until somebody hits
it.

**Time away is not the same as time elapsed.** A working day with a lunch break in it is the
normal case, not an edge case, and the design has to make that unremarkable.

## How I know it worked

I open it after breakfast, work through the day, go to lunch, come back, and never once think
about signing in. And I never catch a screen telling me something that stopped being true while
I was reading it.

## What proves it

The session mechanism itself is pinned server-side: a lifetime is
[configurable](internal/auth/session_test.go#TestNewSessionManagerCustomLifetime) and [a
nonsensical one is refused](internal/auth/session_test.go#TestNewSessionManagerNegativeLifetime),
a valid session [is accepted](internal/auth/session_test.go#TestValidateSessionSuccess), and an
expired one [is refused by the middleware](internal/auth/middleware_test.go#TestRequireAuthExpiredSession)
rather than being honoured — including [in combination with a role
check](internal/auth/middleware_test.go#TestAdminOnlyCombinedExpiredToken). A session that never
existed [is refused too](internal/auth/middleware_test.go#TestRequireAdminNoSession), so the
refusal is not an artefact of expiry handling.

**Nothing proves any of the part this journey is actually about.** The server correctly refuses
an expired session; what happens in front of the person is unimplemented and untested. Two tests
have to be written, and the second is the one that stops coverage rotting:

- An ended session becomes an application-wide condition from the single place requests pass
  through — one test, one code path, no per-screen coverage to maintain.
- **No part of the application reaches the network except through that place.** This is
  enumerable from the source rather than reviewed by hand, so it fails the commit that
  introduces a bypass. Four bypasses exist today, and one of them serves the front page — which
  is why the symptom shows up as charts that will not draw.

**Nothing proves the lunch break.** That a session survives a normal working day with normal
gaps in it is a property of a real day, and nobody has sat in front of it for eight hours.

**The load-bearing assumption:** that a person can be returned to what they were doing after
signing in again. If the thing they were looking at cannot be described well enough to go back
to, then the honest promise shrinks to telling them clearly and starting again — and the
selection journey ([naming a slice of the estate](named-cohorts.md)) is what makes that
survivable rather than infuriating.
