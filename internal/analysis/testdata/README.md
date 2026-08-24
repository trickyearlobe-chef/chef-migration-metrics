# Cookstyle output fixtures

Real `cookstyle --format json` output. Only the `path` fields are rewritten to be
cookbook-relative; everything else is untouched. Do not hand-edit these — recapture them
if the toolchain changes.

## What they pin

- Every offence carries **both** `correctable` and `corrected`.
- `correctable` is the static capability of the cop. `corrected` only reports that a
  correcting run changed the file, and is always `false` on a plain scan.
- A correcting run does **not** shrink `summary.offense_count` in its own report.

## Files

| File | Toolchain | Content |
|---|---|---|
| `cookstyle_scan_mixed_plain.json` | WS 26.1.0 / Cookstyle 8.7.6 / RuboCop 1.86.1 | 6 offences: 3 correctable, 3 not. All `corrected=false`. |
| `cookstyle_scan_mixed_autocorrected.json` | as above | The `-A` run's own report: still 6 offences, the 3 correctable now `corrected=true`. |
| `cookstyle_scan_plain.json` | WS 25.14.2 / Cookstyle 8.6.10 / RuboCop 1.84.2 | 3 offences, all correctable, none corrected. |
| `cookstyle_scan_autocorrected.json` | as above | Same 3 offences after `-A`, all `corrected=true`, count unchanged. |
| `cookstyle_scan_noncorrectable.json` | as above | Real-world non-correctable cops, reduced to one occurrence each. |
| `cookstyle_scan_unsafe_correctable.json` | WS 26.1.0 / Cookstyle 8.7.6 / RuboCop 1.86.1 | `--auto-correct` output where offences are `correctable=true` but `corrected=false` — an unsafe correction CMM does not apply. |

The WS25 and WS26 pairs agree, so the behaviour is not version-specific. The **mixed** pair
is the reference: it is the deployed toolchain and carries both classes of offence in one
document, which is what distinguishes "reads the flag" from "hardcodes true".

## Safe vs unsafe corrections

CMM runs `cookstyle --auto-correct` (safe only), never `--auto-correct-all`. Some cops
report `correctable=true` yet are left untouched because their correction is unsafe.
Across a real cookbook corpus, `--auto-correct` leaves a small share of correctable
offences untouched that `--auto-correct-all` fixes.

So the static `correctable` flag and "what our preview actually fixed" are **different
numbers**, and `cookstyle_scan_unsafe_correctable.json` is the minimal case where they
diverge. Fixtures captured with `-A` will overstate what CMM corrects.

`--auto-correct` rewrites files in place — always run it on a copy.

## The trap these guard against

`cookstyle_scan_mixed_autocorrected.json` is the output of the `-A` invocation itself. A
plain re-scan of the already-corrected tree is a *different* document: 3 offences, all
`corrected=false`, because the fixed ones are simply gone. Deriving counts from a re-scan
rather than from the correcting run's own report yields different numbers.
