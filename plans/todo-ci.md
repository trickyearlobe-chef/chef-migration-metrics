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
