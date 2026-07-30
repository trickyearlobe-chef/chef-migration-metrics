# Cookstyle Correctable Flag

Auto-correctable offence counts are zero everywhere, for every cookbook, including
trivially correctable cops. Complexity scores are inflated as a direct consequence, so
this distorts remediation prioritisation as well as display.

## Symptom

The remediation detail page shows "Auto-Correctable 0 / 0% of total" and badges every
cop "Manual fix", while the Auto-Correct Preview on the same page renders a correct,
non-empty diff. Its header is internally contradictory: "0 of 3 offenses correctable ·
2 files modified · 3 remaining after auto-correct".

## Verified toolchain behaviour

Re-measured 2026-07-30 on **Chef Workstation 25.14.2 / Cookstyle 8.6.10 / RuboCop 1.84.2**,
reproducing an earlier probe on Workstation 26.1.0 / Cookstyle 8.7.6 / RuboCop 1.86.1.
The behaviour is identical on both, so it is not version-specific.

- Each offence carries **both** `correctable` and `corrected`.
- Plain scan: `correctable=true`, `corrected=false`.
- After `-A`: `summary.offense_count` is **unchanged** (3 → 3 measured), and offences flip
  to `corrected=true`.
- `correctable=false` genuinely occurs and must round-trip: measured across 165 real
  cookbook files, 64 correctable offences and 19 non-correctable
  (`Chef/Sharing/DefaultMetadataMaintainer`, `Chef/Deprecations/ResourceWithoutUnifiedTrue`).

So `summary.offense_count` is not "what could not be fixed", and `corrected` is
meaningless on a non-correcting scan. Both assumptions are currently relied upon.

Confirmed on both toolchains. The **Workstation 26.1.0 / Cookstyle 8.7.6** probe is the
reference because it carries both classes of offence in one document:

| | plain scan | the `-A` run's own report |
|---|---|---|
| `summary.offense_count` | 6 | **6 — unchanged** |
| 3 correctable offences | `corrected=false` | `corrected=true` |
| 3 non-correctable offences | `corrected=false` | `corrected=false` |

Giving the arithmetic Chunk 2 needs: `correctable = count(corrected==true)` = 3,
`remaining = offense_count - corrected` = 3.

### CMM runs `--auto-correct`, not `--auto-correct-all`

`remediation/cookstyle_invocation.go:28` and `autocorrect.go:440` build
`{"--auto-correct","--format","json"}` — safe corrections only. `--auto-correct-all`
(`-A`) additionally applies corrections RuboCop marks unsafe. **The two disagree**, so any
measurement used to design the fix must use `--auto-correct`.

Measured across 165 real cookbook files (WS 25.14.2):

| Command | `correctable=true` | `corrected=true` | Gap |
|---|---|---|---|
| `--auto-correct` (what CMM runs) | 77 | 76 | **1** |
| `--auto-correct-all` | 77 | 77 | 0 |

The gap was `Style/ArrayFirstLast` — an unsafe correction. Minimal reproduction on the
deployed toolchain in `cookstyle_scan_unsafe_correctable.json`: two offences, both
`correctable=true`, both `corrected=false` after `--auto-correct`.

**Consequence for the fix — the two counts are not interchangeable:**

- The static `correctable` flag is the cop's *capability*. Persist it per offence
  (Chunk 1); it is the right input for the per-cop pages.
- What CMM's preview *actually fixes* is the `corrected` count from the `--auto-correct`
  run, which can be lower.

If the remediation page's "Auto-Correctable N" comes from the static flag while the diff
beside it comes from the run, the header can contradict the diff again — a smaller version
of the exact bug being fixed. **Recommendation:** the page's headline count and the
complexity input both come from the run's `corrected` count; the static flag drives
cop-level capability views. Needs sign-off before Chunk 2.

`--auto-correct` modifies files in place, which is why previews run on a temporary copy
(`autocorrect.go:354`, and the directory copy helpers at `:594`). Any future probing must
copy first.

### Trap: the `-A` report is not a re-scan

A plain scan of the *already corrected* tree reports **3** offences, all `corrected=false` —
the fixed ones are gone, and nothing claims to have corrected them. The `-A` invocation's
own report instead lists all 6 with flags. The two disagree, so Chunk 2 must read the
output of the correcting run itself and must not re-scan afterwards to derive counts.
(Found by making this mistake while capturing the fixtures.)

Captured as real fixtures rather than prose, so this is pinned rather than remembered
(`internal/analysis/testdata/`, see its README): the WS26 mixed pair
`cookstyle_scan_mixed_plain.json` / `cookstyle_scan_mixed_autocorrected.json`, plus the
WS25 set `cookstyle_scan_plain.json`, `cookstyle_scan_autocorrected.json` and
`cookstyle_scan_noncorrectable.json`. Paths are relativised; content is otherwise
untouched cookstyle output. These are the fixtures Chunk 3 requires.

