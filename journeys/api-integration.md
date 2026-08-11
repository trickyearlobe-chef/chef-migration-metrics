# Building against this from the outside

**As an engineer wiring this up to something else, I want an ordinary documented HTTP API I can
write a program against, so I can get our data into whatever system needs it — with no AI in the
loop, and without having to ask anybody here how it works.**

This is not the assistant journey. There is no model reasoning about anything, no conversation,
nothing deciding what to fetch. There is a program I wrote, running unattended on a schedule,
doing the same thing tonight it did last night. What that program needs from an API is almost the
opposite of what an assistant needs, which is why this is written down separately: an assistant
wants small answers it can think about, and I want all of it, in one pass, in a shape that has
not moved since I wrote the code.

## What I need

The whole surface written down, in OpenAPI — Swagger, as most people still call it — because
that is what generates a client and what my colleagues already know how to read. Not a curated
subset of it, and not a page somebody maintains by hand. If a capability exists and is not in
that document, then for me it does not exist, and I will end up reverse-engineering it out of the
browser's network tab, which is where the wrong assumptions come from.

To start from listing things and then asking about one of them. The organisations, the machines,
the cookbooks, the repositories — get the list, then get everything we hold about any single one
of them: what the static check said about it, what happened when it was last run on a real
machine. That pair is the whole shape of what I want, and it is already how the screens work,
which is a good sign it is the right shape rather than one invented for this.

Room for purposes nobody here has discussed yet. This is the requirement that decides the shape
of the whole thing, so it needs saying plainly: the ask arrived with no use cases attached. The
steer was anything useful from the existing API, and a batch load into another platform is one
example somebody offered, not the specification. So the job is to describe what is there,
completely and accurately, and let whoever turns up work out what they need from it. A surface
built to fit today's example is one somebody has to extend every time anyone has a new idea, and
each of those is a conversation, a release, and a wait.

All of it, not a page at a time. When I load the estate into another system I need the estate,
not the first fifty of it. That is the direct opposite of what the assistant journey asks for,
and both are right — so completeness has to be something I can ask for explicitly, rather than
the two of us arguing over what a list does by default.

Answers that keep their shape. My program breaks silently when a field quietly changes meaning —
not loudly, at the point of the change, but three weeks later when somebody notices the numbers
in the other system are wrong. I would rather be told an upgrade will break me than find out that
way.

To know when it went wrong, in a way a machine can act on. Nobody is watching this at three in
the morning. It has to be possible to tell "nothing changed" from "this failed" without a human
reading a message.

## The decisions behind it

**One description, not two.** The same document the assistant reads is the one I generate a
client from — see [asking my assistant why this is failing](agent-access.md), which needs it for
discovering what can be asked and pins it with the same tests. Two descriptions of one API is the
copy-that-rots problem with extra steps, and the second one is always the stale one.

**It is derived from what is served, never written alongside it.** The same reasoning as in that
journey, and it matters more here: an assistant that reads a wrong description asks a question
that fails and tries something else, whereas my program fails at three in the morning and nobody
finds out until the other system has been wrong for a week.

**Completeness is asked for, not stumbled into.** A list stays bounded by default, because the
alternative is that somebody one day gets the whole estate by accident. Getting all of it is a
separate, deliberate request, and it is allowed to be slow.

**A programme is not a person, but that is not settled here.** Everything about credentials in
this journey follows the assistant journey's decisions, and one of them — that a credential is a
person, at that person's level of access — was made for somebody sitting at an editor. Whether it
survives contact with an unattended job is the open question below, not something to be quietly
answered by whoever builds this first.

## How I would know it worked

I generate a client from the description, write the load, and it runs for a month without me
touching it. When somebody here changes the API, I hear about it because my build tells me, not
because the numbers went wrong.

## What proves it

Pulling the whole of something out is real and works at size: a large set [streams out
synchronously](internal/webapi/handle_exports_test.go#TestHandleExports_LargeSet_StreamsSynchronously)
rather than being truncated or queued, and the things worth extracting come out
[whole](internal/webapi/handle_exports_test.go#TestHandleExports_Sync_NodesCSV) —
[cookbooks](internal/webapi/handle_exports_test.go#TestHandleExports_Sync_Cookbooks),
[roles](internal/webapi/handle_exports_test.go#TestHandleExports_Sync_Roles) and [the
repositories](internal/webapi/handle_exports_test.go#TestHandleExports_Sync_GitRepos) as well.
Asking for something that is not extractable, or in a shape that is not offered, [is refused
plainly](internal/webapi/handle_exports_test.go#TestHandleExports_InvalidExportType) rather than
half-answered.

That an ordinary list stays bounded whatever is asked of it is pinned where a caller asks for
nothing and [gets a bounded
answer](internal/webapi/eventhub_test.go#TestParsePagination_Defaults) and where one asks for
everything and [is clamped anyway](internal/webapi/eventhub_test.go#TestParsePagination_ClampMax),
which is what makes "completeness is asked for, not stumbled into" true rather than a preference.

The rest is the suite for this journey, and most of it is red.

**Nothing proves this suits a purpose nobody has described.** That is the central requirement and
it is unfalsifiable by construction — the test for it is somebody outside this project building
something we did not anticipate and not needing to ask us for anything. Until that happens, the
honest position is that we have guessed. The specific temptation to resist is inventing the
missing use cases and then designing for them: the first draft of this journey asked for
incremental extraction, which nobody requested and which came entirely from taking one offered
example as the brief.

**Nothing proves the shape holds across an upgrade.** There is no test that fails when a field
changes meaning, and a description generated from the code will happily describe the new meaning
as confidently as it described the old one. Generating the document stops it going stale; it does
nothing at all about it changing under somebody.

**Nothing proves an unattended job can run for a month.** Everything here has been exercised by
hand, in one sitting, by somebody logged in.

**The load-bearing assumption, and the open question:** that anchoring access on a person is
workable for a program that runs unattended. It is what the assistant journey decided and this
one inherits, and it is uncomfortable here — the job outlives the engineer who set it up, and
when they leave the nightly load stops, or worse, keeps running as somebody who has gone. The
alternative is an account that is not a person, which the assistant journey rejected for good
reasons that do not obviously apply to a scheduled program. Nobody has decided this. Do not
decide it by building it.
