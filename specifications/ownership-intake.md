# Ownership Intake — discovery-driven import

## TL;DR

An administrator brings existing ownership data into CMM from a source whose shape we do not
know in advance. CMM reads the source, shows what columns it found with sample values, lets the
administrator map those columns onto CMM's ownership fields, previews the result without writing
anything, and only then commits. The import succeeds while the source data is still inconsistent,
and reports what matched, what did not, and why.

This is distinct from the **fixed-header import** (`POST /api/v1/ownership/import`), which
requires the source to already be in CMM's own column order. That path remains in service,
unchanged. This spec adds a second path for sources we cannot dictate the shape of.

## Overview

Three stages, each its own request. Nothing is remembered between them on the server except a
saved mapping, if the administrator chooses to save one.

1. **Profile** — read the source, report its columns with sample values and fill rates. Writes
   nothing.
2. **Preview** — apply a mapping and run the full matching pipeline. Writes nothing. Returns the
   match report.
3. **Commit** — the same pipeline, and the resulting assignments are written.

Preview and commit take the same payload. That is the property that makes preview trustworthy:
an administrator who is happy with a preview can commit it without the shape of the request
changing underneath them.

## Source contract

A source is anything that can yield an ordered list of column names and then a sequence of rows
keyed by those names. CSV is the only implementation in this release. The contract is deliberately
narrower than a CSV reader so that a SQL cursor can satisfy it later without the pipeline changing.

Invariants the contract carries, and which the rest of this spec relies on:

- **Column order is preserved.** Profiling presents columns in source order, because that is how
  the administrator sees them in their own tooling. An unordered map cannot express this.
- **A source is single-pass and not re-readable.** Profile, preview and commit each open their own
  source. A future SQL source is a streaming cursor and cannot be rewound, so nothing in the
  pipeline may assume a second pass.
- **Cell values are strings.** Type interpretation is CMM's job, not the source's, and a SQL
  source returning a typed NULL and a CSV source returning an empty cell must be indistinguishable
  downstream.
- **Iteration errors are terminal and reported, never swallowed.** A source that fails halfway
  through fails the request; it does not return a short result that looks like a small file.

The authoritative contract is the `RowSource` interface in `internal/ownershipimport`. This spec
states the invariants it cannot express; it does not restate its method set.

### Delimiter detection is advisory, never binding

CSV sources may use a delimiter other than a comma. CMM guesses, and the guess is always
overridable:

- Profile, preview and commit each accept an optional explicit delimiter. When supplied it is used
  verbatim and no detection runs.
- **Consistency across lines is the discriminator, not raw frequency.** A candidate that yields the
  same field count on every sampled line beats one that appears more often but yields a ragged
  shape. Prose in a free-text column defeats a frequency count and does not defeat a consistency
  check.
- Because preview and commit each open their own source, each carries its own delimiter. It is not
  remembered from a prior profile call. A saved mapping stores its delimiter alongside its field
  map.

The cost of a misdetection is therefore one field edit, never a failed or wrong import.

## The mapping document

A mapping says how to build each of CMM's ownership fields out of the source's columns. It is JSON,
it is an API contract, and when saved it is persisted in `ownership_import_mappings` — so it
outlives the code that reads it and must be versionable as data.

Six target fields. `owner` and `entity_key` are required; the rest are optional.

```
owner           required   the owner this row assigns to
entity_type     required   node | cookbook | git_repo | role | policy
entity_key      required   the thing being owned
organisation    optional   the organisation the assignment is scoped to
notes           optional   free text carried onto the assignment
display_name    optional   the owner's human-readable name
```

Each field is specified as exactly two things: **one source**, then **an ordered chain of
transforms**. Source and transform are disjoint — nothing appears in both. That separation is what
keeps the document readable: the source answers "which cells", the transforms answer "what is done
to the text", and no transform ever reads a column.

### Source — a tagged union, evaluated once per row

```
{"kind": "column",   "column": "<header>"}
{"kind": "constant", "value": "<literal>"}
{"kind": "concat",   "columns": ["<header>", …], "separator": "<sep>"}
```

- `column` — a header absent from the source's column list is a **mapping-validation error**,
  raised at save or preview time against the whole document. It is not a per-row error: every row
  would carry it, and reporting it ten thousand times hides the one thing the administrator needs
  to fix.
- `constant` — row cells are not read at all. This is how `entity_type` is set, and how a whole
  file is attributed to one organisation.
