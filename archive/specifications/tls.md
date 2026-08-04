# TLS and Certificate Management - Component Specification

> TLS termination and certificate lifecycle management for Chef Migration Metrics.

## TL;DR

Three listening modes: **plain HTTP** (`mode: off`), **static TLS**
(`mode: static`, operator-provided cert/key from **file or DB**), and **ACME
automatic** (`mode: acme`, Let's Encrypt / ZeroSSL with HTTP-01 or DNS-01). Static
cert/key may live on disk (`cert_source: file`) or encrypted in the config store
(`cert_source: db`, UI-driven). The product can also **generate a CSR** in-app for
externally-signed static certs. ACME uses the low-level `golang.org/x/crypto/acme`
client with DB-backed (encrypted) state and Route 53 DNS-01 via `aws-sdk-go-v2`.
Supports cert reload, HTTP-to-HTTPS redirect, and HSTS. Fail-open to plain HTTP on
any TLS failure; lockout recovery is the **repair CLI**. Backward compatible with
deprecated boolean `tls.enabled`.

This spec is split for targeted retrieval:

| File | Contents |
|------|----------|
| [tls-static.md](tls-static.md) | § 2 Static mode — file & DB cert/key, mTLS, reload, fail-open, preflight. |
| [tls-acme.md](tls-acme.md) | § 3 ACME — `x/crypto/acme`, HTTP-01, Route 53 DNS-01, DB storage, renewal. |
| [tls-csr.md](tls-csr.md) | § 4 In-app CSR generation and signed-cert promotion. |

## 1. Listening Modes

### 1.1 Mode Selection

| `server.tls.mode` | Behaviour |
|--------------------|-----------|
| `off` (default) | Plain HTTP on `server.port`. No encryption. |
| `static` | HTTPS on `server.port` using an operator-provided cert/key (file or DB; see [tls-static.md](tls-static.md)). |
| `acme` | HTTPS on `server.port` using certificates obtained automatically via ACME (see [tls-acme.md](tls-acme.md)). |

### 1.2 HTTP-to-HTTPS Redirect

When TLS is active (`mode: static` or `mode: acme`), an optional secondary listener can serve HTTP-to-HTTPS redirects:

| Setting | Default | Description |
|---------|---------|-------------|
| `server.tls.http_redirect_port` | `0` (disabled) | When set to a valid port (e.g. `80`), the application starts a secondary HTTP listener that responds to all requests with a `301 Moved Permanently` redirect to the HTTPS equivalent URL. |

The redirect listener serves **only** redirects — no API responses, no static assets, no health checks. This prevents accidental exposure of sensitive data over plain HTTP.

`http_redirect_port` must differ from `server.port` (the HTTPS listen port) — and, when automatic HTTPS on `443` is in effect (§ 1.5), from `443` as well. If any two listeners would bind the same port, one would fail at startup. This is rejected by validation — at startup and at save time (see [tls-static.md § 2.6](tls-static.md)).

**Exception:** The ACME HTTP-01 challenge path (`/.well-known/acme-challenge/`) is served on the redirect listener when `mode: acme` and the HTTP-01 solver is in use (see [tls-acme.md § 3.3](tls-acme.md)).

### 1.3 Port Defaults

The configured `server.port` default remains `8080` regardless of TLS mode — the
*configured value* never changes when toggling TLS on and off. However, when TLS is
active and healthy the application automatically also serves HTTPS on `443` and
redirects the configured `server.port` to it (§ 1.5), so an operator who leaves
`server.port: 8080` still reaches the standard HTTPS port without reconfiguring,
and an existing `http://host:8080` bookmark still resolves (via redirect) rather
than breaking. Operators may still set `server.port: 443` explicitly, in which case
there is nothing to redirect.

### 1.4 Binding Privileged (Low) Ports

Binding a port below `1024` (e.g. `80`/`443`) requires elevated privilege. The
service runs non-root, so two OS layers must each permit the bind:

- **Capability:** the process needs `CAP_NET_BIND_SERVICE`. The packaged systemd
  unit grants this via `AmbientCapabilities` (compatible with
  `NoNewPrivileges=true`; file-based `setcap` is silently ignored under
  `NoNewPrivileges`). For a manual/dev run, grant it on the binary with
  `setcap cap_net_bind_service=+ep <binary>`.
- **SELinux:** on an enforcing host the target port must carry a permitted label.
  `80`/`443` are already `http_port_t`; a non-standard low port must be labelled
  with `semanage port -a -t http_port_t -p tcp <port>`.

When a bind is denied by the OS (permission error), the server does not
crash-loop: it logs a precise, actionable remediation message naming both layers
and the affected port, then falls back to the next listen candidate and runs in
degraded mode (§ 6.3, and [tls-static.md § 2.4](tls-static.md)). Capability vs
SELinux are distinct — both must be satisfied for a non-standard low port on
enforcing RHEL.

### 1.5 Automatic HTTPS on 443 (port lifeboat)

When TLS is active (`mode: static` or `mode: acme`) **and** the TLS listener builds
successfully (healthy — not the fail-open path of [tls-static.md § 2.4](tls-static.md)
or [§ 6.3](#63-degraded-tls-status-and-recovery)), the application **automatically**:

- binds the HTTPS listener on `443`, and
- starts a secondary listener on the configured `server.port` that `301`-redirects
  every request to the `443` HTTPS URL.

So enabling TLS yields the standard HTTPS port with no extra configuration, while
the previously-used `server.port` URL keeps working via redirect. This is the
**conventional half** of the "port lifeboat" idea.

- **`server.port` already `443`:** there is nothing to redirect — the app serves
  HTTPS on `443` directly, no secondary listener.
- **Privileged bind (§ 1.4):** `443` is a low port. If it cannot be bound (no
  `CAP_NET_BIND_SERVICE`, or SELinux), the application does **not** crash — it logs
  the § 1.4 remediation and **falls back to serving HTTPS directly on
  `server.port`** with no `443` listener and no redirect.
- **Degraded TLS:** when the listener cannot be built and the app falls open
  (self-signed or last-resort plain, § 6.3), `443` is **not** bound — the fail-open
  listener holds `server.port` exactly as today. `443` is bound only when TLS is
  healthy at startup.
- **Interaction with `http_redirect_port`:** when set, that HTTP→HTTPS redirect
  targets the `443` URL. `http_redirect_port` must differ from **both**
  `server.port` and `443` (validated at startup and save time, § 1.2).

**ACME mode.** Automatic HTTPS on `443` applies to `mode: acme` on the same
"healthy at startup" condition, with one wrinkle: in `http-01` the port-80
challenge/redirect listener ([tls-acme.md § 3.3](tls-acme.md)) *is* the redirect
listener.

- **Healthy (a real issued certificate is loaded at startup):** HTTPS binds
  `443`. In `http-01`, the port-80 listener serves the challenge path as always
  but `301`-redirects ordinary traffic to the `443` URL (not `server.port`). As
  in static mode, a secondary listener on `server.port` also redirects to `443`
  (skipped when `server.port` is already `443`). `dns-01` has no port-80 listener
  — only the `server.port` → `443` redirect.
- **Degraded bootstrap (no certificate yet → ephemeral self-signed):** this is
  the fail-open path, so `443` is **not** bound — HTTPS (self-signed) holds
  `server.port` and the port-80 listener redirects to `server.port`, exactly as
  today. When the renewer later obtains and promotes a real certificate **in
  place**, the listener does **not** move to `443` at runtime (no runtime
  port-flip — see *Out of scope* below); `443` is reconsidered only on the next
  restart, which now starts healthy.
- **`443` bind failure / `server.port` already `443`:** identical to static mode
  above.

> **Out of scope (future):** *runtime* health-driven port movement — flipping
> between `443` and `server.port` as TLS health changes at runtime, with a graceful
> listener hot-swap and flap hysteresis — is **not** part of this behaviour. `443`
> binding is decided once, at startup, from TLS health. See the deferred
> "443 lifeboat — health-driven port move" item.

## 2. Static Certificate Mode

Operator-provided cert/key, sourced from disk (`cert_source: file`) or the
encrypted config store (`cert_source: db`). Covers the certificate chain, reload,
fail-open startup, save-time preflight, mTLS, and the DB storage model. **Full
detail: [tls-static.md](tls-static.md).**

## 3. ACME Automatic Certificate Management

Automatic issuance/renewal via `golang.org/x/crypto/acme`, with HTTP-01 and
Route 53 DNS-01 challenges, DB-backed encrypted state, renewal scheduling, and
fail-open. **Full detail: [tls-acme.md](tls-acme.md).**

## 4. CSR Generation

In-app keypair + CSR generation for externally-signed static certificates; the
private key never leaves the server, and a signed cert is matched against the
pending key and promoted. **Full detail: [tls-csr.md](tls-csr.md).**

## 5. TLS Configuration Details

### 5.1 Cipher Suites

The application relies on Go's `crypto/tls` default cipher suite selection — secure, well-maintained, and intentionally not exposed to avoid misconfiguration. TLS 1.3 suites are fixed by the protocol. TLS 1.2 defaults prefer ECDHE with AEAD ciphers (AES-GCM, ChaCha20-Poly1305).

### 5.2 HSTS Header

When TLS is active, include on all HTTPS responses:

```
Strict-Transport-Security: max-age=63072000; includeSubDomains
```

The `max-age` of 2 years follows current best practice. Never sent on HTTP redirect responses.

## 6. Interaction with Other Components

### 6.1 Health Checks

The health check endpoint (`/api/v1/admin/status`) is served on the main listener. Kubernetes probes must use HTTPS or skip TLS verification (`scheme: HTTPS`). The `healthcheck` CLI subcommand must support `--insecure` for HTTPS without verification.

### 6.2 Logging

TLS-related events use the `tls` log scope. Key events:

- **INFO:** mode selected, certificate loaded/reloaded, ACME certificate obtained/renewed, redirect listener started.
- **WARN:** certificate expiring soon (within 7 days), staging CA URL detected.
- **ERROR:** certificate reload failed (continues with previous cert), ACME renewal failed (includes current expiry), ToS not accepted, TLS failed at startup → fell back to plain HTTP (§ 6.3).

### 6.3 Degraded TLS Status and Recovery

When startup falls open — static cert unloadable
([tls-static.md § 2.4](tls-static.md)) or ACME cannot obtain a cert
([tls-acme.md § 3.11](tls-acme.md)) — the application serves an **ephemeral
self-signed certificate** over HTTPS (or, only if that itself fails, plain HTTP)
so the recovery UI stays reachable. The degraded state is published on a public,
DB-independent endpoint so the UI can warn on every page, including before login:

```
GET /api/v1/server/tls-status   →   200 OK
{ "degraded": true, "kind": "self-signed", "reason": "TLS listener setup failed: <cause>" }
```

`kind` is `self-signed` (degraded HTTPS with an untrusted cert) or `plain`
(last-resort cleartext). When TLS is healthy (or `mode` is `off`), the endpoint
returns `{ "degraded": false }`. The endpoint requires no authentication and never
queries the database, so the banner renders even when other subsystems are down.
The `reason` never contains private key material.

The frontend shows a prominent global banner whenever `degraded` is true:
**"TLS degraded — serving an untrusted self-signed certificate (or plain HTTP).
Fix the certificate and restart: <reason>"**. The Server & TLS admin page surfaces
the same state inline.

**Recovery boundary: host access.** Once TLS material lives in the DB
(`cert_source: db`, ACME state, or `ca_path`), the old "move the cert/key/CA file
on the host" recovery no longer applies. The escape hatch is a **repair CLI** run
on the host, which needs the host-side `DATABASE_URL` and
`CMM_CREDENTIAL_ENCRYPTION_KEY` — the same access boundary as before. There is no
break-glass environment-variable or flag override; the deprecated anti-lockout env
override is a documented limitation, not the recovery lever.

Repair CLI subcommands (host-side):

| Command | Effect |
|---------|--------|
| `tls reset` | Sets `server.tls.mode: off` in the config store. Recovers any mode (bad DB cert, mTLS lock, stuck ACME) by returning to plain HTTP. |
| `tls clear-ca` | Removes `server.tls.ca_path` / the CA entry. Recovers an mTLS lockout while keeping TLS on. |

**Recovery scenarios:**

1. **Bad/missing static cert** (file or DB): the listener fails to build and falls
   open automatically to a **self-signed** HTTPS listener
   ([tls-static.md § 2.4](tls-static.md)). Fix the cert via the UI (save-time
   preflight rejects an unusable pair) and restart, or run `tls reset` to force
   plain HTTP.
2. **mTLS lockout** (a `ca_path` that rejects all clients at the handshake, before
   any login): the listener builds successfully so fail-open does **not** trigger.
   Run `tls clear-ca` on the host, then restart — the UI is reachable to correct
   or re-set `ca_path`.
3. **ACME cannot issue** (ToS, DNS creds, CA unreachable, no cert yet): falls open
   to a **self-signed** HTTPS listener ([tls-acme.md § 3.11](tls-acme.md)); fix
   the ACME config via the UI, or `tls reset` to force plain HTTP. If a valid ACME
   cert is later issued, the renewer swaps it in **without a restart**.

There is no in-place runtime recovery for a held port — the degraded (self-signed
or last-resort plain) listener holds it until the process restarts. The degraded
state clears on restart with working TLS, or — for the DB static source and ACME —
when a valid certificate is saved/issued and the listener reloads in place, at
which point HSTS resumes.

## 7. Backward Compatibility

The previous schema used a boolean `server.tls.enabled` field:

```yaml
server:
  tls:
    enabled: true
    cert_path: /path/to/cert.pem
    key_path: /path/to/key.pem
```

- If `tls.enabled: true` and `tls.mode` is not set → treat as `mode: static`, log `WARN` about deprecation.
- If `tls.enabled: false` (or absent) and `tls.mode` is not set → default `mode: off`.
- If both are present → `tls.mode` wins, `tls.enabled` is ignored with a `WARN`.

`cert_source` defaults to `file`, so an upgraded deployment with file-based
cert/key paths keeps working with no config change.

## 8. Security Considerations

### 8.1 Private Key Protection

- File-based private keys (static `cert_source: file`) must have permissions no
  more permissive than `0600`. Log `WARN` if more permissive.
- DB-stored private keys (static `cert_source: db`, CSR pending keys, ACME keys)
  are stored `secret: true` and encrypted at rest (AES-256-GCM, HKDF, per-row
  nonce, AAD; master key `CMM_CREDENTIAL_ENCRYPTION_KEY`).
- Private keys must never be logged, included in error messages, or exposed via
  any API endpoint. APIs return certificate metadata only (subject, SANs, expiry).

### 8.2 Rate Limits

Let's Encrypt rate limits:

| Limit | Value |
|-------|-------|
| Certificates per Registered Domain | 50 per week |
| Duplicate Certificate | 5 per week |
| Failed Validation | 5 per account, per hostname, per hour |

The application must not request certificates unnecessarily. DB-backed ACME
storage ([tls-acme.md § 3.5](tls-acme.md)) is critical to avoid hitting rate
limits on restart.

## 9. Plain HTTP Configuration Reference

```yaml
server:
  port: 8080
  tls:
    mode: off
```

Static and ACME configuration references live in
[tls-static.md § 2.8](tls-static.md) and [tls-acme.md § 3.12](tls-acme.md).

### 9.1 Behind a TLS-Terminating Proxy

`mode: off` is also the **intended** deployment when a load balancer or reverse
proxy terminates TLS in front of the application (distinct from the fail-open
degraded state in § 6.3, which is an error condition). The app serves plain HTTP
on the private network between the proxy and itself; the proxy presents the
public certificate.

In this topology set `server.trusted_proxy: true`. The application then trusts
the `X-Forwarded-Proto` header to determine whether the original request arrived
over TLS, so HSTS is emitted (and secure-cookie / scheme detection behaves
correctly) for proxied HTTPS requests even though the local listener is plain
HTTP. Leave it `false` (the default) whenever the app is directly reachable, or a
client could spoof `X-Forwarded-Proto`.

```yaml
server:
  port: 8080
  trusted_proxy: true
  tls:
    mode: off
```

The repair CLI sets both in one step (the value lives encrypted in the config
store; `trusted_proxy` is read at startup and on config reload):

```
chef-migration-metrics tls mode off --trusted-proxy
```

`tls mode <off|static|acme>` is the deliberate-deployment form of the
recovery-framed `tls reset`; `--trusted-proxy[=true|false]` toggles
`server.trusted_proxy`. Restart to apply. See § 6.3 for the recovery commands.

## Related

- [tls-static.md](tls-static.md) — static cert/key (file & DB), mTLS, preflight.
- [tls-acme.md](tls-acme.md) — ACME issuance/renewal, Route 53 DNS-01.
- [tls-csr.md](tls-csr.md) — in-app CSR generation.
- [configuration-schema-server.md](configuration-schema-server.md) — server/TLS config fields.
- [configuration-validation.md](configuration-validation.md) — save-time preflight.
- [secrets-storage.md](secrets-storage.md) — config-store encryption used for DB cert/key.
