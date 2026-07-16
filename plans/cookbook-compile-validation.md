# Cookbook compile-validation — research / design note

Status: **researched and PARKED, not approved.** Captured 2026-07-16 from a design
thread so it is not lost. Distinct from the cop-blocker harness
(`scripts/cop-validation/`, [[cmm-validation-box]]) — that validates the *cop set*;
this is about validating *customer cookbooks* against the target Chef.

## Verdict (revised)

**Don't build a bespoke compile sandbox. The viable middle tier is containerised
Test Kitchen (dokken-style) on a daemonless, rootless, no-forwarding runtime.**

The dead end was thinking we'd assemble a fake environment from a bare namespace —
that hits the environmental-fidelity wall (missing `/etc/passwd` for
`user`/`group`, systemd for `service`, package DB, filesystem, platform…), and the
more you bolt on the more it just becomes a VM. But **curated OS images already bake
all of that in** — that is what the Chef **dokken** images are (systemd shim,
users, packages, platform), and **kitchen-dokken** is the established fast
container-converge driver. So fidelity is a solved problem via curated images; the
real converge sidesteps every compile-time wart because actions actually run in a
real-enough OS.

The only reason it looked blocked at the customer is *delivery*: Docker's daemon +
bridge need `ip_forward` (the waiver). That is separable — run the same dokken
images **rootless and daemonless with userspace networking**:

- **Rootless podman** (default `slirp4netns`/`pasta`): outbound networking is done
  in *userspace*, so it needs **no host `ip_forward`** and no bridge → no waiver, no
  daemon. Needs unprivileged userns (open on [[cmm-validation-box]]) + cgroups v2.
- Or `systemd-nspawn --private-network` / unpack the image rootfs under the
  namespace launcher, for a no-network converge with pre-vendored deps.

This is essentially "make CMM's existing Test Kitchen tier use a rootless-podman /
dokken driver," reusing the kitchen fixtures it already ingests — not a new
subsystem. It is a **real converge** (faithful), not a compile hack.

Remaining real constraints (validate before committing):
- **Windows** — a Linux kernel can't run Windows containers, so Windows cookbooks
  still need a Windows converge target (Windows TK VM/container on Windows). Genuine
  unsolved gap for the container path.
- **Unprivileged userns** must be enabled on the customer host (the analog of the
  `ip_forward` control); if clamped, run with privilege in CMM's service context.
- Deps/network: `slirp4netns` gives outbound (e.g. to an internal mirror) without
  `ip_forward`; a fully offline `--network none` run needs pre-vendored deps.

The material below (warts, fixtures, node-seeding) still applies as the *why* and
the fixture strategy; the compile-only bare-sandbox framing is superseded by this
rootless-container-converge verdict.

## Intent

A cheap **dynamic** migration-readiness signal sitting between cookstyle (static)
and Test Kitchen (real converge): catch breakage cookstyle can't see — won't parse,
unresolvable resource, a removed symbol on an always-reached code path — without
provisioning a TK VM. It is a **pre-filter**, never a verdict.

## Tiering (each shares the same kitchen fixtures)

1. **cookstyle** — static, instant, no fixtures, pattern-based. The core signal.
2. **Sandboxed compile** — `chef-client --local-mode` (chef-zero) COMPILE against
   the target Chef, run in a Linux-namespace sandbox, seeded from the cookbook's
   kitchen suite. Cheap (seconds, no VM), realistic fixtures, catches gross breakage.
   **Linux cookbooks only** (see Windows below).
3. **Test Kitchen converge** — real converge on a real VM. Faithful; already CMM's
   converge signal. The honest "will it converge on 19" answer.

## Warts (all real — do not re-pitch around these)

1. **ChefSpec/compile-time Ruby executes for real.** ChefSpec stubs *resource
   actions* only; recipe/library/attribute Ruby and triggered `ruby_block`s run.
   `File.write`/`shell_out`/network at compile time actually happen → not hermetic →
   needs a sandbox for untrusted cookbooks.
2. **Stubbed actions break state chains.** A stubbed "create /tmp/foo" leaves no
   file, so a later resource/guard depending on it misbehaves → *false fails*; and
   because actions never run, a step that would fail at real converge *passes* →
   *false passes*. So ChefSpec/compile ≠ converge oracle.
3. **Attribute/fixture dependence.** Recipes read `node[...]` at compile time to name
   resources / pick branches / decide `include_recipe`. Only the cookbook's own
   `attributes/` defaults load; roles, environments, wrapper cookbooks, data bags,
   and `search` do not → nil errors or a *different* resource collection than prod.
4. **systemd / init dependence.** A bare namespace sandbox has no PID 1 systemd, no
   `systemctl`, no D-Bus. `service` resources and any compile-time `systemctl`
   shell-out fail. Mostly a *converge* problem (compile only declares the resource),
   but compile-time shell-outs to the init system are possible → treat such failures
   as sandbox noise, not a real migration signal.

