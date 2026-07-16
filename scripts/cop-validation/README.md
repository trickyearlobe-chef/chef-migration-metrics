# Cop validation harness (spike)

Empirically validates the curated **Blocker** cops (the `RemovedIn` entries in
`internal/remediation/copmapping.go`) against a **real** Chef Infra Client, instead
of trusting hand-curated removal versions. Grew out of the 2026-07-16 lab session
that found ~23% of the curated Blocker set was over-claiming (constructs that still
work on CC19).

Status: **spike / proof-of-concept.** It reproduced the manual 26-cop validation
automatically (15 confirmed blockers, 6 over-claims) and auto-discovered targets the
hand-curation missed. Not wired into the app.

## What it does

One `chef exec ruby` process:

1. **Introspects** each curated Blocker cop from cookstyle — `RESTRICT_ON_SEND`
   gives the method names the cop matches (e.g. `DeprecatedClassMethods` → 8 methods,
   `DeprecatedShelloutMethods` → the `shell_out_compact*` list).
2. **Probes** those targets against the live Chef Infra Client (respond_to /
   `allowed_actions` / `Chef::DSL::Resources` / property validation / `const_defined?`
   / behavioural call-and-catch for node attr methods).
3. **Reconciles**: a curated Blocker whose target is still present ⇒ `OVER-CLAIM`
   (should be Review); target removed ⇒ `CONFIRMED blocker`.
4. **Auto-derives** the `Lint/DeprecatedClassMethods` poly variants from the cop's own
   method list and probes each against the bundled Ruby.

## Run it

On a host with the target's Chef Workstation (see `[[cmm-validation-box]]` —
`cmm.trickyearlobe.com`, Workstation 26 = Chef 19.3.15 + cookstyle):

```
scp cop_validator.rb root@<host>:/tmp/
ssh root@<host> 'cd /tmp && LANG=en_US.UTF-8 chef exec ruby cop_validator.rb'
```

Output is `COP|CURATED_REMOVEDIN|RESTRICT_ON_SEND|PRESENT|VERDICT` lines plus a poly
breakdown.

## Known limitations (spike lessons — see plans/todo-tech-debt.md)

- **Presence ≠ behaviour.** `respond_to?` false-positives on Chef::Node attr methods
  (the `NodeSetUnless` "ghost respond_to"); probe behaviourally (call + catch).
- **Arg-form deprecations** (`attr :x, true`, `depends 'compat_resource'`) are not
  method removals — the method exists, only a call form is dead. Presence probes are
  the wrong question; these need arg-aware or **ChefSpec** behavioural checks. The
  Kernel/Module poly probes (`attr`, `iterator?`) are unreliable for this reason.
- **Windows / platform-gated cops** need Fauxhai/ChefSpec platform faking to validate
  behaviour cross-platform from one Linux host.
- `chef exec` loads the client's vendored cookstyle (8.6.10), not the workstation
  binary's newer copy — fine for probing, note for introspection fidelity.

The production harness adds a ChefSpec behavioural layer and wires
reconcile → auto-demote + a curation-drift warning.
