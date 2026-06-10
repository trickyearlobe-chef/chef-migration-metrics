# Active — TLS in DB, CSR generation, ACME/Route53

Branch: `feature/tls-db-certs-csr-acme`. Full plan + per-chunk detail:
[plans/tls-db-certs-csr-acme.md](tls-db-certs-csr-acme.md).

Sequence: `0 → 1 → 2 → 3 → 3a → 4 → 5 → 6 → 7 → 8a → 8b → 9 → 9a → 10 → 11`
(8c, 8d done — CLI + admin-UI behind-proxy toggles).

## Done

Chunks 0–6 — spec split, config schema/validation, CertManager PEM source, DB
cert/key storage + admin API, the `tls reset`/`tls clear-ca` repair CLI (recovery
escape hatch), the DB cert/key UI (**Feature 1 complete**), the CSR generation
backend (`internal/tls/csr.go` + generate-csr endpoint + match-and-promote), and
the CSR UI (`generateCSR()` + AdminServerPage CSR panel — **Feature 2 complete**).
Chunk 7 — ACME core engine: new `internal/acme/` (DB storage layer, account +
order flow via `x/crypto/acme` behind a `Solver` seam, renewal scheduler with
1h→24h backoff + expiry-warning events). 3a unblocks everything below.
Chunk 8a — self-signed degraded fail-open foundation: fail-open now serves an
ephemeral self-signed cert over HTTPS (HSTS suppressed) instead of cleartext,
plain HTTP last-resort only; HSTS live-gate; status `kind` + in-place promotion
clears degraded; static `cert_source:db` retrofitted; banner kind-aware. Specs
§2.4/§6.3/§3.11 updated. 8a unblocks 8b's fail-open.
Chunk 8b — ACME HTTP-01 + mode wiring: new `internal/acme/http01.go`
`HTTP01Solver` (Solver seam + `Handler()`); `internal/tls`
`NewChallengeRedirectServer` (challenge > redirect priority, § 3.3) on port 80;
`main.go` `setupACME` replaces the not-implemented error — always comes up on
HTTPS (stored cert else 8a self-signed degraded), background Renewer +
`promotingIssuer` `ReloadFromPEM`-promotes in place clearing degraded; HTTPS
listener runs `HTTPRedirectPort:0`; staging WARN; `http-01`+`http_redirect_port:0`
and `dns-01` (Chunk 9) fail open to self-signed; `serverResult`/`awaitShutdown`
drain challenge server + renewer cancel. TDD, no network. Specs already accurate.
Chunk 8c — deliberate plain-HTTP CLI: new `cmd/.../tlsmode.go` adds
`tls mode <off|static|acme>` to the 3a dispatch (generalises `tls reset`, now a
thin alias); `--trusted-proxy[=true|false]` on `mode off` also sets
`server.trusted_proxy`. Required wiring: `server.trusted_proxy` is now a
config-store section (`KeyServerTrustedProxy` in `assembly.go` —
AllConfigKeys/assembleFields/ConfigToSections) so the CLI/UI value is read at
startup + reload (it was YAML-only and lost on migration before). Same env-only
store + section-preserving idempotency as 3a. Spec `tls.md § 9.1` (+ `tls-static.md`
pointer). TDD, no network.
Chunk 8d — admin-UI behind-proxy toggle: `AdminServerPage.tsx` "Terminate TLS at
a proxy (plain HTTP)" switch (`setBehindProxy`) sets `tls.mode: off` +
`trusted_proxy: true` (derived on-state `mode off && trusted_proxy`), with an
`X-Forwarded-Proto`-spoof lockout warning when on; `trusted_proxy` surfaced on
`ServerConfig` (`types/config.ts`). Backend fix: the server PUT handler
(`handle_admin_config_server.go`) now also persists `KeyServerTrustedProxy` — it
was assembled by 8c but the PUT key-list dropped it, so a UI value was lost on
reload. TDD (vitest toggle/payload/warning; Go PUT-persists-trusted_proxy).
Chunk 9 — Route53 DNS-01 solver: new `internal/acme/route53.go` `Route53Solver`
(Solver seam over a `route53API` interface — `*route53.Client` in prod, fake in
tests). Present UPSERTs the `_acme-challenge.<domain>` TXT (value double-quoted),
polls `GetChange` to `INSYNC`; CleanUp DELETEs best-effort (no INSYNC wait).
`NewRoute53Solver` resolves region/zone (dns_provider_config → config-store) and
creds (config-store secrets → AWS env/IAM-role default chain, both halves
required for static), no network at construction. New config-store keys
`server.tls.acme.route53.{access_key_id,secret_access_key,region,hosted_zone_id}`
(standalone, excluded from AllConfigKeys). `setupACME` dns-01 case builds the
solver (fails open to self-signed on error / non-route53 provider); port-80
challenge server guarded nil for dns-01. go.mod +aws-sdk-go-v2 subset (noted in
[todo-ci.md](todo-ci.md) § 4). TDD, no real AWS. Specs already accurate.
Chunk 9a — Route53 hostname self-registration: new `internal/acme/hostname.go`
`HostnameRegistrar` (reuses the Chunk 9 Route 53 client/zone via a new
`(*Route53Solver).NewHostnameRegistrar`; shared `pollChangeInSync` extracted
from the solver's `waitInSync`). UPSERTs an A record per `acme.domains` entry to
a resolved IPv4 (resolver: literal `hostname_ip` → named `hostname_interface`
global-unicast IPv4 → default-route auto-detect via a packet-less UDP dial;
explicit-but-unusable = ERROR + skip, no fall-through). Wildcards skipped with
WARN; fail-soft (logs ERROR, never blocks issuance/renewal/fail-open). New
ACMEConfig fields (`RegisterHostname`/`HostnameTTL` default 60/`HostnameInterface`/
`HostnameIP`) + validation (register_hostname requires route53; IPv4-only
`hostname_ip`; ttl 1–86400) + `types/config.ts`. Renewer `WithHostnameRegistrar`
option re-asserts at the top of each cycle (startup + every renewal → corrects a
changed DHCP IP); `setupACME` dns-01/route53 wires it when `register_hostname`.
Two deferred items in [todo-tech-debt.md](todo-tech-debt.md): immediate
re-register on config change, and surfacing hostname errors in TLS status (both
land with the Chunk 10 UI). TDD, no real AWS/network. Spec `tls-acme.md § 3.13`
already accurate.

## Next chunk

**Chunk 10 — ACME UI + AWS creds (Feature 3 frontend).** Render the missing
`dns_provider` + `dns_provider_config` (region, hosted zone) fields, AWS cred
inputs (secret, write-only), dns-01 conditional block, ToS toggle, staging
warning, ACME cert-status panel (issued/expiry/last renewal). Hostname
self-registration (Chunk 9a): `register_hostname` tickbox + an "IP source: Auto /
Interface / Manual IP" selector over `hostname_interface` / `hostname_ip` (+
`hostname_ttl`), shown only for `dns_provider: route53`. Also pick up the two
Chunk 9a tech-debt items (surface hostname registration error in the status
panel; consider an immediate config-change re-register path). Scope:
`AdminServerPage.tsx`, `types/config.ts`, `api/config.ts`, backend cred storage.
Depends on Chunks 9, 9a. See detail plan Chunk 10.

## Notes

- CI supply-chain scanning work shipped in PR #49; remaining follow-ups moved to
  [plans/todo-ci.md](todo-ci.md).
