# Testing this before it is trusted

**As the tester engaged to assess this, I want to point my tools at it primed by its own
description, and have that description be the truth — so that when I report coverage, the
number means something, and when I report nothing, that is a finding rather than a gap in
what I was given.**

I am not reading the source. I get a running instance, an account, and whatever the product
says about itself. Everything I test comes from that. If the description is a subset of the
real surface, my report is a subset too, and the part I never saw is the part that ships.

## What I need

The whole surface, from the service itself. Not a page somebody maintains — the document the
service generates, fetched from the instance I am testing, so it describes the build in front
of me and not the one before it. Anything reachable that is missing from it is untested, and
neither of us finds out.

What each call takes. My tools work by sending the wrong thing: the field that should be a
number as a string, the field that does not exist, the required one left out, the string that
is a megabyte long. To do that I have to know what the right thing was.

What each call needs to be allowed. The access level per call, and the truth of it — including
where a call is refused inside the handler rather than at the door. That is the test I care
most about: whether a low-privileged account reaches something it should not. If the document
says viewer and the service says operator, I waste a day; if it says viewer and the service
lets a viewer through when it should not, that is the finding, and I can only see it by
comparing what I was told against what happened.

A refusal that is a refusal. If I send a field this thing does not understand and get back a
200, I cannot tell what it acted on and what it dropped. Silent tolerance is not robustness —
it is a service that cannot tell me what it did, and neither can I.

Failures that give me nothing. An error should tell me it failed and not what it is made of.
No stack traces, no query fragments, no internal hostnames, no paths on disk.

An account I can hold down to a level. To test whether a viewer can reach an admin call, I need
a viewer credential that is genuinely a viewer, including when it is used by a program rather
than a person.

To know what I am allowed to break. Whether the instance I am hitting is a copy or something
somebody depends on, and whether the destructive calls are real. Fuzzing a surface that can
delete an estate is a different engagement from fuzzing one that cannot.

## The decisions behind it

**The description is generated from what is served, so testing against it is testing against
the product.** This is the whole reason an assessment driven from it is worth anything. A
hand-written document would make my coverage a measure of somebody's diligence rather than of
the software — see [building against this from the outside](api-integration.md).

**The access level is folded in from both places it is enforced.** A route's wrapper covers
every call on it; a handler can require more for one method than another. A document showing
only the first understates more than fifty calls, and every one of those is a false finding I
would have to withdraw.

**A credential carries its account's level and no more**, so a low-privileged test account
stays low-privileged when a program uses it — see [building against this from the
outside](api-integration.md).

**What the description says a call accepts is what the service reads.** A document that lists
fields the service ignores sends me hunting for an injection into something that was never
read.

## How I would know it worked

I generate a client and a fuzzing corpus from the description, run it, and every refusal and
every acceptance is one the description predicted. Where they disagree, that is my report.

## What proves it

The description covers every address the service serves, in both directions — nothing served
is [missing from it](internal/webapi/router_routes_test.go#TestOpenAPI_DescribesEveryServedAddress),
and nothing in it [is unserved](internal/webapi/router_routes_test.go#TestOpenAPI_DescribesNothingItDoesNotServe).
The addresses a subtree dispatches internally cannot [be left
undeclared](internal/webapi/router_routes_test.go#TestOpenAPI_NoSubtreeIsLeftUndeclared), which is
where a surface hides.

The access level in the document is the level enforced, [probed against the running
service](internal/webapi/openapi_roles_test.go#TestOpenAPI_DescribedRoleIsTheRoleEnforced) rather
than asserted.

A credential is held to its account's level: it [reads as that
account](internal/webapi/credential_scope_test.go#TestCredentialReadsAsTheAccountItBelongsTo),
[cannot reach above
it](internal/webapi/credential_scope_test.go#TestCredentialCannotReachAboveItsAccountsRole), a
read-only one [cannot write
anywhere](internal/webapi/credential_scope_test.go#TestAReadOnlyCredentialCannotWriteAnywhere), and
a destroyed one [is refused at the
door](internal/webapi/credential_scope_test.go#TestADestroyedCredentialIsRefusedAtTheDoor).

The rest is this journey's suite, and most of it is red.

**Nothing proves a refusal is refused rather than ignored.** Measured: every call that reads a
JSON body accepts fields it does not understand and answers as though it applied them. So a
caller who misspells a field is told it worked. This is the first thing a fuzzer finds and the
hardest to argue is harmless, because neither side can say afterwards what the service acted
on.

**Nothing proves an error gives nothing away.** Failures are a consistent shape, which is not
the same as being free of internals. No test asserts the absence of a path, a query fragment or
a hostname from a failure — and this deployment ships its logs somewhere widely readable, so
what a failure says is not the only place it says it.

**Nothing proves the description lists only fields the service reads.** Some calls are
described by reflecting the whole internal type, which can advertise fields the service never
looks at — a false lead for anybody testing what an input reaches.

**Nothing proves any of this survives a fuzzer, because one has never been run.** No load, no
malformed input at volume, no long strings, no deep nesting. Everything here has been exercised
by hand, by somebody sending what the service expected.

**The load-bearing assumption: the instance under test is a copy.** The destructive calls are
real and there is nothing in the product that makes a test instance safe to break. That is an
arrangement between people, and no test can hold it.
