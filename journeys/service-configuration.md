# Changing how it behaves without taking it down

**As the administrator running this service, I need to change how it works from the screen in
front of me and have the change take effect, because I cannot get a shell on this machine and
even if I could, a restart means losing a collection cycle.**

Everything about this deployment is somebody else's estate. Getting a file edited on the host
means a change request, a window, and a person with access I do not have. If a setting can only
be changed that way then in practice it cannot be changed.

## What I need

Every setting reachable from the interface: what we collect and how often, which Chef servers,
the version we are moving to, how much work runs at once, how noisy the logging is, how long
things are kept, what the analysis tools do, how exports behave, how names are displayed.

Changes to take effect now. Not on the next restart — now, on the next cycle, on the next
request.

To be told when I have typed something impossible, at the point I type it, rather than finding
out because collection stopped overnight.

For a bad setting not to take the service down with it.

## The decisions behind it

**Configuration lives in the database and is edited through the interface. That is the whole
model, not the convenient case.** The only legitimate exceptions are the two values that unlock
the database itself: how to reach it, and the key that decrypts what is stored in it. Those
cannot live in the thing they unlock.

**There are settings in the code that can be supplied by the environment, and they are not a
precedent.** They are older than the model and only survive because removing them has not been
worth the risk. A field being declared as configurable in code is not evidence that it can be
set — that question is answered by whether it is wired into the store. If a setting is
unreachable, the answer is to wire it in or delete it, never to document an environment
variable.

**Nothing caches a setting at startup.** A component that reads its configuration once and holds
it is indistinguishable from a component that requires a restart, and it will be discovered by
somebody changing a value and watching nothing happen. Values are read live, every time.

**A rejected change leaves the running configuration alone.** The previous good configuration
keeps running. The alternative — a service that stops working because somebody typed a letter
into a number field — is the lockout failure again, arriving by a different route.

## What proves it

Live change is pinned: a reload [swaps the running
configuration](internal/configstore/reloader_test.go#TestConfigHolder_Reload_SwapsConfig), and
readers [get the current one rather than a
copy](internal/configstore/reloader_test.go#TestConfigHolder_GetReturnsSamePointer) so nothing
can be holding a stale snapshot. The settings that decide what the service listens on [come from
the database](internal/configstore/reloader_test.go#TestConfigHolder_Reload_SourcesListenFromDB),
which is the case that proves the model reaches even the values you would expect to be fixed at
startup.

Surviving a bad change is pinned twice, and these are the load-bearing ones: [a validation
failure preserves the running
configuration](internal/configstore/reloader_test.go#TestConfigHolder_Reload_ValidationFailurePreservesConfig),
and [a database error during reload does
too](internal/configstore/reloader_test.go#TestConfigHolder_Reload_DatabaseError). Reloading
[applies defaults](internal/configstore/reloader_test.go#TestConfigHolder_Reload_AppliesDefaults)
so a partially filled record cannot produce zero values that read as deliberate choices, and
[repeated reloads are safe](internal/configstore/reloader_test.go#TestConfigHolder_MultipleReloads).
Reading while a reload is in progress [is safe
too](internal/configstore/reloader_test.go#TestConfigHolder_ConcurrentReloadAndGet), which
matters because live reads are the entire point.

Impossible values are refused at the point they are set, pinned across the settings where a
wrong one does real damage — [an unknown encryption
mode](internal/config/config_test.go#TestValidation_InvalidTLSMode), [a retention period that
makes no sense](internal/datastore/datastore_test.go#TestPurgeLogEntriesOlderThanDays_InvalidRetention) —
and schedules are [parsed and rejected up
front](internal/collector/scheduler_test.go#TestParseSchedule_InvalidExpressions) rather than
silently never firing.

**Nothing proves every setting is actually reachable.** This is the gap most likely to bite. The
model says all of it is editable from the interface; what is asserted is that the mechanism works
for the settings that have tests. A field that exists in the code, is declared as configurable
and is wired to nothing would pass everything here and simply do nothing when set. That is not a
hypothetical failure mode — it is the reason the rule above says a declaration in code is not
evidence.

**Nothing proves a change reaches every component live.** The mechanism is proven; each
component's use of it is not. A component that took a copy at construction time would pass all
of the above.

**The load-bearing assumption:** that the two bootstrap values really are the only exceptions.
Every additional thing that can only be set outside the interface makes the deployment less
operable by the person who has to operate it, and each one arrives looking reasonable on its own.
