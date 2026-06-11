# Active — TLS refinements (backlog)

Two independent TLS follow-ons, **spec-first in parallel**. Sourced from
[todo-tls.md](todo-tls.md) §1 and [todo-tls-antilockout.md](todo-tls-antilockout.md) §2.
Decision: 443 lifeboat is **conventional half only** — the automatic
health-driven port-flip stays out of scope (remains todo-tls-antilockout.md §2).

Specs: do NOT edit without asking (CLAUDE.md). Each spec chunk ends by presenting
the proposed delta for confirmation before writing.

## Workstream 1 — Cert-chain display + reorder-on-save

### W1-S — spec (do first) — DONE
- Landed: `tls-static.md` §2.2 (chain-metadata display shape + reorder-on-save,
  static-upload-only, warn-and-store) + §2.7 metadata sentence; `tls-acme.md` §3.14
  `tls_certificate_info` now describes the chain. Implementation chunks W1-A/W1-B
  to follow in a fresh thread.
- Decisions (confirmed 2026-06-10):
  - Reorder scope: **static-upload paste only** (cert_source: db admin save).
    CSR-promoted / file-source bundles untouched.
  - Broken/incomplete chain on save: **warn-and-store**, never reject (fail-open
    ethos; only unparseable PEM rejected by existing preflight §2.6).
  - Self-signed root (subject==issuer) detected + sorted last; chain displayed in
    full leaf→intermediate(s)→root with per-cert role.
- Output: spec delta (chain metadata shape + reorder contract). No code.

### W1-A — chain display (dep: W1-S) — DONE
- Branch `feature/tls-chain-display`. `CertMetadata` gains `role`; new
  `ChainMetadataFromPEM` parses every cert, derives role structurally (root =
  self-signed; leaf = subject issues no other; else intermediate), skips non-cert
  blocks. `tls_certificate_info` is now a chain **array** (static-DB + ACME);
  removed the orphaned `CertMetadataFromPEM`. UI: `CertChainPanel` renders one
  card per cert with role label. Go + 386 FE tests green.

### W1-B — reorder-on-save (dep: W1-S) — DONE
- Branch `feature/tls-chain-reorder`. New `ReorderChainPEM` (`internal/tls/`)
  sorts an operator-supplied `cert_source: db` bundle into leaf → intermediate(s)
  → root by issuer→subject linking, non-self-signed leaf first (so it survives as
  cert[0] for the key-match preflight that runs after reorder). Wired into the
  admin save path's `haveCert && haveKey` branch *before* `ValidateStaticPairBytes`.
  Incomplete/non-linking bundles stored with a non-fatal `warnings` entry in the
  PUT response (never rejected); CSR-promote and file source left as-is. Go green.
- Follow-up (todo-tls §1): warning is transient (response only) — persistent GET
  status + FE display unbuilt.

## Workstream 2 — 443 lifeboat (conventional half only)

### W2-S — spec (do first) — DONE
- Landed in `tls.md`: new §1.5 (automatic HTTPS on 443 + redirect, privileged-bind
  fallback, degraded→no-443, runtime flip out of scope) + §1.3 reconciled + §1.2
  collision rule extended to 443. Implementation chunk W2-A to follow in a fresh
  thread.
- Original scope: `specifications/tls.md` §1.2–1.5.
- Decision (confirmed 2026-06-10): **automatic on TLS-enable** (not opt-in). When
  TLS is active AND the listener builds healthy, auto-bind HTTPS on 443 and start
  a 301 redirect on the configured `server.port` → 443. This deliberately amends
  §1.3's "no surprising port changes" — spec must reconcile it (old URL still
  works via redirect, so no data-loss surprise).
- Contract: 443 bound only when TLS healthy at startup. 443 unbindable
  (unprivileged) → existing §1.4 bind-failure fallback: serve HTTPS on
  `server.port`, no redirect. Degraded TLS (§2.4 fail-open) → no 443; self-signed
  holds `server.port` as today. `server.port: 443` already set → no redirect.
  `http_redirect_port` (when set) targets 443; must differ from both server.port
  and 443.
- OUT of scope: runtime health-driven port-flip + hot listener rebind (stays
  todo-tls-antilockout.md §2). Output: spec delta. No code.

### W2-A — impl (dep: W2-S)
- Scope: `internal/tls/listener.go` (reuse `ChallengeRedirectServer` /
  `http_redirect_port`), `internal/config/config.go`, server wiring.
- Implement 443 + redirect-old-port on healthy TLS; bind-failure fallback when 443
  unavailable. No graceful hot-rebind (restart-required boundary unchanged).
- TDD: healthy TLS → 443 listener + old-port redirect; 443 bind failure → fallback.

## Dependencies
- W1-S → {W1-A, W1-B}. W2-S → W2-A. W1 and W2 fully independent; specs can run
  in parallel.

## Notes
- Auto health-driven port-flip (hot rebind, hysteresis, flap protection) is
  deliberately deferred — see todo-tls-antilockout.md §2 design tensions.
- `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` on DB path stays a documented
  limitation (todo-tls-antilockout.md §1) — not in scope here.
