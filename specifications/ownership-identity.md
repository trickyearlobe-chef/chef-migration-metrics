# Ownership — Identity and Alias Management

## TL;DR

An owner is a person or team; an **alias** is something else they are called — an email address, a
git commit name or address, a username, or the raw string an ownership source used. Aliases are
what let an import, a git history or a signed-in session resolve to an owner.

This part covers the three things that make a wrong owner correctable: editing a person's aliases,
finding the people who may already be somebody else, and folding one owner into another.

Related: [ownership-owner-model.md](ownership-owner-model.md) (the owner and alias records),
[ownership-intake.md](ownership-intake.md) (how an import resolves a raw string to an owner),
[ownership-api.md](ownership-api.md) and [ownership-api-2.md](ownership-api-2.md) (the rest of the
web API).

---

## Why correction has to be easy

An imperfect owner is acceptable provided the clue survives, because people are good at getting to
the right person from a recognisable one. Correction is therefore **deferred to the point of use**,
not done at ingest: the moment somebody recognises "Fat Tommy" as Thomas Smith is when they are
looking at who owns something, not when twenty thousand rows are being loaded.

That model only works if correcting is a small action at that moment. It also has to *stick*: a
correction that the next scheduled ingest undoes is worse than none, because it looks fixed.

## What makes a correction durable

**The alias is the durable part, not the assignment.** Moving somebody's work to the right person
leaves the original owner and its aliases in place, so the raw string from the source still
resolves to the owner the work was moved off and the next ingest puts it straight back. See
[ownership-intake.md](ownership-intake.md) § Re-ingesting the same source.

A merge is therefore defined as moving **both**: the work and every identity the source owner
answered to.

**A merged-away owner's own name is kept as an alias of the survivor.** Once the owner record is
gone, a source naming it would otherwise create the person again. Seeding it as a `custom` alias
against the surviving owner is what closes that loop.

## Alias types

The alias types are fixed by the database: `email`, `git_email`, `git_name`, `username`, `custom`.
The authoritative list is the CHECK constraint on `owner_aliases.alias_type`; the API and UI must
offer exactly these. **A SAML subject is not among them** — it is refreshed on every login and
appears in no other system, so it can never join to anything. Identity anchors on username and
email.

**This list is known to be the wrong shape.** See *Proposed: shape is not source* below.

---

## Proposed: shape is not source

> **Status: proposed, not built.** Everything above describes what ships. This section is a
> design change raised 2026-08-02 after the model was exercised against real data.

### The conflation

`alias_type` carries two unrelated ideas at once:

| value | what shape the identifier is | where we learned it |
|---|---|---|
| `email` | an address | typed in, imported, or a contact field |
| `git_email` | an address | git commit history |
| `git_name` | a person's name | git commit history |
| `username` | a login name | intended: SSO |
| `custom` | not determined | the ownership import's raw string |

Two of the five are shape-and-source compounds, and `custom` means "source: import, shape:
unknown". Meanwhile `source` already exists and already records provenance. **Provenance is held
in two columns, inconsistently.**

### What the conflation costs

Each of these was observed, not predicted.

- **Uniqueness includes provenance.** The constraint is on `(alias_type, alias_value)`, so one
  address can be claimed by two different owners under two types. Resolution tries `email` before
  `git_email`, so the second owner becomes unreachable by that address and nothing reports it.
- **Resolution order is a confidence ranking expressed as a type list** — the raw import string
  first, a commit address last. That is a statement about how far each *source* is trusted.
- **Questions about shape must enumerate types.** The email-localpart signal asks for
  `alias_type IN ('email','git_email')` — meaning "the address-shaped ones". Any new
  address-bearing source silently falls outside it.

### Shape is not enforced at the source

**This is the constraint that shapes the fix.** No source guarantees that a value has the shape
its field implies:

- Git does not validate `author_email`. Real commit history in this project carries an address
  with no top-level domain, tens of commits behind it.
- A SAML deployment can point the identity attribute at the wrong claim — a display name in the
  email attribute — and nothing downstream would know.

So **what the source asserted and what the value looks like are different facts, and either can be
wrong.** Deriving shape from the value is no more authoritative than trusting the field it arrived
in.

### The model that follows

- **Record what the source asserted** — the field it came out of, not a conclusion about it.
- **Record where it came from**, at a granularity that names the field: git commit author address,
  a specific SAML attribute, an import column, an owner's contact field, a person typing it, a
  merge.
