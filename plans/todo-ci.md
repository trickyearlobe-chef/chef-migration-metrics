# TODO — CI / supply-chain scanning

Tiered scan policy shipped in PR #49 (`ci: tiered supply-chain scanning +
SHA-pin all actions`) and follow-on merges. The implementation details live in
git history; only the open items remain here.

## 1. Promote MEDIUM npm gate to blocking
The Trivy npm production-dep gate fails on HIGH/CRITICAL; MEDIUM is staged behind
`continue-on-error`. Promote it to blocking once the two pending fixable mediums
land — react-router@6.30.4 + brace-expansion@5.0.6 (needs the registry; a
dependabot branch for react-router-dom is open).

## 2. Verify scanner inputs on first CI run
First push is the real test (cannot run GitHub Actions locally). Confirm:
- trivy-action input names (`scan-ref`, `TRIVY_INCLUDE_DEV_DEPS`).
- osv-scanner v2 `go install` path.

## 3. (Optional) Pin nfpm for full tool pinning
`nfpm@latest` in `release.yml` is the last unpinned tool version. Pin it to a
specific version for complete tool pinning.
