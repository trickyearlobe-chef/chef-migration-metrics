# What this is for

An organisation runs tens of thousands of servers managed by Chef. They are on an old
version and need to be on a new one. Nobody can say what is stopping that, how far along
they are, or who has to do the work — so the upgrade stalls, and the reason it stalls is not
technical. It is that the estate is unreadable.

This makes the estate readable. It collects what is actually deployed, works out what will
break on the target version, ranks it by what it costs against what it unblocks, attributes
it to the people who own it, and tracks it as it gets fixed.

Everything here serves one question: **what is stopping us moving to the target version, and
what should we do first?**

There is exactly one target version at a time. Not a matrix, not a per-user selector — one,
set centrally.

## Who this is for

- **The migration lead** — answers to a programme board and needs to know where the estate
  stands and whether it is moving.
- **The engineer doing the work** — needs a short, correct list of things to fix. Their time
  is the scarce resource, so a list padded with false alarms is worse than no list.
- **The cookbook owner** — needs to know which of this is theirs, and nothing else.
- **The administrator** — connects this to the Chef servers, the git repositories and the
  identity provider, and keeps it running somewhere they usually cannot reach directly.

## The journeys

| Journey | The question it answers |
|---|---|
| [Knowing where the estate stands](estate-progress.md) | How far through are we, and is it moving? |
| [Which machines can move](node-readiness.md) | What is blocking this server, and is it even still there? |
| [The one fix that unblocks thousands](role-impact.md) | Where does one change buy the most? |
| [Whether a cookbook survives](cookbook-compatibility.md) | Will this break, and can I believe the answer? |
| [Deciding what to fix first](remediation-priority.md) | Of everything broken, what do I start on? |
| [Naming a slice of the estate](named-cohorts.md) | Can I get back the selection I built yesterday? |
| [Trusting what the scan says](scan-trust.md) | Does this red mean anything, or is it tidying? |
| [Proving a cookbook really runs](converge-testing.md) | Does it work on a real machine, or just read clean? |
| [Saying so when the machine is wrong](human-verdict.md) | I have seen this run — where do I say so? |
| [What actually happened out there](run-history.md) | Which machines are failing, and on what? |
| [Who has to do the work](ownership-attribution.md) | Whose is this, and what has nobody claimed? |
| [Getting ownership in](ownership-intake.md) | How do I load who owns what, and keep loading it? |
| [One person, many names](ownership-identity.md) | Is this the same engineer written down twice? |
| [Asking my assistant why this is failing](agent-access.md) | Can the AI in my editor read what this knows? |
| [Building against this from the outside](api-integration.md) | Can I write a program against it, and is it documented? |
| [Testing this before it is trusted](security-assessment.md) | Can somebody assess it from what it says about itself? |
| [Getting in, never locked out](service-access.md) | Who may use it, and can I always get back in? |
| [Changing my own password](own-password.md) | Can I change it without asking somebody? |
| [Changing it without downtime](service-configuration.md) | Can I change a setting from the screen? |
| [Finding the setting I came for](admin-navigation.md) | Where is it, and will I find it again? |
| [Credentials that stay secret](service-secrets.md) | Where are the passwords, and who can read them? |
| [Letting it reach our git servers](service-git-access.md) | Which key do I paste in, and is that really them? |
| [Keeping the data flowing](service-collection.md) | Is it still collecting, and would I know? |
| [Diagnosing a box I cannot reach](service-diagnosis.md) | What do I send support, and is it safe to send? |
| [Not losing it](service-continuity.md) | Can I restore, and can I upgrade without loss? |
| [Working all day](working-all-day.md) | Can I just get on with it for eight hours? |

Every route in the application maps to a journey above. [Working all day](working-all-day.md)
has no route of its own — it is
the journey that holds what must be true on **every** screen, so a rule binding all of them has
somewhere to live other than an arbitrary one.

**Some capabilities have no route at all**, and are covered only in prose inside these journeys:
certificates renewing themselves, collection running on a schedule, receiving pushed telemetry,
retention and purge, installation and upgrade, and the fallback that stops a bad setting locking
everybody out. No walk through the navigation would find them, which is why route coverage is a
floor and not the measure.

## How to read a specification here

Every file is a journey: who the person is, what they are trying to get done, what has to be
true for them to succeed, and how they would know it worked — in their words.

Two things a journey also carries:

- **The decisions that bind.** Stated as decisions, so they are not silently re-litigated by
  the next person who finds them inconvenient.
- **Its own load-bearing assumption, flagged for checking.** Where a journey leans on
  something structural, it says so and tells the reader to verify before designing against
  it. A document that names the claim that will hurt you if it is wrong is worth more than
  one that reads as uniformly confident.

None of it describes how the software works. The code is the only source of truth for that,
and a contract lives in a test that fails when it stops being true.

- **Every journey names a test**, as a link that is checked when it is committed. One real
  link is enough — it is not a coverage rule.
- **And says which parts nothing can prove.** Roughly a third of what these journeys promise
  cannot carry an assertion at all: that a list is short enough to work through, that a chart
  is honest, that a number is one you would defend to a board. Naming those explicitly is
  what stops the tests that do exist reading as more than they are.

**No status claims.** Nothing here says built, shipped, planned or proposed. Status written
in prose is the thing that rots fastest. Naming a test replaces it: if the test is red the journey is not proven, and that
says so without anybody keeping a sentence up to date.

The one claim this page makes for itself, that there is a single centrally-set target version
and that it is what decides the verdicts, is pinned by [the target version
contract](internal/analysis/readiness_test.go#TestEvaluateOrganisation_TargetVersionDrivesVerdict),
with [no target version set](internal/analysis/readiness_test.go#TestEvaluateOrganisation_NoTargetVersion)
covered too, because that case must not read as everything being fine.

Conventions for writing code are in `docs/project-conventions.md`.