- **Treat the observed shape as an observation**, free to disagree with the assertion. A
  disagreement is diagnostic, not an error: identities arriving from an address field that do not
  look like addresses are how a misconfigured claim mapping is found. This is the same class of
  problem as an unset `username_attr`.
- **Confidence is a property of the source**, ranked in code, not a column and not a type list.
- **Uniqueness keys on the identifier value alone**, canonicalised. One string belongs to one
  person, whatever any source believes it to be. A second claim on the same string is then a
  visible conflict — and a conflict is a strong duplicate-person signal, which is exactly what the
  current model discards.

Canonicalising on write would also remove the raw-then-lowercase retry the import resolution does
today, which exists only because the column has no case-insensitive collation.

### Open questions

- **The import's raw string genuinely has no known shape** — sometimes an address, sometimes a
  username, sometimes a team. Under this model it is recorded as asserted-unknown rather than
  forced into a type.
- **Migration collisions.** Collapsing the address types into one namespace means any string
  currently held twice must resolve to one owner. Those are duplicate people, so the migration
  must report them for a human rather than silently picking a winner.
- **Timing.** Node and repo matching consume this resolution chain, so building them first means
  revisiting them.

## Where aliases are edited

**On the owner's own page.** What somebody owns and what they are called elsewhere are different
questions, and both belong on the person. Editing there takes no owner field — the page is already
the owner.

The global alias screen remains as a secondary admin view for searching across every recorded alias
and for bulk import. It is not the place a single person's aliases are maintained.

**Choosing an owner is a search over owners, not over aliases.** An owner that has no alias yet is
exactly the one somebody arrives needing to correct, and an alias search cannot find it.

## Aliases are seeded, never inferred

Nothing populates aliases by guesswork. Two paths seed them from a source that genuinely carries
the identity:

- **Ownership import** seeds the raw owner string as a `custom` alias against the owner it created
  ([ownership-intake.md](ownership-intake.md)).
- **Committer assignment** seeds the commit address as a `git_email` alias against the owner it
  created. Recorded as a git address because that is what it is: a commit address is not a
  corporate address, and treating them as the same silently attaches work to the wrong person.

A failed seed costs later matching accuracy, not the operation that triggered it — the alias
uniqueness constraint is global, so a value already held by somebody else simply is not seeded.

**Every alias records its `source`**, so an automatic one can be told from a human's and reversed.

## Possible duplicate owners

Inventing people on import is deliberate — it is where alias candidates come from — so the
catalogue accumulates near-duplicates. The standing report pairs each owner with who they might
already be, so a duplicate can be recognised without already suspecting it.

**Pairs are leads, never matches.** Nothing is merged without a person deciding.

**Two signals, both reported with the text that matched:**

- **Owner name similarity**, which covers every owner including those carrying no alias at all.
- **Alias value similarity**, which catches people whose names share nothing but who are recorded
  under near-identical identities.

A pair is reported **once**, under its strongest evidence, whichever signal produced it.

**Each side carries how much work it holds**, because that is what decides which way round to
merge — and the merge is offered in the direction that moves the least.

### The report is scanned, not computed on read

Comparing every owner with every other owner does not finish: owner names cluster hard — people
share surnames, and committer-derived names are email localparts — so the work is quadratic in that
density rather than in the row count.

The scan is therefore **bounded to the nearest few candidates per owner and per alias**, run away
from the request that reads the list, and its result stored. Consequences that are part of the
contract:

- **The list is as old as the last scan**, and the page says when that was.
- **"Nothing looks alike" and "nobody has scanned yet" are different messages** and must not both
  render as an empty list.
- **One scan runs at a time.** A second request while one is running is accepted and does nothing.
- **A person with more near-twins than the bound keeps only the closest few.** This is a bound, not
  completeness — the twentieth-best guess is noise in a list somebody reads.
- **A merged-away owner's pairs disappear with it**, without waiting for the next scan.

## Coverage is stated, not implied

The report says how many owners have no alias recorded. Those owners are still compared, but only
under the one name they were created with. Silently omitting part of the catalogue from a duplicate
report is worse than not having one.

## Merging is answerable for

A merge deletes a person, so it requires the same role as deleting an owner outright, and it writes
an `owner_merged` audit entry naming the surviving owner and carrying what moved.
