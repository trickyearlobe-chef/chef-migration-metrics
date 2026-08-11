# Asking my assistant why this is failing

**As the engineer doing the migration work, I want the AI assistant that is already open in my
editor to be able to read what this service knows, so I can ask it why a cookbook is failing and
get an answer grounded in our estate rather than in something it read on the internet.**

The work is diagnosis. A cookbook comes back red, and the question is always the same one: what
is actually wrong with it, where else does that pattern appear, and what do I change. All of that
is already on the screen, spread over several of them. Reading it and reasoning about it is
exactly what the assistant is good at, and today it cannot see any of it, so I paste screenshots
and retype findings into a chat window.

## What I need

The assistant I already use, whichever one it is. That is not one product — it is Copilot for one
team, Claude for another, and the assistant our platform vendor bundles into the editor for a
third, and it will be a different list next year. So this has to be something they can all
consume, not an integration with one of them.

A way in that the tooling already understands. If I give an assistant one address and one
credential and it works out for itself what it can ask, that is a five-minute setup. If somebody
has to write glue code for each question, it will be built once for the questions we thought of
and never extended.

The failure detail itself, not just the verdict. When a run fails out there I want the assistant
to be able to read what actually came back — the error and the trace under it, which cookbook was
running at the time, and which machines it happened on. And then the source: the contents of the
file it points at, out of the repository we already hold, so the assistant is reading the same
recipe I would open rather than guessing at it from the name. A verdict without the trace and the
source is the part I could already summarise myself; the three together are the diagnosis.

The two judgements we have already made about a cookbook, as we made them. What the static check
says is wrong with it — and specifically our reclassified version of that, where we have decided
which of those findings actually block the upgrade and which are tidying, because the raw tool's
opinion is the thing that sends people down dead ends. And what happened when we last ran the
cookbook on a real machine, which is the only evidence that outranks the static check. If the
assistant can only see the raw findings, it will confidently rank the wrong ones first, and I
will have automated exactly the mistake this service exists to stop people making.

A credential I can get for myself, from my own record, without raising a ticket. I want to see
that I have one and roughly when it was last used, and I want to be able to throw it away and get
a new one the moment I am unsure about it — pasting credentials into editor tooling means they
end up in more places than I can keep track of.

Everything the API can answer, described in OpenAPI — Swagger, as most people still say it —
because that is what every assistant and every client generator already reads. There is an API
today. Nobody outside this project can describe it, and the descriptions we have written by hand
have been wrong before — paths that were renamed, and at least one that was written up as though
it had been built and never was. A description I cannot trust is worse for me than none, because
I will believe it and then spend an afternoon on a request that was never going to work.

To be able to narrow before I fetch, and to fetch a page at a time. This is the requirement I
would forget and then regret. An assistant has a finite amount of room to think in, and it is
small compared to our estate — a hundred and twenty thousand machines, tens of thousands of
findings. One unbounded answer fills that room, and once it is full the assistant stops being
able to reason at all: it forgets the question, drops what it read three steps ago, and starts
inventing. So the same narrowing I do on the screen has to be available to it, and no answer may
ever be unbounded by default. It should be able to ask for the shape of a thing first — how many,
grouped how — and only then pull the handful it actually needs to read closely.

That also means it has to be able to tell what it can ask for, without being told. Whatever we
expose has to name its own capabilities well enough that an assistant scanning a long list of
them picks the right one, and can tell when it has used one wrongly rather than confidently
reporting the empty answer it got. We have field reports of exactly that failure on other tools
we have built — the right tool present, not found, and an agent settling for a worse one.

Reading is the whole of what I am certain about. Diagnosis is reading, and an assistant that can
only read is one I can point at production without thinking hard about it.

But diagnosis is supposed to end up in the register of failures somebody has looked at and made a
call on, and the assistant is the thing doing the diagnosing — so if it cannot write there, I
read its answer and retype it, which is the copying I started out trying to stop. And when an
entry is being carried by a ticket rather than a person, the reference is already sitting in the
entry. An assistant that can also reach our ticketing system can follow that thread on its own:
see what was found here, see what the ticket says, and keep the two saying the same thing. That
is the part that saves the team real time.

So I want to choose, at the moment I make a credential, whether it can only read or can also
write. Most of mine will be read-only, because most of the time I am asking questions and I would
rather not think about it. When I am working through a batch of failures I will make one that can
write, and I will know I did. What I will not accept is a note the assistant wrote appearing
under my name, worded like something I decided, with nobody able to tell afterwards. Nothing in
that register is overwritten — a new call supersedes the old one and both stay readable — so the
risk is not losing anything, it is signing findings I have not read.

It has to work where the service is installed. Our customer's copy runs inside their estate,
reached through a controlled desktop. If this needs a second thing deployed alongside it, it will
not happen there, and there is where I need it.

