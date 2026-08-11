# Building against this from the outside

**As an engineer wiring this up to something else, I want an ordinary documented HTTP API I can
write a program against, so I can get our data into whatever system needs it — with no AI in the
loop, and without asking anybody here how it works.**

Not the assistant journey. No model, no conversation, nothing deciding what to fetch — a program
I wrote, running unattended, doing tonight what it did last night. It needs close to the opposite:
an assistant wants small answers it can think about, I want all of it, in a shape that has not
moved since I wrote the code.

## What I need

The whole surface written down, in OpenAPI — Swagger, as most still call it — because that is
what generates a client and what my colleagues can read. Not a curated subset, not a hand-kept
page. A capability missing from that document does not exist for me, and I will go
reverse-engineering it out of the browser's network tab, which is where wrong assumptions start.

List, then ask about one. The organisations, machines, cookbooks and repositories — get the list,
then everything we hold on any single one: what the static check said, what happened when it last
ran on a real machine. That pair is the whole shape, and it is already how the screens work.

Room for purposes nobody here has discussed. The ask arrived with no use cases: the steer was
anything useful from the existing API, and a batch load into another platform is one example
somebody offered, not the brief. So describe what is there, completely, and let whoever turns up
work out what they need. A surface fitted to today's example needs extending every time anyone has
an idea, and each of those is a conversation, a release and a wait.

States I can tell apart. If a cookbook has no repository, say so — do not send me the same blank
I would get for a question nobody has answered yet. A field that vanishes when it is empty is the
same fault: my program cannot distinguish "we do not know" from "this version does not have that
field", and it will guess wrong. Screens can render a dash for both; a program cannot.

All of it, not a page at a time. Loading the estate means the estate, not the first fifty. That is
the direct opposite of the assistant journey, and both are right — so completeness is something I
ask for, rather than the two of us arguing over what a list does by default.

Answers that keep their shape. A field quietly changing meaning breaks me silently — not at the
change, but three weeks later when somebody notices the other system's numbers are wrong. I would
rather be told an upgrade will break me.

To know when it went wrong, in a way a machine can act on. Nobody is watching at three in the
morning, so "nothing changed" and "this failed" have to be distinguishable without a human reading
a message.

## The decisions behind it

**One description, not two.** The same document the assistant reads — see [asking my assistant why
this is failing](agent-access.md) — pinned by the same tests. Two descriptions of one API is the
copy-that-rots problem with extra steps, and the second is always the stale one.

**Derived from what is served, never written alongside.** It matters more here than there: an
assistant reading a wrong description tries something else, my program fails at three in the
morning and nobody finds out for a week.

**A state is sent as itself.** Never rewritten to blank because a screen wanted a dash, never
omitted because it happens to be empty. Where the display wants something else, that is the
display's job.

**Completeness is asked for, not stumbled into.** Lists stay bounded by default, or somebody one
day gets the whole estate by accident. Getting all of it is deliberate, and allowed to be slow.

**A program gets an account of its own.** Credentials follow the assistant journey, and that
decision was made for somebody sitting at an editor. It survives an unattended job because the
job stops borrowing mine: it has its own, and the level it needs is set on it.

## How I would know it worked

I generate a client, write the load, and it runs for a month untouched. When somebody changes the
API I hear it from my build, not from the numbers.

## What proves it

Pulling the whole of something out works at size: a large set [streams
out](internal/webapi/handle_exports_test.go#TestHandleExports_LargeSet_StreamsSynchronously)
rather than being truncated or queued, and what is worth extracting comes out
[whole](internal/webapi/handle_exports_test.go#TestHandleExports_Sync_NodesCSV) —
[cookbooks](internal/webapi/handle_exports_test.go#TestHandleExports_Sync_Cookbooks),
[roles](internal/webapi/handle_exports_test.go#TestHandleExports_Sync_Roles),
[repositories](internal/webapi/handle_exports_test.go#TestHandleExports_Sync_GitRepos). Asking for
something not extractable [is refused
plainly](internal/webapi/handle_exports_test.go#TestHandleExports_InvalidExportType).

Lists stay bounded whatever is asked: a caller asking for nothing [gets a bounded
answer](internal/webapi/eventhub_test.go#TestParsePagination_Defaults), one asking for everything
[is clamped](internal/webapi/eventhub_test.go#TestParsePagination_ClampMax).

The rest is this journey's suite, and most of it is red.

**Nothing proves this suits a purpose nobody has described.** Unfalsifiable by construction — the
test is somebody outside building something we did not anticipate without asking us anything. The
temptation to resist is inventing the missing use cases and designing for them: the first draft of
this journey asked for incremental extraction, which nobody requested and which came from reading
one offered example as the brief.

**Nothing proves the shape holds across an upgrade.** No test fails when a field changes meaning,
and a generated description will describe the new meaning as confidently as the old. Generating it
stops it going stale; it does nothing about it changing under somebody.

**Nothing proves an unattended job can run for a month.** All exercised by hand, in one sitting, by
somebody logged in.

**The load-bearing assumption, now settled: the job gets its own account.** We already make these
— local ones here, and our identity provider carries accounts for machines — so it does not have
to borrow mine, and the load neither stops when I leave nor keeps running as somebody who has
gone.
