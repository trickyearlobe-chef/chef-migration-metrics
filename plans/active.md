# Active — CI supply-chain scanning hardening (DRAFT, review before merge)

Branch: `chore/ci-supply-chain-scanning`. Cannot run GitHub Actions locally —
first push is the real test. Do not merge until reviewed.

## Goal

Tiered scan policy in `.github/workflows/ci.yml` `security` job that is strict
where it matters without alert fatigue:

- **Hard fail (block PR):**
  - govulncheck — Go reachable vulns, ANY severity (call-graph = real signal).
  - Trivy — npm PRODUCTION deps, HIGH/CRITICAL (always on). MEDIUM staged behind
    `continue-on-error` until the two pending fixable mediums land, then promote.
- **Report-only (SARIF → Security tab, never block):**
  - Trivy full scan incl dev deps + LOW + secret + misconfig.
  - osv-scanner (includes dev deps; OSV DB, no npm needed).
  - behavioural lockfile scan (`scripts/npm-supply-chain-scan.sh`).
- **KEV/EPSS-aware tier:** GitHub Dependabot — config ALREADY EXISTS
  (`.github/dependabot.yml`: gomod + npm + github-actions). No change needed;
  just enable Dependabot alerts + security updates in repo settings (those drive
  the EPSS/KEV prioritisation).

## Hardening applied

- All actions in the `security` job pinned to commit SHAs (not tags).
- Tool versions pinned (govulncheck v1.1.4, osv-scanner v2.3.8, trivy-action
  v0.36.0, codeql-action v3).
- Scan the LOCKFILE — no `npm ci` in the scan job, so a compromised dependency's
  install hook cannot execute on the runner. (`.npmrc` ignore-scripts already
  covers the lint/test jobs that do install.)
- `permissions:` scoped: `contents: read` + `security-events: write` (SARIF) on
  this job only.

## Pinned SHAs (verify on first run)

- actions/checkout v6.0.3 → df4cb1c069e1874edd31b4311f1884172cec0e10
- actions/setup-go  v6     → 4a3601121dd01d1626a1e23e37211e3254c1c06c
- actions/setup-node v6.4.0 → 48b55a011bda9f5d6aeb4c2d9c7362e8dae4041e
- aquasecurity/trivy-action v0.36.0 → ed142fd0673e97e23eac54620cfb913e5ce36c25
- github/codeql-action/upload-sarif v3 → dd903d2e4f5405488e5ef1422510ee31c8b32357

## Done

- SHA-pinned ALL actions across `ci.yml` (lint/test/security) and `release.yml`
  (checkout, setup-go, setup-node, upload-/download-artifact, trivy, codeql,
  softprops) — 24 `uses:` lines, each a 40-char SHA + `# version` comment.
- Added `make scan-trivy` (vuln+secret+misconfig, HIGH/CRITICAL gate, skips
  node_modules/embedded/.samples) and chained it into `make scan`.

## Follow-ups / open

- Promote MEDIUM npm gate to blocking once react-router@6.30.4 +
  brace-expansion@5.0.6 land (needs registry).
- Verify trivy-action input names (`scan-ref`, `TRIVY_INCLUDE_DEV_DEPS`) and the
  osv-scanner v2 go-install path on first CI run.
- Optional: pin `nfpm@latest` (release.yml) to a version for full tool pinning.