## Verified code state — re-checked at `e602fdb`

Every claim below re-read at the current commit; earlier line numbers had drifted.

| Claim | Evidence |
|---|---|
| `CookstyleOffense` has no correctability field | `analysis/cookstyle.go:55-65` — only `Corrected` |
| Correctable counted from the wrong flag | `analysis/cookstyle.go:603-604`, `:751-752` — `if off.Corrected { sr.CorrectableCount++ }` |
| `EnrichedOffense` drops it entirely | `remediation/copmapping.go:42-51` — no such field |
| Fingerprints store the wrong flag | `analysis/cookstyle_fingerprint.go:37` — `correctable: off.Corrected` |
| Struct tag drift | `webapi/handle_cookstyle_cops_shared.go:23,37` — `Correctable bool \`json:"corrected"\`` |
| Preview subtracts against a total that never shrinks | `remediation/autocorrect.go:393-394` |
| The `-A` parser discards per-offence flags | `remediation/autocorrect.go:584-586` — parses only `summary` |
| Complexity inflated by the zero | `remediation/complexity.go:167` — `ManualFixCount * WeightNonAutoCorrectable` (weight 4, `:37`) |
| No operator trigger for the reset | `remediation/autocorrect.go:556`, `:571` — no route in `router.go`; the only `reset` route is `platform-display-names` (`router.go:954`) |

### Resolves open question 1: `correctable` is canonical

The toolchain emits both keys with distinct meanings — `correctable` is the static
capability, `corrected` only reports that a correcting run changed the file. The
remediation handlers already read `correctable`. `handle_cookstyle_cops_shared.go` names
its field `Correctable` and merely *tags* it `json:"corrected"` — so the intent is
`correctable` everywhere and the tags are simply wrong. No design decision is needed:
fix the tags, do not migrate the handlers to `corrected`.

## Root cause — two independent broken derivations

Neither reads `correctable`.

1. **Persistence drops the field.** `remediation.EnrichedOffense`
   (`copmapping.go:42-51`) has no correctability field, and scan results are stored as
   `json.Marshal(enrichOffenses(...))` (`analysis/cookstyle.go:985` server,
   `:1049` git). Read-side handlers unmarshal a `"correctable"` key that was never
   written, so it decodes false for every offence.
2. **The preview subtracts against a total that never shrinks.**
   `remediation/autocorrect.go:393` computes
   `csResult.OffenseCount - afterOutput.Summary.OffenseCount`. Per the probe above that
   is always ~0. `autocorrectJSONOutput` (`:584-592`) parses only `summary` and discards
   the per-offence `corrected` flags it would need. `FilesModified` and `diff_output`
   come from real file contents and are correct — which is why the diff is right while
   every number around it is wrong.

Contributing: `analysis.CookstyleOffense` (`cookstyle.go:59`) declares only `Corrected`,
and `cookstyle.go:603`/`:751` do `if off.Corrected { CorrectableCount++ }`. That count is
only logged, never persisted, so fixing it alone changes nothing user-visible.

## Blast radius

- `remediation/complexity.go:486,566` — `AutoCorrectableCount` from the broken preview
  (0) and `ManualFixCount` = total, so `WeightNonAutoCorrectable` (`:167`) applies to
  every offence. **Complexity scores are systematically inflated.** Propagates to
  quick-wins on `RemediationPage.tsx` (`manual_fix_count === 0` can never be true) and to
  role detail via `datastore/role_detail.go:306`.
- `webapi/handle_git_repo_remediation.go:191,229,244,312` — twin of the cookbook page.
- `webapi/handle_cookstyle_cops.go:149,262` — `AutoCorrectablePct` always 0.
- `webapi/handle_cookstyle_cop_cookbooks.go:216,243` — `auto_correctable` always 0.
- `webapi/handle_cookstyle_cops_shared.go:23,37` — field named `Correctable` but tagged
  `json:"corrected"`; the drift in miniature.
- `analysis/cookstyle_fingerprint.go:37` — stores `correctable: off.Corrected`, so
  historical fingerprints all record false. Not retroactively repairable.

Frontend is a faithful pass-through — no fix needed there.

## Open questions to settle before Chunk 1

- ~~Which key is canonical?~~ **Settled: `correctable`** — see above. Note the rollout
  consequence still stands: every pre-rescan stored row decodes false until the re-scan
  completes, so the order in Data repair below matters.