- `concat` — the only source that reads several columns. Joined in the order given. **An empty cell
  contributes an empty segment and separators are not collapsed**, so the output is deterministic
  and a missing middle component is visible rather than silently closing up.

### `entity_type` is constrained further

Its source **must** be `{"kind": "constant"}`, and the constant must be one of the values permitted
by the live `entity_type` CHECK on `ownership_assignments`. It is an enum; per-row variation from a
column is not supported.

Anything else is a **mapping-validation error at save or preview time** — not a per-row rejection.
The per-row `invalid_entity_type` reason survives only on the fixed-header path, where the value
genuinely does come from the row and can differ line by line.

### Transforms

An ordered list, each strictly text in and text out, applied left to right. None reads a column.
The catalogue:

```
trim                 remove leading and trailing whitespace
lowercase            fold to lower case
uppercase            fold to upper case
strip_domain         take the part before "@"; IP literals are left unchanged
prefix               prepend a fixed string
suffix               append a fixed string
replace              substitute one fixed substring for another
regex_extract        yield the first capture group of a pattern
default              substitute a fixed value when the text is empty
```

Two semantics an implementer would otherwise get wrong, both pinned by tests:

- **`strip_domain` leaves an IP literal unchanged.** A host part that is an address, not a name,
  has no domain to strip, and truncating it produces a value that looks plausible and is wrong.
- **`regex_extract` yields the empty string** when the pattern does not match or captures nothing.
  It never yields the input unchanged — a silent pass-through would let unextracted raw text reach
  an owner name and create owners nobody recognises.

`default` is named that rather than `constant` deliberately, so it cannot be confused with the
`constant` **source**. A pattern that will not compile is a mapping-validation error.

## Owner-name normalisation

`owners.name` is constrained by a regex in the schema, so an arbitrary source string cannot be used
as an owner name directly. CMM slugifies it.

**Slugify is not a transform.** It is applied implicitly and unconditionally as the final step of
the `owner` field only, after that field's own transform chain. Making it a transform would let a
mapping omit it and produce a document that validates and then fails at write time against a
database constraint.

The rule, in order: fold to lower case; replace every rune outside the permitted set with a single
hyphen, without decomposing non-ASCII; collapse runs of hyphens; trim leading and trailing hyphens
and then any leading punctuation the constraint forbids in first position; reject if the result is
empty or still fails the owner-name regex.

The implementation's output is asserted against **both** the Go-side regex and the database
constraint, by a property test over arbitrary input rather than by fixed cases.

**Decided: no accent stripping, and therefore no new dependency.** Unicode decomposition would
require a module that is in neither `go.mod` nor `go.sum`, for one cosmetic transformation, and
every new dependency carries a supply-chain check. The accepted trade-off is that an accented value
folds toward hyphens rather than to its ASCII base.

**This costs nothing in matching**, because the raw string is preserved twice: as `display_name`,
and as a `custom` alias seeded against the owner. Fuzzy matching and every future import compare
against the original. The slug is only a stable, constraint-legal handle.

`display_name` receives the value **before** slugification. That is why, absent an explicit mapping,
it defaults to the `owner` field's pre-slugify output rather than to any column.

A raw string that slugifies to empty — a value that is entirely punctuation, for instance — is
rejected with its own reason, `invalid_owner_name`. It is neither a missing field nor a malformed
row, and folding it into `missing_required_field` would hide the most actionable class of import
miss behind the least actionable one.

## Resolving a person

The incoming data identifies people inconsistently — sometimes an email address, sometimes a
username, and **often just a person's name**. "Alice Smith" is as likely to appear in a customer's
ownership export as any handle, and an SSO display name looks nothing like `firstname.lastname`.
Anything built assuming a single clean identifier will be wrong on real data.

So the raw owner string is tried against the alias catalogue in this order, at its original case
and then folded to lower case (the alias table has no case-insensitive collation, while export
casing varies freely):

```
custom      what this import itself seeds — a re-run must resolve to the owners the last run made
email       a corporate address
username    a login handle
git_name    a person's name, as it appears in commit authorship
git_email   a commit address
```

`git_name` is what carries the person-name case. It is a real alias type in the schema, and it is
where a name belongs — not `custom`, which this import writes to.

**There is deliberately no bare-localpart tier.** A commit address is not a corporate address, and
the committer-assign path already forces owner names to the email localpart — so matching on
localpart alone is precisely how one person inherits another's identity.

