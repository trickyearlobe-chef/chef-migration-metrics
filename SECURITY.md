# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Chef Migration Metrics, please
report it responsibly using
[GitHub private vulnerability reporting](https://github.com/trickyearlobe-chef/chef-migration-metrics/security/advisories/new).

**Please do NOT open a public issue for security vulnerabilities.**

### What to include

- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept
- The version(s) affected, if known
- Any suggested fix or mitigation

### What to expect

| Step | Timeframe |
|------|-----------|
| Acknowledgement of your report | 3 business days |
| Initial assessment and triage | 7 business days |
| Fix development and testing | Best effort, depends on severity |
| Public disclosure (coordinated) | After a fix is available |

We follow [coordinated vulnerability disclosure](https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure)
and will credit reporters (unless anonymity is requested) in the release notes
and any published advisory.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Previous minor | Best effort |
| Older | No |

## Security Practices

- **Dependency scanning** -- Dependabot monitors Go modules, npm packages,
  GitHub Actions, and Docker base images for known CVEs.
- **Code scanning** -- CodeQL static analysis runs on every pull request.
- **Minimal runtime image** -- The container runs a static Go binary on
  debian:bookworm-slim with only ca-certificates, git, and
  openssh-client installed.
- **Non-root container** -- The container runs as a dedicated service user,
  not root.
