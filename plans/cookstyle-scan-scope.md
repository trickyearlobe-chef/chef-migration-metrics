# Cookstyle scan scope — the repository is not the cookbook

A cookstyle scan covers the whole cloned repository. Customer repositories carry a Rakefile and a
Jenkins CI file containing `File.exists?`, near-identical in ~95% of repos, so ~95% of cookbooks
read as breaking on Chef 19 when the cookbook code is fine.

The journey is already written: [trusting what the scan says](../journeys/scan-trust.md) holds the
decisions, and [whether a cookbook survives](../journeys/cookbook-compatibility.md) carries the
one-line consequence. **Read those first — this file is only what an implementer needs on top.**

## Verified, do not re-derive

All checked 2026-08-09 against the lab Chef server, the lab box's Chef 19.3.15 source, and the dev
database. Each cost real digging.

- **Offences already store the file path.** `location.file`, repo-relative, inside the `offences`
  JSONB. So filtering needs no rescan.
- **Chef's upload strips almost nothing.** With no chefignore, `Rakefile`, `Jenkinsfile`,
  `Gemfile`, `spec/`, `test/` and even an invented `extradir/helper.rb` all upload — proved by
  uploading a probe cookbook and downloading it back. The only automatic exclusion is a top-level
  dot-directory. There is no allowlist in the upload path, and `COOKBOOK_SEGMENTS` is not the
  filter.
- **chefignore matches globs, so bare directory names do nothing.** A chefignore listing `spec`
  strips nothing; `spec/*` works. Lab repos get this wrong, and `kubernetes-cluster` consequently
  has 23 `spec/` and 20 `test/` files on the Chef server despite listing both.
- **We never read chefignore.** No reference to it anywhere in the tree. Do not start: it would
  import the customer's mistake as our verdict.
- **The git/server divergence is ours, not Chef's.** `CookbookVersionManifest` parses only the
  legacy segments and never `all_files`, and a segment view cannot express a file in an arbitrary
  directory. So the server path silently drops `spec/` *and* real code like `extradir/helper.rb`.

## The hard part: the path is discarded three times

The path exists on the raw offence and is dropped before anything that decides a verdict.

- The fingerprint projection **deliberately omits source locations** — its own comment says
  re-derivation does not consume them. So the reclassification-without-rescan path cannot filter
  by path until the fingerprint carries the split.
- `ClassifiedOffense` carries cop name, severity and classification only.
- The complexity scorer consumes those, so it never sees a path either.

Threading the path through, or splitting counts before classification, is the design problem.
Raw offences are retained, so no rescan is needed either way.

## Decided — do not re-litigate

- Filter at derivation or display, **never at scan time**. The data must survive so the detail
  page can still list what was excluded.
- A **curated, operator-editable exclusion list**, not an inferred allowlist. Seed it from the
  patterns Chef's own cookbook generator ships (`Rakefile`, `spec/*`, `test/*`, `kitchen.yml*`,
  `Gemfile`, `.github/*`) for defensible provenance.
- Every entry carries a reason, same discipline as calling a finding harmless.
- Excluded findings stay visible on the cookbook as non-blocking, and are counted across the
  estate for prevalence.

## Default taken, not an owner decision

**Two counts per cop rather than a filter** on the estate-wide findings view: how many cookbooks
this blocks, and how many carry it only in excluded files. Nothing is hidden, and the two tabs
stay comparable. Flagged as a default in the journey. Revisit if the owner disagrees.

## Build the test first, and expect it red

A repository carrying the same breaking finding twice — once in cookbook code, once in a helper
task. The verdict follows only the cookbook copy, **and** the helper task's finding is still
readable afterwards. The second half is what stops this being implemented as deletion.

This is the first journey property with no test behind it, so it is also where the deliberately-red
mechanism gets built: its own build tag and make target, kept out of the gating suite, so red means
"not proven" rather than a broken build.

## Already built, do not rebuild

The estate-wide prevalence view exists. Cop Analysis (Server) and Cop Analysis (Git) on the
remediation page already carry per-cop `cookbooks_affected`, `total_offences` and `unblocks`, and
sort on them. The aggregation parses each offence individually, so the path is already in hand at
the point the counts accumulate. This is a correction to numbers people already read, not a new
view.

## Related defect, decide whether it is in scope

The server-side path under-counts: reading segments only, it never scans Ruby that Chef genuinely
ships outside them. So today the git side over-counts and the server side under-counts, and these
are the numbers a migration lead quotes. Either fix it here or record it in
[snagging](todo-snagging.md) — but do not leave it undecided.
