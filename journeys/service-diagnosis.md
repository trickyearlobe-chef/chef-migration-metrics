# Working out what is wrong on a box I cannot reach

**As the person supporting this service, I need enough evidence to diagnose a problem on a
machine I have no access to, gathered by somebody who is not a specialist, and safe to send out
of their organisation.**

This is the constraint that shapes everything here. The service runs inside somebody else's estate.
I reach it, if at all, through a controlled desktop with no copy and paste and no file transfer.
The person in front of it can click things and send me a screenshot. "Can you run this command
and paste the output" is not available, and neither is a support engineer reading a log over
somebody's shoulder for an afternoon.

## What I need

A single action that collects everything diagnostic into one file the operator can download and
send. Not a list of instructions — one button, because every extra step is a step that gets done
wrong or not at all.

For that file to be safe to send. It leaves their organisation and it may pass through channels
neither of us controls, so it must not carry their server names, their organisation names or
their credentials. What is wrong with the software is almost never bound up with who they are.

Enough in it to actually diagnose without a second round trip. Versions, configuration with the
secrets removed, recent errors, the shape of the data — how many of what — and the health of the
host.

To read the logs from the interface, filtered, because most questions are answered by the last
few errors and a screenshot of those is often the whole diagnosis.

To see where the service is spending its time when the complaint is that it is slow, since "slow"
at estate scale and "slow" in a lab are different problems.

## The decisions behind it

**The bundle is anonymised by default and identifying information is opt-in.** The safe thing has
to be the default, because the operator sending it is not the person who will be blamed if it
carries something it should not. Somebody who genuinely needs real names can ask for them
deliberately.

**A missing part of the bundle does not fail the bundle.** If one source of information is
unavailable — that is often the very fault being diagnosed — the rest is still collected. A
diagnostic tool that refuses to produce anything when something is broken is useless precisely
when it is needed.

**Logs are readable through the interface, not only on the host.** Requiring host access to read
a log means the diagnosis cannot happen.

**Assume what we log is widely readable.** This deployment ships its logs to a shared system that
many people in the organisation can read. Anything written to a log should be treated as published
inside that organisation, which is a reason to log carefully rather than a reason to log less.

## What proves it

Anonymisation by default is pinned by the case that matters: with nothing asked for, a real
organisation name [does not appear in the
bundle](internal/webapi/handle_admin_diagnostic_test.go#TestHandleDiagnosticBundle_OrgAnonymisation).
The bundle [requires
authentication](internal/webapi/handle_admin_diagnostic_test.go#TestHandleDiagnosticBundle_AuthRequired),
so it is not a way for anybody to extract the estate's shape.

Degrading rather than failing is pinned twice, and this is the property the journey depends on
most: a source that errors [still yields a
bundle](internal/webapi/handle_admin_diagnostic_test.go#TestHandleDiagnosticBundle_SourceError), and
so does [being unable to inspect the running
process](internal/webapi/handle_admin_diagnostic_test.go#TestHandleDiagnosticBundle_NoProcessOutput).
The ordinary case [produces a complete
bundle](internal/webapi/handle_admin_diagnostic_test.go#TestHandleDiagnosticBundle_BasicBundle), and
[carries the shape of the data when it is
available](internal/webapi/handle_admin_diagnostic_test.go#TestHandleDiagnosticBundle_WithDepthStats).
The estate summary inside it is pinned including [the empty
case](internal/webapi/handle_admin_diagnostic_test.go#TestBuildBundlePlatformDistribution_Empty), so a
new installation produces a valid bundle rather than an error.

Host health figures that go into it are real measurements rather than placeholders — [the
snapshot](internal/syshealth/syshealth_test.go#TestSnapshot_ReturnsValidStats) — and log retention is
pinned so that reading logs from the interface has something to read and does not grow without
bound: [a nonsensical retention period is
refused](internal/datastore/datastore_test.go#TestPurgeLogEntriesOlderThanDays_InvalidRetention) and
purging [only drops days that have fully
expired](internal/datastore/log_entries_partition_functional_test.go#TestLogEntryPartitions_PurgeDropsOnlyFullyExpiredDays)
rather than taking part of the period somebody asked to keep.

**Nothing proves the bundle is free of identifying information.** One test asserts that
organisation names are absent. Machine names, addresses, repository addresses and user names are
not covered, and the promise the journey makes is about all of them. Anybody adding a new section
to the bundle can put identifying data in it and every test here will still pass. This is the most
consequential untested claim in the product: the failure sends an organisation's internal names
outside it, and it is unrecoverable once sent.

**Nothing proves no secret reaches a log.** Stated in [credentials that never leave the box in the
clear](service-secrets.md) and repeated here because the log path is where it would happen.

**Nothing proves the bundle is sufficient.** Whether it actually answers a real support question
without a second round trip is only established by using it in anger, and it is the thing the whole
journey is for.

**The load-bearing assumption:** that anonymisation is applied where the bundle is assembled rather
than by each section. If each section is responsible for cleaning itself, then the guarantee is only
as good as the newest section, and the test above will keep passing while the promise quietly stops
being true.
