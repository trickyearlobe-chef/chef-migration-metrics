# TODO — CI / supply-chain scanning

## Enforce the rest of the pre-commit hook in CI
The secret/deny-pattern scan and the journey prose lint are local-opt-in — installed via
`make install-hooks` (`core.hooksPath`) — and can be bypassed silently with `--no-verify`
or by never installing. Add a CI job that runs those checks against the PR diff so they
are a real gate, not advisory.

## Pin nfpm for full tool pinning
`nfpm@latest` in `release.yml` is the last unpinned tool version. Pin it to a
specific version for complete tool pinning.
