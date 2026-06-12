# Active — server live listener rebind

Branch: `refactor/config-live-reload`. Spec: `configuration-live-reload.md`
(Listener-Rebind section, updated). Goal: apply `server.*` changes in place
instead of a process re-exec — re-exec is unavailable on unsupervised hosts and
kills in-flight goroutines (test-kitchen runs, collection runs); bind-new-first
makes a rebind strictly safer (a failed bind is a no-op).

Bucket 2 (the restart bugs) is already closed (chunks A–G). This is the Bucket 1
`server.*` downgrade-candidate work, full scope (incl. TLS mode/ACME), approved
2026-06-12.

## Core protocol (all rebind chunks)

**Bind-new-first / keep-old / rollback:** bind the new listener(s) before
retiring the old; only once the new is serving, drain+close the old. On any bind
failure, keep the old serving and return the error to the operator (e.g. "address
already in use" → another process holds the port). Preflight: validate
bindability, but prefer bind-and-keep over test-bind-release (no release race).

**Diff-aware server flag:** the server PUT bundles listen/tls/websocket/graceful.
The handler must detect which sub-keys actually changed and report the worst
granularity *of the changed set* (so a graceful-only change reports applied, a
port change reports listener). Introduced in H2, extended per chunk.

**Seam:** a server controller (new `internal/serverctl` or in `main.go`) owns the
live listener(s) and exposes `Rebind(newServerCfg) (ReloadGranularity, error)`.
Wired to webapi via `WithListenerRebinder`; the server config applier calls it.

## Chunk H1 — `graceful_shutdown_seconds` → applied [DONE]
- `resolveShutdownTimeout()` reads the drain budget live from the holder at
  shutdown time (fallback to boot cfg; ≤0 → 15s); `awaitShutdown` uses it.
- Server PUT is now diff-aware (minimal): `serverReloadGranularity` diffs the
  submitted vs pre-save live sections; graceful-only change → applied (no
  restart), any listen/tls/websocket/trusted_proxy change → process (pessimistic
  until H2–H4). Reports `reload`/`restart_required` like the generic handler.
- Tech debt logged: dead `apptls.ListenerConfig.GracefulShutdownTimeout` field.
- H2 extends the diff: listen key → listener granularity + wire the rebinder.

## Chunk H2 — listener ownership + `listen_address`/`port` rebind [DONE]
- `internal/serverctl.Controller`: owns the live `Instance` (Addr+Shutdown),
  `Rebind(addr,port)` binds-new-first via an injected `BuildFunc`, swaps current,
  drains the old in the background (live graceful budget). No-op on unchanged
  target; bind failure keeps the old serving + returns the error.
- webapi `ListenerRebindHolder` (func-based, no import cycle) +
  `WithListenerRebinder`; `ErrNoListenerRebinder` sentinel. Wired up front like
  `tlsReload`.
- Server PUT diff-aware: listen key changed → call rebinder. Success → report
  `listener` (no restart); no rebinder wired → `process` (restart_required);
  bind error → 500 (old keeps serving). `serverReloadGranularity` now takes the
  resolved listen granularity; static map keeps tls/websocket/trusted_proxy.
- main: holder created in `setupAndServeHTTP`; plain-`off` mode adopts a plain
  controller, healthy static-TLS-on-configured-port adopts a TLS controller
  (BuildFunc rebuilds `apptls.Listener` at the new target, re-points `tlsReload`
  + re-arms cert watch; db source refetches cert from store so a prior hot-swap
  is preserved).
- **Scope deferred to H4** (restart_required until then — holder simply not
  adopted, so the no-rebinder path applies): active auto-443 lifeboat
  (`https443Ln != nil`), ACME mode, and the degraded self-signed/plain
  fallbacks. These need the full listener-topology rebuild that H4 owns.
- TDD: serverctl protocol (rebind across ephemeral ports, old drained, forced
  bind-failure keeps old); handler (listener/process/500); main (plain rebind +
  bind-failure). All under `-race`.

## Chunk H3 — `server.websocket.*` → subsystem [hub rebuild] [DONE]
- `EventHub.Reconfigure(max, buf)`: max_connections/send_buffer_size now atomic
  and reconfigured live. Lowering max never evicts existing clients (new
  registrations rejected until count drops below the new ceiling); buffer change
  sizes only clients registered after the call. Non-positive → unchanged.
- Handler timeouts (write/ping/pong) pulled live via `WithWebSocketConfigFunc`
  (router closes over `liveConfig()`); resolved once per connection, so existing
  connections keep their started values — graceful.
- Server PUT diff-aware: websocket key changed → `r.hub.Reconfigure(...)` from the
  reloaded live config (defaulted) → reports `subsystem` (no restart). The hub is
  router-owned, so no holder/seam is needed and it always applies (never
  restart_required). `serverKeyGranularity[websocket]` flipped process→subsystem.
- main: hub now created with `WithMaxConnections/WithSendBufferSize` from
  `server.websocket.*` (was ignoring config at boot — pre-existing gap, fixed).
- TDD: hub reconfigure (live max/buf, non-positive no-op, lowered-max keeps
  existing + rejects new, buffer applies to new only); handler resolveConfig
  (live/static/default); handler PUT → subsystem + hub reconfigured. All `-race`.

## Chunk H4 — TLS mode transitions + ACME port rebind [largest / riskiest]
Split into sub-chunks (one session each), static↔off first, then static
topology, then acme.

### H4a — off↔static mode transition [config-driven seam] [DONE]
- **Seam generalised.** serverctl.Controller becomes mode-agnostic: target is an
  opaque `key` string + a per-call `build func() (*Instance, error)` (was stored
  BuildFunc + addr/port). No-op iff key unchanged; bind-new-first / drain-old
  protocol is unchanged. main owns key construction (`plain|addr:port` /
  `tls|addr:port`) and the build closure (plain vs static-TLS).
- **webapi seam** `ListenerRebindHolder.Apply(cfg config.ServerConfig)` (was
  `Rebind(addr,port)`): carries the full desired server config so the applier can
  rebuild either listener type. `ErrNoListenerRebinder` unchanged.
- **main**: one controller adopted at boot (plain-off, or static single-listener
  via the existing guard). Applier dispatches on `cfg.TLS.Mode`: off→plain,
  static→static-TLS (single HTTPS listener on the configured port). `buildTLSInstance`
  now assembles its `ListenerConfig` from the passed `cfg` + `loadDBCertKey` (db),
  not a boot `baseCfg`. Refuse (→ ErrNoListenerRebinder, restart_required) when
  target mode is `acme` or static with `http_redirect_port != 0` — deferred to
  H4b/H4c.
- **handler**: detect a normalised mode change (off↔static); trigger Apply when
  listen section OR mode changed. Drop `KeyServerTLS` from `serverKeyGranularity`;
  fold a resolved `tlsGran` into `serverReloadGranularity` like `listenGran`
  (mode-toggle applied in place → listener; other tls sub-changes still process).
- **Same-port toggle applies live** (no spec caveat needed). bind-new-first can't
  bind a port the old listener still holds, so for a same-address:port variant
  change `applyServerListener` instead: (1) constructs the new TLS listener WITHOUT
  binding (`newTLSListener` → validates the cert — a bad cert fails here with the
  old listener untouched), then (2) `Controller.RebindInPlace` releases the old
  listener and binds the new on the freed port, with a bind retry (`listenTCP`,
  ~2s) to absorb the OS release lag (SO_REUSEADDR reclaims it). A port-changing
  toggle still uses bind-new-first (`Rebind`). Residual: a post-release bind
  failure (non-physical after validation + we held the port) would briefly leave
  it down until restart — accepted over the socket-reuse complexity.
- **Deferred to H4b** (logged in todo-tech-debt): auto-443 re-plan +
  http_redirect_port topology; same-mode static topology changes (min_version /
  mTLS CA / redirect); static→off does not clear the stale tlsReload pointer.
  Off→static via H4a serves a single HTTPS listener on the configured port (no 443
  lifeboat); static→off only rebinds when the static boot was the adopted
  single-listener case.
- TDD: serverctl key no-op + cross-variant rebuild + RebindInPlace (drain-then-build,
  build-failure-leaves-nothing); webapi Apply seam; main off↔static rebind across
  ports + same-port toggle applies live + same-port bad-cert-keeps-old +
  bind-failure-keeps-old + unsupported-target refusal; handler mode-toggle →
  listener, non-mode tls change → process, no-rebinder → process. All `-race`.

### H4b — static topology changes (no mode change)
Split into sub-chunks (one session each); single-listener field changes first,
then the multi-listener topology (redirect/443).

#### H4b-1 — single-listener static field changes [config-fingerprint key] [DONE]
- **Trigger generalised.** The server PUT now drives the in-place rebinder on ANY
  tls-section change (not just an off↔static mode toggle or a listen change), so a
  same-mode static field change reaches `applyServerListener`. `modeChanged` +
  `preSaveMode` + `normTLSMode` dropped; the byte-diff `tlsSectionChanged` is the
  trigger and the mode comparison is subsumed (mode lives in the tls section).
- **Topology-fingerprint key.** `serverListenerKey` gains a third `|`-segment from
  `tlsTopologyFingerprint(cfg)` (static: cert source/cert/key/ca paths + min_version,
  normalising cert_source ""→"file"; empty otherwise), so a same-port change to
  min_version / mTLS ca_path / cert source-or-paths yields a different key and rebinds
  in place (same addr:port → `RebindInPlace`, validate-then-rebind). `keyListenTarget`
  now extracts the middle addr:port segment.
- **Scope.** Applies on adopted single-listener static deployments (port 443, or the
  degraded-443 fallback — `https443Ln == nil && http_redirect_port == 0`). The auto-443
  lifeboat and an explicit http_redirect_port are still refused (→ ErrNoListenerRebinder,
  restart_required) — deferred to H4b-2/H4b-3.
- TDD: main (static min_version applies live across a same-port rebind + TLS1.2-max
  client then rejected; unchanged static save is a no-op/applied); handler (same-mode
  static min_version change → rebinder called once → listener; no rebinder → process).
  All `-race`.

#### H4b-2 — http_redirect_port topology [DONE]
- **Pre-bound redirect seam.** `apptls.Listener.SetRedirectListeners([]net.Listener)`
  (the redirect analogue of `SetHTTPSListener`): when set, `Serve` serves each
  redirect server on its pre-bound listener (`srv.Serve(ln)`) instead of
  `ListenAndServe`, so the live rebuild can reclaim a redirect port the old process
  still holds across a same-target drain via a retrying bind.
- **Applier.** `newTLSListener` now sets `HTTPRedirectPort` from cfg;
  `serveTLSListener` pre-binds each `RedirectAddrs()` port via `listenTCPTarget`
  (new, retry-capable; `listenTCP` refactored onto it) with the same attempts as the
  HTTPS port, then `SetRedirectListeners` — a bind failure unwinds everything bound.
  The static `http_redirect_port != 0` refusal in `applyServerListener` is removed
  (only ACME is still refused).
- **Fingerprint.** `tlsTopologyFingerprint` gains `;redirect=%d`, so a same-port
  add/remove/change of `http_redirect_port` yields a new key → `RebindInPlace` (HTTPS
  port unchanged). A redirect-only change still tears down + rebinds the HTTPS
  listener (controller swaps the whole Instance) — accepted, consistent with the H4a
  residual.
- **Boot adopt.** Guard relaxed from `https443Ln == nil && http_redirect_port == 0`
  to `https443Ln == nil`: the single-HTTPS-on-configured-port topology (server.port
  == 443, or the 443-not-bindable fallback) with an explicit redirect is now adopted.
  The auto-443 lifeboat (`https443Ln != nil`) is still NOT adopted — H4b-3.
- TDD: apptls (pre-bound redirect served + 301s); main (add / change / remove
  redirect → listener, old redirect drained; min_version change with redirect kept →
  both ports reclaimed via retry); handler (http_redirect_port change → rebinder
  called once with the port → listener). `RefusesUnsupportedTargets` trimmed to ACME
  only. All `-race`.

#### H4b-3 — auto-443 lifeboat re-plan [DONE]
- **State.** `serverApp.autoHTTPSActive bool` + `autoHTTPSPort int`, set at boot when
  `https443Ln != nil` (`autoHTTPSPort` = the actual bound lifeboat port via
  `listenerPort()` — 443 in prod, a free port under the `auto443Listen` test seam).
  The static serve branch now ALWAYS adopts the controller (the `https443Ln == nil`
  guard is dropped); auto-443-ness is carried forward, never re-probed (a same-target
  RebindInPlace drains the old listener async, so a re-probe would see 443 still held
  and wrongly downgrade).
- **Key.** `app.listenerKey(cfg)` method: when `autoHTTPSActive && mode==static`,
  `tls443|<addr>:<autoHTTPSPort>|<tlsTopologyFingerprint>;cfgport=<port>` — the HTTPS
  target is the lifeboat port (so a server.port change keeps the same target →
  RebindInPlace swaps the redirect) and the configured port is folded into the
  fingerprint (server.port / http_redirect_port / min_version / CA / cert change →
  new key). Else delegates to the unchanged `serverListenerKey`. `adoptListenerController`
  + `applyServerListener` now key via this method.
- **Topology.** `app.effectiveTLSTopology(cfg) (httpsPort, redirectPorts)`: auto-443
  → `(autoHTTPSPort, [server.port, http_redirect_port])`; else `(port,
  [http_redirect_port])`. `newTLSListener` signature now `(…, httpsPort,
  redirectPorts)`, setting `Port=httpsPort` + `RedirectPorts=redirectPorts`
  (`HTTPRedirectPort` dropped — folded into RedirectPorts, equivalent for the
  non-auto path). `serveTLSListener` already pre-binds `RedirectAddrs()` with retry,
  so it reclaims the lifeboat port + every redirect on a same-target drain unchanged.
- **Applier refusal.** When `autoHTTPSActive`, refuse (→ ErrNoListenerRebinder,
  restart_required) any target that leaves the auto-443 topology: `mode != static`
  or a listen-target (listen_address) change. Surviving cases are same-lifeboat-target
  static changes → always RebindInPlace.
- **Residual (todo-tech-debt):** leaving auto-443 (static→off/acme) and a
  listen_address change stay restart-required; an off/acme→auto-443 upgrade is not a
  thing (auto-443-ness is boot-only). H4c owns ACME.
- TDD (all `-race`): main — auto-443 min_version applies live (HTTPS still on the
  lifeboat port, redirect kept, TLS1.2 then rejected); server.port change re-plans
  the redirect (old drained, new serves, HTTPS unchanged); add http_redirect_port
  applies live; unchanged save → applied no-op; mode→off refused (restart_required).
  New `adoptAuto443Boot` helper mirrors the boot lifeboat topology.

### H4c — ACME transitions + ACME port rebind [riskiest]
ACME mode owns THREE long-lived resources (HTTPS listener + renewer goroutine +
port-80 challenge/redirect server) vs the controller's single-`Instance` model,
so split into two sessions: first fold the topology into one rebindable Instance
(no new transitions), then enable the transitions.

#### H4c-1 — ACME topology → composite serverctl.Instance + boot-adopt [refactor] [DONE]
- `acmeRuntime{listener, challengeSrv, renewerCancel}` + `shutdown(ctx)`
  (cancel renewer → shutdown challenge → drain HTTPS, all nil-guarded). setupACME
  builds it and wraps it in one `serverctl.Instance` (Shutdown = rt.shutdown).
- Boot adopts that Instance into the controller (new `acme|<addr:httpsPort>|<fp>`
  key branch in `listenerKey`; `app.acmeActive bool` set like `autoHTTPSActive`).
  When the rebinder is wired the controller owns teardown → boot serverResult
  drops challengeSrv/renewerCancel (awaitShutdown drains via the controller, as
  for static/off); no rebinder → keep them for the fallback teardown path.
- Applier still refuses EVERY acme transition (entry `cfg.TLS.Mode=="acme"`, and
  leaving via `app.acmeActive`) → ErrNoListenerRebinder (restart_required). No
  operator-visible change — pure scaffolding so all existing boot/shutdown tests
  still pass and H4c-2 has the swap seam.
- TDD: setupACME with rebinder wired adopts the controller; composite Shutdown
  cancels renewer + stops challenge + closes HTTPS; acme→off/static Apply refused
  (acmeActive); existing acme boot/degraded/auto-443 tests unchanged. All `-race`.

H4c-2 is large + riskiest, so split by direction: exit first (reuses H4c-1's
tested composite teardown + existing plain/static builders), then entry + the
on-demand acme rebuild.

#### H4c-2a — acme EXIT (acme→off / acme→static) [in-place] [DONE]
- Applier stops refusing when leaving acme (`app.acmeActive && mode != acme`):
  builds the off/static target via the existing plain/static closures and swaps,
  so the old acme Instance.Shutdown (H4c-1) tears down renewer + challenge +
  HTTPS as a unit. Entry (`mode == acme`) and acme-internal saves stay refused
  (H4c-2b).
- **Always release-first (`RebindInPlace`).** The live acme topology holds several
  ports (HTTPS + port-80 challenge + any redirect), so drain the whole Instance
  first, then bind the replacement on the freed port(s) with the existing retry —
  bind-new-first is unsafe (the new listener could clash with a port the old
  topology still holds). Residual (todo-tech-debt): a clean-port exit could
  bind-new-first; not worth the multi-port-overlap bookkeeping.
- Clear `app.acmeActive` on a successful exit so subsequent saves use normal
  off/static logic. Stale tlsReload pointer on acme→off mirrors the H4a residual.
- TDD: acme(degraded http-01)→static on a new port + same port (reclaim via
  retry) → listener, new HTTPS serves, old acme HTTPS/challenge stop; acme→off →
  listener, plain serves; acmeActive cleared; entry + acme-internal still refused.
  All `-race`.

#### H4c-2b — acme ENTRY + internal/port change [in-place]
- Applier dispatches an acme target via an on-demand `buildACMEInstance` closure
  (refactored out of setupACME, fail-open to degraded self-signed inside the new
  Instance), for off/static→acme entry and acme-internal changes (domains/email/
  challenge/ca_url/dns/min_version/http_redirect_port/server.port) via an acme
  fingerprint key → rebuild. Renewer cancel/restart through the Instance swap;
  `acmeTrigger` repointed to the new renewer each rebuild; port-80 challenge
  pre-bound with retry (like redirect ports) so a same-port rebind reclaims it;
  an HTTPS-port-only change keeps the challenge reachable. Auto-443 in acme +
  fail-open (degraded) interactions reconciled; deferrals → todo-tech-debt.

## Notes / cross-branch
- `auth.*` is listed in this spec's rebind section as subsystem; on this branch
  it is NOT yet implemented (done on `fix/saml-config-ux`). At merge, reconcile —
  the SAML branch must also flip `configuration-live-reload.md` §auth.
- Each chunk = one session (start fresh; read only this plan + the touched code).
