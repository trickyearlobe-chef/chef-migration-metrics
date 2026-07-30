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

Captured as real fixtures rather than prose, so this is pinned rather than remembered
(`internal/analysis/testdata/`): `cookstyle_scan_plain.json`,
`cookstyle_scan_autocorrected.json`, `cookstyle_scan_noncorrectable.json`. Paths are
relativised; content is otherwise untouched cookstyle output. These are the fixtures
Chunk 3 requires.

**Still to confirm:** the same probe on Workstation 26 (`dev.home.arpa`) — blocked, the
1Password SSH agent will not sign. Expected to match, since WS25 and WS26.1 already agree.

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

## Chunk 1 — carry the flag end to end

Add `Correctable` to `analysis.CookstyleOffense` and to `remediation.EnrichedOffense`,
populate it through `enrichOffenses`, and correct the read-side struct tags. Fix
`cookstyle_fingerprint.go` to store `correctable`, not `corrected`.

Acceptance: a scan of a cookbook with a trailing-whitespace offence persists
`correctable: true`; the remediation page reports a non-zero correctable count.

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
2. **Reset previews explicitly** — **blocker: there is no operator trigger.**
   `ResetPreviews`/`ResetAllPreviews` are Go methods with no HTTP route and no CLI entry
   point, unlike `rescan-all-cookstyle` (`POST /api/v1/admin/rescan-all-cookstyle`).
   At a VDI-only site this step is currently unexecutable; building the trigger is
   unscoped work that must be added to Chunk 1 or 2.
   Original note: — `autocorrect.go:309` skips generation when a preview
   already exists, so without `ResetPreviews`/`ResetAllPreviews` (`:556-576`) the fix
   silently no-ops. `diff_output`/`files_modified` are sound and need no correction.
3. Complexity recomputes from previews and follows automatically.

`cookstyle_offence_fingerprints` history stays wrong; accept it.

Note the re-scan cost against the customer's fleet before scheduling it — this is the
same host that OOMed on 2026-07-29/30.

## Related

Spec `specifications/cookstyle-*.md` should state that `correctable` is the static
capability and `corrected` only reflects an applied correcting run. Ask the owner before
editing.
