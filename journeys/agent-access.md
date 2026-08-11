# Asking my assistant why this is failing

**As the engineer doing the migration work, I want the AI assistant already open in my editor to
read what this service knows, so I can ask it why a cookbook is failing and get an answer
grounded in our estate rather than in something it read on the internet.**

Today it cannot see any of this, so I paste screenshots and retype findings into a chat window.

## What I need

Whichever assistant I have. Copilot for one team, Claude for another, whatever the platform
vendor bundles for a third, and a different list next year. So: something they can all consume,
not an integration with one of them.

A way in the tooling already understands — one address, one credential, and it works out for
itself what it can ask. Glue code per question gets written once, for the questions we thought
of, and never extended.

The failure itself, not the verdict. The error and the trace under it, which cookbook was running,
which machines it happened on. Then the source: the contents of the file it points at, out of the
repository we already hold. A verdict alone I could summarise myself; the three together are the
diagnosis.

Our two judgements about a cookbook, as we made them. Not the raw static check but our
reclassification of it — which findings actually block the upgrade and which are tidying — and
what happened when we last ran it on a real machine. Given only the raw findings, the assistant
will rank the wrong ones first, which is the mistake this service exists to stop.

A credential from my own record, without raising a ticket. I want to see when it was last used,
and destroy it the moment I am unsure — pasting credentials into editor tooling puts them in more
places than I can track.

Everything the API can answer, described in OpenAPI — Swagger, as most still say it — because
that is what assistants and client generators read. Our hand-written descriptions have been wrong
before: paths renamed, and one written up as built that never was. A description I cannot trust
is worse than none, because I will believe it.

To narrow before I fetch, and fetch a page at a time. An assistant's room to think is small
against a hundred and twenty thousand machines. One unbounded answer fills it, and then it forgets
the question and starts inventing. So the narrowing I do on screen has to be available to it,
nothing unbounded by default, and it should be able to ask the shape of a thing — how many,
grouped how — before pulling what it needs to read closely.

And to tell what it can ask for without being told. What we expose has to name its own
capabilities well enough that an assistant picks the right one from a long list, and can tell when
it has used one wrongly rather than reporting the empty answer it got. We have field reports of
that exact failure on other tools we built.

Diagnosis is reading, and an assistant that only reads is one I can point at production without
thinking hard. But diagnosis is supposed to end up in the register of failures — so if it cannot
write there, I read its answer and retype it, which is the copying I set out to stop. And where an
entry is carried by a ticket, the reference is already in the entry: an assistant that also reaches
our ticketing system can keep both sides saying the same thing.

So I choose, when I make a credential, whether it can also write. Most will be read-only. Working
through a batch of failures I will make one that writes, and I will know I did. What I will not
accept is a note the assistant wrote appearing under my name, worded like something I decided.
Nothing in that register is overwritten, so the risk is not losing anything — it is signing
findings I have not read.

It has to work where the service is installed: inside the customer's estate, reached through a
controlled desktop. A second thing deployed alongside will not happen there.

## The decisions behind it

**The description is derived from what is served, not written next to it.** A hand-written copy
starts rotting when it is committed — the failure that killed the document set this replaced. A
renamed path must break the build, not a customer's client.

**A credential is another way into the same account.** My level of access, exactly what I see on
screen, through something other than the web interface. No second permissions model. An account
can belong to a machine as easily as to me, so nothing here needs a service account. When I leave,
my access leaves with me.

**The person making one chooses whether it can write; read only if they do not choose.** Not an
administrator setting — the choice belongs to whoever is about to hand it to a tool, because only
they know what for.

**Writing means the register of failures and nothing else.** A credential that can write is not
an unlocked service; it can record a finding where findings go.

**An entry it wrote must be visibly not mine.** The register exists so a later reader can weigh
who said what. So allowing writing and recording what made the entry are one decision.

**Shown once, destroyable instantly.** No recovering an old one. Immediate, because the reason to
destroy one is believing somebody else has it.

**No answer is unbounded, and the caller does not have to ask for the bound.** A default that
returns everything eventually will, and it lands as an assistant gone vague rather than an error
anyone sees. Asking for more gets less.

**The assistant-facing surface is part of the service.** Not a sidecar. It ships and upgrades with
everything else.

## How I would know it worked

I add one address and one credential to my editor, ask why a cookbook is failing, and get back the
actual finding, the actual version, and the other places with the same problem — all checkable on
screen. And when somebody renames something I find out from a red build, not from my assistant
quietly answering with nothing.

## What proves it

Almost none of it yet. The journey suite is the list.

Pinned today is only the door it comes through: a credential in the request header [is treated as
signed in](internal/auth/middleware_test.go#TestRequireAuthValidBearerToken) as a browser
[cookie is](internal/auth/middleware_test.go#TestRequireAuthValidCookie), an expired one [is
not](internal/auth/middleware_test.go#TestRequireAuthExpiredSession), and reaching above your
level [is refused](internal/auth/middleware_test.go#TestRequireRoleDenied) — the mechanism that
makes "a credential carries its account's level and no more" true rather than aspirational.

An address nobody serves [is reported as not
existing](internal/webapi/router_api_routing_test.go#TestUnroutedAPIPath_IsNotFoundNotMethodNotAllowed)
rather than existing-but-unavailable. An assistant working from a description guesses; the
difference between those two answers is whether it corrects itself or insists.

**Nothing proves the answers are any good.** Whether an assistant produces a diagnosis worth
acting on depends on the model, the question, and whether what we expose is the part that matters.
Only using it settles that.

**Nothing proves it works with any particular assistant.** Each is differently opinionated about
discovery, and a description correct on paper can still be unusable by the tool somebody has.

**Nothing proves the credential does not leak.** Editor tooling copies configuration where neither
I nor this service can see. Quick destruction is a mitigation, not a fix. A write-capable one is
the worse loss, and all that stands behind it is my habit of only making them deliberately.

**The load-bearing assumption:** that what is worth asking is close to what the screens already
answer. If diagnosis needs questions no screen asks — across cookbooks, runs and ownership at once
— then describing the existing surface is a smaller piece of the work than it looks.