## The decisions behind it

**The description is derived from what the service actually serves, not written next to it.** A
hand-written description is a copy, and a copy starts rotting the moment it is committed. This is
the same failure that killed the document set this journey replaced. If the description and the
routes can disagree without anything noticing, they will disagree, so the only version worth
building is one where a renamed path breaks the build rather than a customer's client.

**A credential is a person, not a service account.** It acts as me, at my level of access, and it
can see exactly what I can see on the screen and nothing else. Nobody has to reason about a
second, parallel permissions model, and when I leave, the access leaves with me because it was
never separate from me in the first place.

**The person making a credential chooses whether it can write, and read only is what they get if
they do not choose.** Not an administrator setting, not a property of the service — the choice
belongs to whoever will be handing the credential to a tool, at the moment they hand it over,
because they are the only one who knows what they are about to do with it. A credential that can
write is a deliberate act, and it stays visible as one.

**Writing means the register of failures, and nothing else.** The estate, the configuration and
anybody's account stay out of reach whatever the credential says. A credential that can write is
not an unlocked version of the service; it is one that can record a finding where findings go.

**An entry it wrote must be visibly not mine.** The register exists so a later reader can weigh
who said what and why, and an entry that reads as a person's judgement when it was a machine's is
not a lesser version of that — it is the opposite of it. So allowing writing and recording what
made the entry are one decision, and neither ships without the other.

**The credential is shown to me once, and I can destroy it at any moment.** If I lose it I make a
new one; there is no recovering the old one. Destroying it takes effect immediately, not at the
end of some session — the whole reason I would destroy one is that I think somebody else has it.

**No answer is unbounded, and the bound is not something the caller has to remember to ask
for.** A default that returns everything is a default that will one day return everything, and
the failure lands as an assistant that has gone vague rather than as an error anybody can see.
The ceiling is ours, not the caller's, and a caller asking for more than it gets less, not more.

**The assistant-facing surface is part of the service.** Not a companion process, not a sidecar.
It ships and upgrades with everything else, so the version of it is never a question anybody has
to ask.

## How I would know it worked

I add one address and one credential to my editor, ask "why is this cookbook failing", and get
back an answer that names the actual finding, the actual version, and the other places in our
estate that have the same problem — and I can check every one of those on the screen and find
they match.

And the day somebody renames something, I find out because a build went red, not because my
assistant quietly started answering a question with nothing.

## What proves it

Almost none of this yet. That is the honest position, and the journey suite is the list: every
line above has a test, and a red one means it is still to do.

What is pinned today is only the door it will come through. A caller presenting a credential in
the request header [is treated as signed
in](internal/auth/middleware_test.go#TestRequireAuthValidBearerToken) exactly as a browser
[presenting a cookie is](internal/auth/middleware_test.go#TestRequireAuthValidCookie), one whose
credential has run out [is not](internal/auth/middleware_test.go#TestRequireAuthExpiredSession),
and a caller reaching for something above their level of access [is
refused](internal/auth/middleware_test.go#TestRequireRoleDenied) — which is the mechanism that
makes "a credential is a person, at their level of access" true rather than aspirational.

That an address nobody serves is reported as not existing, rather than as existing but
unavailable, [is pinned](internal/webapi/router_api_routing_test.go#TestUnroutedAPIPath_IsNotFoundNotMethodNotAllowed).
That matters more here than it looks: an assistant working from a description will ask for things,
and the difference between "there is no such thing" and "there is, but not like that" is the
difference between it correcting itself and it insisting.

**Nothing proves the answers are any good.** That an assistant given this access produces a
diagnosis an engineer would act on is not a property any test can hold. It depends on the model,
on the question, and on whether what we expose is the part that matters. Only using it settles
that, and it may turn out that what is missing is not access but the shape of what is behind it.

**Nothing proves it works with any particular assistant.** Every one of them is differently
opinionated about how it discovers and calls things, and a description that satisfies the
standard on paper can still be unusable by the tool somebody actually has. That is found out by
plugging a real editor into it, not by a test.

**Nothing proves the credential does not leak.** Editor tooling copies configuration into places
neither I nor this service can see. Being able to destroy a credential quickly is the mitigation,
and it is a mitigation, not a fix. One that can write is the worse loss, and the only thing
standing between a leaked one and a register full of findings nobody made is that most of mine
will be read-only and I will have chosen the ones that are not. That is a habit, not a control,
and it is the weakest part of this.

**The load-bearing assumption:** that the set of things worth asking is close to the set of things
the screens already answer. If diagnosis actually needs questions no screen asks — cutting across
cookbooks, runs and ownership in one go — then describing the existing surface is a smaller piece
of the work than it looks, and the honest thing is to find that out early rather than after the
description is complete.
