# Changing my own password

**As somebody with an account on this service, I need to change my own password from my own
record.**

Today the only way one changes is an administrator setting it, so every change costs a ticket
and somebody else knows the password.

## What I need

- To change it myself, where I see the rest of my account.
- To give my current password first, so an unlocked screen is not the whole of the security.
- To be told the rules before I am told I got them wrong. A minimum length is not a secret.
- Not to be offered it if I sign in through the identity provider — I have no password here,
  and a field that cannot work sends me to a ticket anyway.
- Nothing else reaching anybody else's password, and nothing I hand to a tool changing one.

## The decisions behind it

**It belongs to the account, at every level of access.** A viewer's own credentials are
theirs. This must survive any later narrowing of what a viewer may do.

**The administrator's reset stays.** Different act, for somebody already locked out, and it
cannot ask for the old password because they do not have it. Neither replaces the other.

**Changing it ends my other sessions.** A default, not a decision: the administrator's reset
already does this, and two paths to one change behaving differently is worse than either
choice.

## How I would know it worked

I change a weak password from my own account page in under a minute and nobody else learns
what it was. Coming in through the identity provider, I am never shown the option.

## What proves it

**Almost none of it.** The suite is the list, red on purpose, starting with [nobody being able
to change their own
password](internal/webapi/own_password_journey_test.go#TestJourney_ICanChangeMyOwnPassword).

Pinned already: what a password must look like once chosen — [too
short](internal/auth/password_test.go#TestValidatePasswordTooShort), [the
boundary](internal/auth/password_test.go#TestValidatePasswordExactMinLength), [case
variety](internal/auth/password_test.go#TestValidatePasswordNoUppercase). Those apply wherever
one is set.

Green from the start and meant to stay so: [nothing serves anybody else's
password](internal/webapi/own_password_journey_test.go#TestJourney_ThereIsNoAddressForSomebodyElsesPassword),
and a [credential cannot change
one](internal/webapi/own_password_journey_test.go#TestJourney_ACredentialCannotChangeAPassword).

**Nothing proves the administrator's reset works** — the only way a password changes today has
no test against the endpoint.

**Nothing proves the field is hidden from identity-provider accounts.** The suite can only ask
whether the service reports which kind of account it is.
