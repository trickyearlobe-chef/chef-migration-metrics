# Getting in, and never being locked out

**As the administrator running this service, I need people signed in through the company's
identity provider with the right level of access, and I need to be certain that I myself can
always get back in — because this runs on a machine I may not be able to reach.**

The second half is the part nobody writes down until it has happened. This is installed inside
somebody else's estate, reached through a controlled desktop, sometimes with no console access
at all. A configuration mistake that makes the service refuse to start is not an inconvenience
there; it is an outage I cannot fix.

## What I need

People signing in with their existing company credentials, not a separate password to manage.
Their level of access decided by the identity provider, so that when somebody changes role or
leaves, that is handled where it is already handled and not in a second list here that nobody
remembers to update.

A first administrator on a brand-new installation, before there is anybody to grant anything.

Somebody who has never signed in before to become a user on their first successful sign-in,
without me provisioning them by hand.

Local accounts as well, because the identity provider is exactly what is unavailable on the day
I most need to get in.

Somebody who guesses passwords to be shut out, without that becoming a way to lock a real
administrator out on purpose.

## The decisions behind it

**The identity provider decides who somebody is and what they may do; we do not second-guess
it.** If it says viewer, they are a viewer, and it stays that way on the next sign-in even if
something here was edited in between. Two systems both believing they own access rights is how
people end up with rights nobody granted.

**A person is anchored on their company username, not on whatever token the sign-in produced.**
Some identity providers issue a different opaque identifier on every single sign-in. Anchoring
on that would create a new user, with no work attached, every time somebody logged in. The
username is the thing that persists, and it is also what ownership is keyed on — see [one
person, many names](ownership-identity.md).

**A sign-in must carry a name to sign in as, or it is refused.** With no name the only thing
left is the opaque token, and taking it would coin a person out of something that is not a name.
Refusing is the whole of the fix: it removes the possibility rather than detecting it afterwards.
Say which claim is missing, because the fix belongs with whoever configures the provider.

**An email address is not required.** Somebody arriving without one signs in and works
normally — it only means there is less to match them to the commits they wrote, and their
sign-in name can do that job on its own. Do not turn this into a second reason to refuse.

Refusing is safe only because there is always a local administrator: that is what local
accounts are for, and it is why they are not optional.

**Anybody arriving without an explicit level of access gets the lowest one.** Never the highest,
never nothing, and not a failure — the safe default is "can look, cannot change".

**A misconfiguration must degrade the service, never prevent it starting.** A certificate that
is named but missing does not stop the service; it falls back — a certificate we generate
ourselves, and failing that, an unencrypted connection. An unencrypted service that I can reach
and fix is strictly better than an encrypted one I cannot, because from the second one there is
no route back. This is deliberately not a setting; it is the behaviour, because a setting to
control it would itself be somewhere to make the mistake.

## What proves it

The anchoring decision is pinned where it would fail: a sign-in that carries no stable subject
[is refused rather than creating
somebody](internal/auth/jit/provisioner_test.go#TestProvision_EmptySAMLSubject), a person who
already exists [is recognised rather than
duplicated](internal/auth/jit/provisioner_test.go#TestProvision_ExistingUser), and a first-time
arrival [becomes a user](internal/auth/jit/provisioner_test.go#TestProvision_NewUser). Somebody
arriving with no level of access stated [becomes the lowest
one](internal/auth/jit/provisioner_test.go#TestProvision_DefaultsRoleToViewer), Where the provider
supplies no username, what happens instead is pinned twice — [a name is derived from what did
arrive](internal/auth/jit/provisioner_test.go#TestProvision_FallbackUsername) and [the opaque
identifier stands in when the attributes are
absent](internal/auth/samlsp/provider_test.go#TestExtractUserInfo_FallbackToNameID). Names are
[made safe before being
stored](internal/auth/jit/provisioner_test.go#TestSanitiseUsername).

How the provider's answer becomes a level of access is pinned by [role
resolution](internal/auth/samlsp/provider_test.go#TestResolveRole), with the details of a
sign-in [read out of the
assertion](internal/auth/samlsp/provider_test.go#TestExtractUserInfo).

The anti-lockout behaviour is pinned at the point it matters: a configuration naming a
certificate file that does not exist [is not treated as
fatal](internal/config/config_test.go#TestValidation_TLSStaticCertNotFound_NotFatal). Genuinely
invalid settings are still refused — [an unknown encryption
mode](internal/config/config_test.go#TestValidation_InvalidTLSMode), [a mode requiring a
certificate with none
named](internal/config/config_test.go#TestValidation_TLSStaticMissingCertPath) — so the
tolerance is specifically for "the file is not there", which is the mistake that actually
happens on somebody else's machine.

Locking out password guessing is pinned [when the threshold is
reached](internal/auth/local_test.go#TestAuthenticateTriggersLockout) and [beyond
it](internal/auth/local_test.go#TestAuthenticateLockoutExceedsThreshold), and a failure of the
lockout machinery itself [does not become a way
in](internal/auth/local_test.go#TestAuthenticateLockoutLockUserFails).

**Nothing proves that a person is never coined out of an opaque token.** The decision above
says a first sign-in with nothing to anchor on is refused; what is pinned is the opposite, and
the two tests naming it say so plainly. Until that changes, somebody can still arrive as a
string that is not a name, and their work will hang off it.

**Nothing proves the fallback ladder end to end.** That a missing certificate is tolerated is
asserted; that the service then actually binds a self-signed listener and, failing that, an
unencrypted one, is not. The ladder is the whole anti-lockout promise and it is the least
tested thing in this journey. It has been exercised by hand, not by a test.

**Nothing proves a real identity provider works.** Assertions are parsed from fixtures. Every
provider is differently wrong in practice — clock skew, attribute naming, how sign-out is
handled — and only a live one settles it.

**The load-bearing assumption:** that the identity provider does not reissue a username to a
different human being. Access, ownership and accountability all hang off that single string.
Nothing here could detect a violation; it would simply hand one person another person's work.