## Design decisions

- **Sandbox = Linux namespaces, no packet forwarding.** The customer blocks enabling
  `ip_forward` without a security waiver (granted ~only for OpenShift nodes). That
  control is about *bridge/NAT routing* — container *networking*. Safety sandboxing
  wants the opposite (no network), so namespace isolation for mount/pid/mem/fs needs
  **zero forwarding**. Verified on [[cmm-validation-box]] (Alma 10, 2026-07-16):
  `ip_forward` stayed 0 through an `unshare --user --map-root-user --mount --pid
  --net` run; unprivileged userns is open (`user.max_user_namespaces=30553`, no
  disable knob). So this class of sandbox needs **no waiver**.
- **Launcher options** (cheapest fit last): `unshare` (util-linux, present);
  `bubblewrap` (clean rootless sandbox, extra dep); or a **small Go launcher**
  (`clone` + `CLONE_NEWNS/NEWPID/NEWNET/NEWUSER` + `pivot_root` + cgroups). The Go
  option is the best fit — CMM is already Go, so it's one auditable binary, no daemon,
  no bridge, no runtime for security to object to (≈ Liz Rice's "build almost-docker
  in Go" talk).
- **Engine = chef-zero local-mode COMPILE.** Chef does the attribute assembly
  (defaults + roles + environments + suite attributes + data bags) for us — we do NOT
  re-implement precedence. Scope is compile-phase; **converge stays Test Kitchen.**
- **Fixtures = the kitchen suite.** `kitchen.yml` suites carry `run_list` +
  `attributes`, and the kitchen dir carries `data_bags/`/`roles/`/`environments/`.
  Author-built to make the cookbook converge, and CMM already ingests it for the TK
  signal — so one fixture set feeds both the compile pre-filter and the TK converge.
- **Converge-in-sandbox (only if ever needed) = systemd-nspawn.** Namespace-based,
  boots an init, `--private-network` = isolated netns (no bridge/forwarding). Heavier
  (needs a rootfs); out of scope for the compile pre-filter.

## Limits / residual risk

- **Coverage:** cookbooks without kitchen fall back to node-seeded fixtures (from the
  Chef server: the node's ohai + normal attrs, its roles/environments, stubbed
  search/data bags — CMM has all of it) or cookstyle-only.
- **Fidelity:** kitchen attributes are a *test scenario*, not a specific production
  node. Correct for compile-readiness; production-faithful is the node-seeded path /
  TK.
- **Setup scripts:** the customer's pre-converge Win/Linux setup hooks
  ([[kitchen-setup-hooks-requirement]]) don't run at compile — converge may need them.
- **No network in sandbox** → dependencies must be pre-vendored (Policyfile/Berkshelf
  lock + local cache, bind-mounted read-only).
- **userns may be clamped** (`user.max_user_namespaces=0`) on a hardened customer
  host → run the launcher in CMM's privileged service context instead of rootless.
- **Secrets:** chef-vault/data-bag secrets can't be pulled for node-seeded runs →
  placeholder-stub; vault-gated code paths then compile differently.

## Validation

- **Done** (cmm box, Alma 10, 2026-07-16):
  - Namespaces + unprivileged userns available (`user.max_user_namespaces=30553`).
  - **Rootless podman proves the delivery claim:** an outbound-networked rootless
    container left host `ip_forward=0` throughout (userspace net via slirp4netns/
    pasta) — no bridge, no daemon, no waiver. `netbackend` rootless, subuid/subgid
    auto-set.
  - **Dokken image solves fidelity:** `docker.io/dokken/almalinux-9` pulled fine and
    bakes in systemd + `/etc/passwd` + rpmdb (AlmaLinux 9.8). Registry reachable.
  - Cop-blocker harness (separate deliverable, `scripts/cop-validation/`).
- **Next:** (1) drive a trivial cookbook's kitchen suite through a real converge on
  the rootless-podman/dokken driver (kitchen-dokken or kitchen-podman) end-to-end;
  (2) confirm userns on a customer-representative host; (3) offline/internal-mirror
  dep flow; (4) Windows converge target (separate — Linux can't host Windows
  containers); (5) node-seeded fixtures for no-kitchen cookbooks.

## Related

- `scripts/cop-validation/` — the cop-blocker harness (validates the cop set).
- `plans/todo-tech-debt.md` — over-claim demotions + harness productionise items.
- `specifications/dual-compatibility-signals.md` — CS vs TK separation (this adds a
  CS-side dynamic pre-filter; TK stays the converge signal).
- [[cmm-validation-box]], [[kitchen-setup-hooks-requirement]].