- **Does changing the fingerprint key fire spurious change detection fleet-wide?**
  Chunk 1 changes `cookstyle_fingerprint.go:37`, so every fingerprint changes on the next
  scan.
- **Which spec gets the correctable/corrected definition?** `specifications/cookstyle-*`
  matches several files; `analysis-cookstyle.md` is the likely home. Needs owner sign-off.
- **Chunk 2's acceptance criterion is not reproducible** — the "two-offence probe
  cookbook" is not in the repo and no location is given, and the Symptom section
  describes a different, three-offence cookbook.

## Verified locally against the lab (2026-07-30)

Reproduced the bug and confirmed the fix end to end on the dev database, scanning
real cookbooks with Cookstyle 8.6.10.

Before: 8 scan results, **0** containing the `correctable` key; 7 previews totalling
**293 offences with 0 correctable** — the customer symptom exactly.

After a rebuild and re-scan (results, previews and complexity cleared first, as
`rescan-all-cookstyle` does):

- `correctable` persisted in every result that has offences (the one without it is a
  cookbook with zero offences and a null `offences` column).
- Previews: **123 total, 105 correctable, 18 remaining, 23 files modified**.
- `total == correctable + remaining` holds for every row.
- Complexity records carry real `auto_correctable_count` values, so **Quick Wins fire
  again** — two cookbooks qualify (`auto_correctable_count > 0 && manual_fix_count == 0`),
  which was unsatisfiable while the count was always zero.

**The static/actual divergence is real, not theoretical:** kubernetes-cluster is 83
statically correctable but 82 actually fixed; cron is 12 against 10. Wiring the headline
card to the static flag would have shown "83 auto-correctable" beside a diff fixing 82 —
the contradictory header this fix exists to remove.

## Chunk 1 — carry the flag end to end, and build the reset trigger

Add `Correctable` to `analysis.CookstyleOffense` and to `remediation.EnrichedOffense`,
populate it through `enrichOffenses`, and correct the read-side struct tags. Fix
`cookstyle_fingerprint.go` to store `correctable`, not `corrected`.

**Also build the preview-reset operator trigger** (folded in here 2026-07-30 — without it
the fix silently no-ops, see Data repair). An admin endpoint alongside
`POST /api/v1/admin/rescan-all-cookstyle`, wired to `ResetPreviews`/`ResetAllPreviews`
(`autocorrect.go:556`, `:571`), **plus the admin UI control** so it is executable at a
VDI-only site.

Acceptance: a scan of a cookbook with a trailing-whitespace offence persists
`correctable: true`; a non-correctable offence persists `correctable: false`; the
remediation page reports a non-zero correctable count; an operator can reset previews from
the UI without shell access.

## Chunk 2 — fix the preview arithmetic

Count offences with `corrected == true` in the `-A` output; `remaining = offense_count -
corrected`. Requires `autocorrectJSONOutput` to parse per-offence flags, not just
`summary`.

Acceptance: for the two-offence probe cookbook, preview reports 2 correctable / 0
remaining / 1 file modified, consistent with its own diff.

## Chunk 3 — contract test closing the CI gap

CI is green today because tests hand-fabricate stored JSON containing keys the pipeline
never writes (`handle_cookbook_remediation_test.go:302-321` injects `"correctable"`;
`handle_cookstyle_cops_test.go:38-40` injects `"corrected"`). The two disagree about
which key is canonical, and nothing pins the persisted shape against `enrichOffenses`
output.

Add a contract test asserting the marshalled `EnrichedOffense` shape matches what the
handlers unmarshal, plus a fixture captured from real cookstyle output rather than
hand-authored.

## Data repair — required, code fix alone is insufficient

The field was never written and there is no per-offence correctable column in any
migration, so stored data is unrecoverable without re-running scans. Order matters:

1. Re-scan (`rescan-all-cookstyle`) to repopulate `*_cookstyle_results.offences`.
2. **Reset previews explicitly.** `autocorrect.go:309` skips generation when a preview
   already exists, so without `ResetPreviews`/`ResetAllPreviews` (`:556`, `:571`) the fix
   silently no-ops. `diff_output`/`files_modified` are sound and need no correction.
   These were Go methods with no HTTP route and no CLI entry, making the step
   unexecutable at a VDI-only site; **the endpoint and its UI control are now scoped into
   Chunk 1** (agreed 2026-07-30).
3. Complexity recomputes from previews and follows automatically.

`cookstyle_offence_fingerprints` history stays wrong; accept it.

Note the re-scan cost against the customer's fleet before scheduling it — this is the
same host that OOMed on 2026-07-29/30.

## Related

Spec `specifications/cookstyle-*.md` should state that `correctable` is the static
capability and `corrected` only reflects an applied correcting run. Ask the owner before
editing.