### Where nothing resolves

Two independent signals produce candidates, and a candidate is **always** a suggestion:

- **An exact email-localpart hit across differing domains.** Git history routinely carries one
  person as `alice@corp.example`, `alice@dev.corp.example` and a noreply address. This is a much
  stronger lead than any similarity score, so it is offered first — but the same localpart under
  two domains is as often two different people, which is why it is a lead and not a match.
- **Trigram similarity** over alias values, from the existing suggestion facility. The scoring is
  not reimplemented here.

### Creating an owner

**When nothing resolves, the owner is created** from the raw string — with or without candidates —
and the raw string is kept as the display name and seeded as a `custom` alias.

The row is **never attributed to a suggested owner**: that attribution needs a human. But it is not
rejected either. Two reasons, and the second is the load-bearing one:

- Requiring owners to pre-exist makes a first import impossible, which is the whole of journey 5.
- **Adjudicating names mid-import is the expensive thing.** Correcting twenty thousand people at
  ingest is a job nobody will finish, and it blocks the data landing at all. A mistaken owner stays
  correctable long afterwards — and the moment it actually gets corrected is when somebody is
  looking at a repo and asks *"who owns this? Fat Tommy? I wonder if that's Thomas Smith."* They
  have the context then; the person running the import does not.

So an imperfect owner is acceptable **provided the clue survives**. What must never happen is a
silent guess or a discarded string, because both destroy the thing a human would later recognise.

**Similarity scoring cannot recognise a person.** A nickname shares almost no characters with the
name on the account, so no threshold will ever surface it. This is why the report lists every person
it would add as a *person* — name, the rows they appear in, and any owner they might already be —
rather than folding them into an assignment count. Someone scanning that list catches what no score
can. **Reviewing it is not a gate**; the import proceeds either way.

Preview and commit accept an optional flag that turns creation off, for a strict import against an
established owner catalogue where a new person really is a mistake. Each row reports whether
committing it would create an owner; that is orthogonal to the outcome, which describes the
assignment.

### Re-ingesting the same source

This decides whether ownership can be kept up to date on a schedule.

**The alias is what makes a correction durable.** Once the raw string resolves to the right person,
every future ingest of the same file lands on them, and the repeated assignment is a no-op.

**Reassignment alone is not durable.** Moving somebody's assignments leaves both the original owner
and its aliases in place, so the raw string still resolves to the owner the work was moved off, and
the next ingest puts it straight back. Correcting a mistaken owner therefore means **moving the
alias to the right person**, not just moving the work. Both behaviours are pinned by tests.

**Ingest is additive.** A row that disappears from the source does not remove the assignment it
created, so ownership that has been revoked at source persists until someone removes it in CMM.

## The match report

Keyed by 1-based `source_row`, carrying both the raw and the mapped values for every field, so an
administrator can see what their mapping did to each row without re-reading the file.

Per row:

```
owner_match      exact | alias | fuzzy_suggestion | unknown
entity_match     found | not_found
outcome          would_create | duplicate_exists | owned_by_other | rejected
rejected_reason  unknown_owner | invalid_entity_type | missing_required_field
                 | malformed_row | invalid_owner_name
alias_conflict        boolean
alias_conflict_owner  the owner already holding the colliding alias
owner_suggestions     at most 3 {owner_name, score}
```

Aggregate counts per outcome, the top 20 unmatched owner strings, and the whole report downloadable
as CSV. **Unmatched owner strings are recorded, never dropped** — an import that quietly discards
what it could not match cannot be corrected, because nobody knows what was lost.

### `entity_match = not_found` does not reject the row

Ownership assignments are soft references with no foreign key. An entity CMM has never collected is
a legitimate assignment target, and assigning ownership *before* collection has run is a primary use
case, not an edge case. Rejecting on `not_found` would break it.

### Existing ownership is reported, not treated as failure

- **`owned_by_other`** — an assignment exists for this entity under a *different* owner. **The new
  assignment is still created.** `ownership_assignments` is many-to-many and overlapping ownership
  is legitimate by design; the source data carries multiple owners for one entity routinely. The
  outcome exists so the administrator sees the overlap, not to signal a problem. `existing_owners`
  carries the names already holding the entity.
- **`duplicate_exists`** — an identical assignment for the *same* owner already exists. No-op. What
  counts as identical is decided by the live uniqueness index on `ownership_assignments`, which
  treats an absent organisation as a distinct value rather than as a wildcard.

