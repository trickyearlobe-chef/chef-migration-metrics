# TODO — CI / supply-chain scanning

Tiered scan policy shipped in PR #49 (`ci: tiered supply-chain scanning +
SHA-pin all actions`) and follow-on merges. The implementation details live in
git history; only the open items remain here.

## 1. Promote MEDIUM npm gate to blocking
The Trivy npm production-dep gate fails on HIGH/CRITICAL; MEDIUM is staged behind
`continue-on-error`. Promote it to blocking once the two pending fixable mediums
land — react-router@6.30.4 + brace-expansion@5.0.6 (needs the registry; a
dependabot branch for react-router-dom is open).

## 1b. Enforce the pre-commit hook in CI
The `.githooks/pre-commit` checks (secret/deny scan, spec ≤500 lines, and the new
§5 "no implementation code in specs" lint) are local-opt-in only — installed via
`make install-hooks` (`core.hooksPath`), not run in CI. They can be bypassed
silently (`--no-verify`, or simply never installing). Add a CI job that runs the
hook's checks against the PR diff so they are a real gate, not advisory. (Recorded
2026-06-23 with spec-drift-control Chunk A; see `plans/spec-drift-control.md`.)

## 2. Verify scanner inputs on first CI run
First push is the real test (cannot run GitHub Actions locally). Confirm:
- trivy-action input names (`scan-ref`, `TRIVY_INCLUDE_DEV_DEPS`).
- osv-scanner v2 `go install` path.

## 3. (Optional) Pin nfpm for full tool pinning
`nfpm@latest` in `release.yml` is the last unpinned tool version. Pin it to a
specific version for complete tool pinning.

## 4. New Go modules from the Route 53 DNS-01 solver (Chunk 9)
The dns-01 solver (`internal/acme/route53.go`) added the `aws-sdk-go-v2` subset.
This enlarges the Go lockfile-scan surface — direct: `aws-sdk-go-v2`,
`aws-sdk-go-v2/config`, `aws-sdk-go-v2/credentials`,
`aws-sdk-go-v2/service/route53`; plus ~10 indirect modules pulled by `config`'s
default credential chain (`feature/ec2/imds`, `service/sso`, `service/ssooidc`,
`service/sts`, `service/signin`, `internal/*`, `smithy-go`). Expected and
accepted per `tls-acme.md` § 3.1 (the SDK subset is deliberately preferred over
`lego`, which pulls every DNS-provider SDK). Action: confirm the osv-scanner /
Trivy Go gate covers these and triage any advisories; no module here should be a
surprise on the first scan.

## 5. Node 20 deprecation — `softprops/action-gh-release`
The action targets Node 20 and GitHub is forcing it onto Node 24. Nothing is
broken today: it is SHA-pinned at `release.yml:394`
(`3bb12739c298aeb8a4eeaf626c5b8d85266b0e65` = v2.6.2). Action: bump the pin to a
Node 24 release before Node 20 support is withdrawn, keeping the exact-SHA pin
and running the usual supply-chain check. Last survivor of the
collector-performance batch.
