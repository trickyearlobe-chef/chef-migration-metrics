# Active Plan — Kitchen Lifecycle Setup Hooks

Goal: make git-kitchen runs succeed for cookbooks that depend on setup steps.
Two parts: (1) preserve repo-provided lifecycle hooks (they're test setup),
(2) let operators run setup scripts — e.g. create users, on Windows and Linux —
before converge so cookbooks that depend on them pass. Both chunks are in scope:
(2) is a general capability for any tenant whose setup lives outside
`.kitchen.yml`, not specific to the current customer (decided 2026-06-07).

Source todo: `plans/todo-bulk-kitchen-scanning.md` § Lifecycle Hooks.
Spec: `specifications/test-kitchen-drivers-overlay-generation.md` § Lifecycle
Hooks — currently a STUB, finalise first.
Branch: fresh `feature/kitchen-setup-hooks`.

Start each chunk in a fresh thread; read only this plan + the spec section.
TDD: write/extend tests before code.

## Context

The overlay (`internal/gitkitchen/overlay.go`) writes `.kitchen.local.yml`;
the cookbook's `.kitchen.yml` is untouched. Today the overlay injects only the
opt-in `pre_destroy` IP-release hook, composing with any repo `pre_destroy` via
`writeLifecycleHook` + `readExistingPreDestroy`. Test Kitchen hash-merges
`lifecycle:` (per-phase arrays replace), so phases the overlay never writes are
preserved — but this is unverified for non-`pre_destroy` phases and the
repo-setup-hook contract is an admitted spec stub. There is no mechanism to run
a customer setup script that isn't already wired into `.kitchen.yml`.

Customer constraint: repos carry Windows + Linux shell scripts (e.g. user
creation) that cookbooks need before converge; without running them, TK fails.

## Chunk 1 — Finalise the lifecycle-hooks contract (spec)  [DONE 2026-06-07]

TK lifecycle-hook facts (from kitchen.ci, 2026-06-07):
- Phases `pre_/post_/finally_` × `create|converge|verify|destroy`. `local:` runs
  on the workstation (`cwd:`, `environment:`, `KITCHEN_INSTANCE_HOSTNAME` set);
  `remote:` runs on the guest via SSH/WinRM. `remote:` is unavailable at
  `pre_create`/`post_destroy`. Platform targeting via `includes:`/`excludes:`.
- Hooks take COMMAND STRINGS — there is no `path:`-to-script option. Confirmed
  against test-kitchen 4.0.0 source (`lifecycle_hook/remote.rb`,`local.rb`):
  `remote:` does `transport.connection.execute(command)` (inline string on the
  guest, no upload); `local:` runs the string on the workstation (cwd=kitchen
  root, `KITCHEN_INSTANCE_HOSTNAME` in env). There is NO file-transfer in a
  hook. Therefore to run a repo script ON THE GUEST the body must be **inlined**
  into a `remote:` hook — "upload" (B) would mean hand-rolling scp/winrm in a
  `local:` hook, re-implementing transport auth TK already owns. Decision:
  inline (A) is the mechanism, not a preference.

Resolve and write into `test-kitchen-drivers-overlay-generation.md` § Lifecycle
Hooks:
- Reserved vs repo-owned phases (CMM owns `pre_destroy` only?).
- Preservation rule for every repo-defined phase (TK hash-merges `lifecycle:`,
  replaces per-phase arrays; overlay must not clobber phases it doesn't own).
- Setup-script mechanism — the decisions:
  - Mechanism: **inline** the script body into a `remote:` hook (resolved above;
    TK hooks have no upload).
  - Phase: `pre_converge` (guest exists, before the cookbook converge).
  - Discovery: operator-configurable **patterns** (one or more) matched against
    repo file paths — customers' conventions vary. Default glob (path-friendly);
    regex a possible later extension. Scoped **per OS family** (linux/windows):
    scripts are inherently OS-keyed (sh/SSH vs ps1/WinRM) and CMM already derives
    `osFamily` from the platform → maps to the hook `includes:`. Per-image
    override deferred (not needed initially). Multiple matches → separate
    `remote:` hooks in deterministic (sorted-by-path) order.
  - Windows vs Linux: emit `includes:` per the matched OS family.
  - Failure semantics: setup hooks MUST fail the run when they fail (opposite of
    the failure-isolated IP-release hook — the cookbook depends on them).
Acceptance: section is no longer a stub; decisions above recorded.

## Chunk 2 — Preserve repo lifecycle hooks (correctness)  [DONE 2026-06-07]

Regression tests lock the preservation rule: the overlay names `pre_destroy`
only (CMM's single reserved phase), so every other repo-defined lifecycle phase
survives TK's array-replace merge untouched. No read-back beyond `pre_destroy`
is needed. `overlay_test.go`: `WritesOnlyPreDestroyPhase_IPReleaseOn`,
`NoLifecycleBlock_IPReleaseOff`. `executor_test.go`:
`PreservesRepoLifecyclePhases` (repo `pre_create`/`pre_converge`/`post_converge`
never leak into the overlay; `pre_destroy` still composes). Todo items ticked.

## Chunk 3 — Customer setup scripts (opt-in)  [DONE 2026-06-07]

- Config: `TestKitchenConfig.SetupScripts` (`SetupScriptsConfig{Linux,Windows}`)
  — per-OS-family glob pattern lists; `config.go` + admin validators reject
  empty/malformed globs.
- Overlay: `writeLifecycle` emits one `lifecycle:` block; setup bodies are
  inlined into `remote: pre_converge` hooks (no `skippable:` → must fail the
  run), composing with the `pre_destroy` IP-release hook. yaml.Marshal handles
  block-scalar quoting (round-trip verified). `includes:` omitted — overlay is
  per-platform and TK invokes `kitchen test <instance>` (spec updated, decided
  2026-06-07).
- Executor: `discoverSetupScripts` globs the OS-family patterns against the
  workspace, reads bodies, dedupes, sorts by path.
- UI: AdminTestKitchenPage "Setup Scripts (pre-converge)" section, per-family
  textareas; sanitised at save. Types + tests added.
Acceptance met: linux + windows emission, multi-line escaping, config
validation, executor discovery, UI save/load all tested and green.

All chunks (1–3) complete — git-kitchen runs can now preserve repo lifecycle
hooks and run opt-in customer setup scripts before converge.

## Notes

- Elasticsearch is NOT in scope. Kept as config scaffolding only (no exporter
  exists); deferred behind the data-export re-spec in `todo-data-layer.md`. Do
  NOT add an ES config UI. Re-point the `todo-configuration.md` ES item at the
  re-spec decision rather than treating it as ready work.
- `roadmap.md` is stale (all 3 phases already built) — prune as a follow-up chore.