### The alias collision is not an outcome

The owner-alias uniqueness constraint is **global — one owner per alias value across the whole
table — not per owner**. So the same raw string cannot be seeded as a `custom` alias under a second
owner.

That is a fact about the *alias seed*, not about the assignment. It must not suppress or recolour
the assignment result. It is reported as two fields carried alongside whatever the outcome is: when
the conflict occurs, **the assignment is created normally and only the alias seed is skipped**. The
import never fails on it.

A row may therefore legitimately report `outcome = would_create` **and** `alias_conflict = true` —
a combination that a single merged "already owned by someone else" outcome made unrepresentable, and
which is exactly the case an administrator needs to see. **Aggregates are reported per outcome and
separately for alias conflicts; the two are never summed.**

### A fuzzy match is a suggestion, never an application

`owner_match = fuzzy_suggestion` means candidates exist and **none was applied**. The row is
attributed to a new owner built from the raw value, never to the suggested one.

It does **not** reject. Rejecting would lose the assignment at ingest, when nobody yet has the
context to adjudicate the name — and losing it is worse than recording it against a person who may
later turn out to be a duplicate, because a duplicate is correctable and a discarded row is not.
The suggestion travels with the row and with the new-people list instead, so the clue is there for
whoever later asks who this person is.

`owner_match` is what distinguishes "no idea" from "close matches exist", and it is always read
alongside the outcome. The row carries up to three suggestions with scores, from the **existing**
alias-suggestion facility — the trigram scoring is not reimplemented here.

The remedy, when someone recognises the person, is to point the alias at them; the next ingest then
resolves correctly and permanently. See *Re-ingesting the same source*.

## Endpoints

All under `/api/v1/ownership/import`.

| Endpoint | Purpose | Auth |
|---|---|---|
| `POST …/profile` | Columns, sample values, fill rates, row count, detected delimiter, warnings. Persists nothing. | protected |
| `POST …/preview` | Full pipeline, no writes, returns the match report. | protected |
| `POST …/commit` | Same payload as preview; writes the assignments. | operator or admin |
| `POST …/mappings` | Create a saved mapping. Name is unique; collision is a conflict. | operator or admin |
| `GET …/mappings` | List saved mappings — identity and provenance only, **not** the field map. Paginated. | protected |
| `GET …/mappings/{id}` | The full document, field map and delimiter included. | protected |
| `PUT …/mappings/{id}` | Replace name, delimiter and field map. Same validation as create. | operator or admin |
| `DELETE …/mappings/{id}` | Remove it. Referenced by nothing; no cascade. | operator or admin |

Profiling returns, per column, the name, up to 10 de-duplicated sample values, the proportion of
rows where it is non-empty, and a distinct-value count. Fill rate and distinct count are what let an
administrator recognise an identifier column from a free-text one without opening the file.

**Editing a saved mapping never re-runs a past import.** A mapping is a template, not a record of
what happened. Preview and commit accept **either** an inline field map **or** a saved mapping id,
never both; supplying both is a bad request, and an unknown id is a not-found.

Mapping-document validation — unknown target field, non-constant `entity_type` source, a column
naming a header the source does not have, an uncompilable pattern, an unknown transform — returns a
bad request identifying the offending field path. Naming the path matters: a document with six
fields and several transforms each has many places to be wrong, and "invalid mapping" sends the
administrator hunting.

## What must not change

The fixed-header import route stays registered and behaviourally identical, and that is covered by
a regression test rather than by inspection. It is the fast path for administrators who already
have a file in CMM's format, and removing it would break a documented workflow to gain nothing.

## Persistence

Migration `0056` creates `ownership_import_mappings`. The column set is derived from the migration
itself; what is decided here and must not drift is that the name is unique, the source kind is
constrained, the delimiter is stored with the mapping, and the field map is stored as JSON rather
than as normalised rows — it is a document with a nested union type, and shredding it into tables
would make every schema change to the mapping language a migration.

## Related

- [ownership](ownership.md) — owner model, entity-key formats, resolution order, soft-reference
  invariant.
- [ownership-api](ownership-api.md) — the wider ownership endpoint inventory.
- [ownership-operations](ownership-operations.md) — audit-log contract and the fixed-header import
  row cap.
- [auth](auth.md) — the alias model and identity anchors.
