# Credentials that never leave the box in the clear

**As the administrator setting this up, I need to give the service the keys and passwords it
needs to reach Chef servers, databases and hypervisors, and be able to say honestly — to a
security review that will ask — that none of them can be read back out.**

This service is only useful because it holds credentials for a lot of other systems. That makes
it worth attacking, and it makes it something somebody has to sign off. "Where are the passwords
and who can see them" is the first question, and the answer has to be better than "in a file on
the host".

## What I need

To enter a credential once, through the interface, and have it stored encrypted. Never typed
into a screen that is doing something else, like setting up an import.

To use a stored credential by naming it, so the thing that needs it never handles the secret
itself.

To be able to see what credentials exist, what they are for and when they were last changed,
without any path that returns the secret.

To replace one when it rotates, and delete one when the system it reaches is gone.

To know that a backup or a diagnostic bundle of this service does not carry them out in the
clear.

## The decisions behind it

**A secret is write-only from the outside.** It goes in and it is used internally; there is no
read-back, not for administrators either. An interface that can show a secret is an interface
that can be made to show a secret.

**Listing and reading are different operations with different results.** The general read path
that the rest of the product uses cannot return a secret at all — not encrypted, not masked,
absent. Relying on every caller to remember to exclude them is a promise that somebody eventually
breaks.

**Encryption is bound to what the value is for, not just to the key.** A stored value that could
be lifted from one place and made to decrypt in another has not really been protected — it has
been obfuscated. So the identity of the thing being encrypted is part of the encryption, and a
value moved somewhere it does not belong does not decrypt.

**The same value encrypted twice does not produce the same stored bytes.** Otherwise anybody who
can see the store can tell which credentials are identical, which is information they should not
have.

**The key that decrypts all of this cannot live in the thing it decrypts.** It is one of the two
values that must come from outside the interface — see [changing how it behaves without taking it
down](service-configuration.md).

## What proves it

The separation of listing from reading is pinned in both directions, and it is the property most
likely to be broken by a well-meaning change: the general read path used across the product
[excludes secrets entirely](internal/configstore/configstore_test.go#TestGetAllExcludesSecrets),
and the secret path [returns only secrets and not ordinary
settings](internal/configstore/configstore_test.go#TestGetSecretOnlyReturnsSecrets). Listing
[returns descriptive information](internal/configstore/configstore_test.go#TestListReturnsMetadata)
rather than values.

The encryption properties are pinned directly: a value [survives a round
trip](internal/configstore/configstore_test.go#TestEncryptDecryptRoundTrip), the same value
[encrypts to different stored bytes each
time](internal/configstore/configstore_test.go#TestEncryptProducesDifferentCiphertextPerWrite), and
a value [does not decrypt when it is not where it
belongs](internal/configstore/configstore_test.go#TestDecryptFailsWithWrongAAD) — that last one is
what makes the binding real rather than decorative.

The ordinary operations are pinned too: an unknown name [is not
found](internal/configstore/configstore_test.go#TestGetNonExistentKey) rather than returning
something empty, deletion [is safe to
repeat](internal/configstore/configstore_test.go#TestDeleteIdempotent), and credentials
[are reachable through the adapter the rest of the product
uses](internal/configstore/credential_adapter_test.go#TestCredentialAdapter_FullLifecycle).

That a diagnostic bundle does not carry identifying information out is pinned by [the
anonymisation
contract](internal/webapi/handle_admin_diagnostic_test.go#TestHandleDiagnosticBundle_OrgAnonymisation).

**Nothing proves a secret never reaches a log.** The store will not hand one back, but a value in
memory can be printed by any code holding it. No test asserts the absence of a secret from log
output, and logs are routinely shipped somewhere a lot of people can read — so a single careless
line would expose a credential widely and quietly. This is the most serious untested claim in
this journey.

**Nothing proves what happens when the key changes.** Rotating the key that decrypts everything is
not covered here. Assume it is a manual operation with no rehearsal behind it.

**Nothing proves a backup is safe to hand over.** Backups are asserted to be complete and
verifiable, not to be free of credential material — see [not losing
it](service-continuity.md). Treat a backup as sensitive.

**The load-bearing assumption:** that the encryption key is supplied to the service in a way that
does not itself write it down somewhere readable. Everything above is defeated by a key sitting in
a world-readable file, and nothing in this product can check that.
