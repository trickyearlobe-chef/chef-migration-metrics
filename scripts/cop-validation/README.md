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

## poly_disambiguate.rb — poly-message cop disambiguation

The viable way to classify a **poly-message** cop (one cop_name, several
deprecations of differing impact — e.g. `Lint/DeprecatedClassMethods`): read the
cop's authoritative table from cookstyle (not `RESTRICT_ON_SEND`, which
over-approximates), then **behaviourally** probe each construct against the live
Ruby/Chef. Used 2026-07-16 to complete the `DeprecatedClassMethods` variant table
in `internal/remediation/copmapping.go`, which discovered `ENV.clone`/`dup`/`freeze`
raise `TypeError` on Ruby 3.4 (Blocker, we were missing them) and `iterator?`/`attr`
are still present (Review — a false-positive Blocker via fallback until added).

## False-negative sweep (2026-07-16) — hidden-blocker discovery

The prior work validated only **false positives** (curated Blockers that over-claim).
This sweep hunts the dangerous direction: cops that flag something genuinely
removed/broken on CC19 but were **not** curated, so they defaulted to Review — a
hidden blocker (see `specifications/cop-classification.md`, asymmetric confidence).

Method (scripts, run in order on `cmm.trickyearlobe.com`):

1. `enumerate_cops.rb` — full inventory of `Chef/Deprecations/*`, `Lint/*`,
   `Chef/Correctness/*` from the RuboCop registry (272 cops: 79 / 152 / 41).
2. `inspect_candidates.rb` — pull each candidate's authoritative table / MSG.
3. `probe_candidates.rb` — **behavioural** probe (call + catch) of each target
   against CC19.3.15 / Ruby 3.4.8; a `NoMethodError`/`NameError` (gone) or
   `TypeError`/`ArgumentError` (breaks) ⇒ Blocker, a clean run ⇒ Review.
4. Cross-check the cop actually **fires** in a real cookbook layout (cookstyle
   respects per-cop `Include` paths — a loose `.rb` misses resource/library cops).
5. `showcops_desc.rb` — confirm each addition is emitted by `--show-cops` (not
   stale) and its RemovedIn agrees with the description's "removed in N" (else the
   curation linter flags it).

Reconciliation — **11 hidden blockers found and added** to `copmapping.go`
(cop → removed-on-CC19? → was-curated?):

| Cop | Probe result on CC19.3.15 | RemovedIn |
|-----|---------------------------|-----------|
| `Chef/Deprecations/UsesChefRESTHelpers` | `Chef::REST` NameError | 13.0 |
| `Chef/Deprecations/ChefShellout` | `Chef::ShellOut` NameError | 13.0 |
| `Chef/Deprecations/UsesDeprecatedMixins` | 4 mixins NameError | 14.0 |
| `Chef/Deprecations/ResourceUsesDslNameMethod` | `dsl_name` absent | 13.0 |
| `Chef/Deprecations/NodeSetWithoutLevel` | `node['x']=y` raises read-only | 11.0 |
| `Chef/Deprecations/PartialSearchClassUsage` | `Chef::PartialSearch` NameError | 19.0 |
| `Chef/Deprecations/PartialSearchHelperUsage` | `partial_search` NoMethodError | 19.0 |
| `Chef/Deprecations/EpicFail` | `epic_fail` NoMethodError | 19.0 |
| `Lint/BigDecimalNew` | `BigDecimal.new` NoMethodError (Ruby 2.7) | 19.0 |
| `Lint/UnifiedInteger` | `Fixnum`/`Bignum` NameError (Ruby 3.2) | 19.0 |
| `Lint/DeprecatedConstants` (poly) | NIL/TRUE/FALSE/Random::DEFAULT/Struct::* NameError | 19.0 |

Poly note: `Lint/DeprecatedConstants` base is Blocker; `Net::HTTPServerException`
(still an alias on Ruby 3.4) is the one Review carve-out variant.

Probed but **left Review** (recorded so they are not re-swept):

- `Lint/UriEscapeUnescape` — URI.escape/unescape are removed on Ruby 3.4, **but the
  cop is `Enabled: false` in cookstyle's default config**, so it never appears in a
  scan → not classifiable. (The strongest-looking candidate; disabled ⇒ moot.)
- `Lint/ErbNewArguments` — `ERB.new(str, safe_level, trim_mode)` still **runs** on
  Ruby 3.4 (both nil and numeric safe_level) → Review, not a removal.
- `Lint/UriRegexp`, `Lint/DeprecatedOpenSSLConstant` — targets present on Ruby 3.4.
- `Chef/Deprecations/ResourceUsesOnlyResourceName` — failure is conditional at
  resource *resolution* (CC16+), not reproducible from a static class-build probe;
  needs a ChefSpec resolution test before promotion.
- Windows/Powershell helper cops (`WindowsVersionHelpers`, `PowershellCookbookHelpers`,
  `DeprecatedWindowsVersionCheck`) — can't validate from a Linux host; need Fauxhai.
- All `Chef/Correctness/*` — advisory correctness/style patterns, no removed API.
- `InSpec/Deprecations/AttributeHelper` + `AttributeDefault` — surfaced in real
  customer data (106 each) so probed on the box (`probe_inspec_attribute.rb`): a real
  `inspec exec` of `attribute('foo', default: 'bar')` on InSpec 7.0.107 **runs and
  passes** (deprecation warning only). The `attribute` alias and the `default:` option
  are deprecated, not removed → Review. (The InSpec/Deprecations department is outside
  the Chef/Deprecations + Lint + Chef/Correctness sweep scope but was checked because it
  appeared in real scans.)

Scripts added this sweep: `enumerate_cops.rb`, `inspect_candidates.rb`,
`probe_candidates.rb`, `probe_loose_ends.rb`, `showcops_desc.rb`.

### Coverage limit — cookstyle does NOT catch all Ruby removals

The sweep also established (lab-verified, CC19.3.15 / Ruby 3.4.8) that cookstyle's
default scan misses a whole class of Ruby removals: of `URI.escape` (removed Ruby 3.0),
`String#taint`/`tainted?` (removed Ruby 3.2), and removed default gems
(`net/telnet`/`xmlrpc`/`sdbm` — `require` → LoadError; note `webrick` is vendored by the
omnibus and still loads, so gem-removal breakage is install-dependent), the default
config flagged **none** — the Ruby-3.x ones break at runtime anyway. RuboCop only flags
a removal when an *enabled* cop with an explicit pattern exists; it is not an
authoritative list of what the target Ruby removed. So the CookStyle signal is a
**necessary-but-incomplete** check for Ruby-level compatibility, and the behavioural
converge signal (Test Kitchen / ChefSpec) is the completeness backstop. Full detail,
gap classes, and follow-ups (enable `Lint/UriEscapeUnescape`; custom regex cops) are in
`plans/todo-tech-debt.md` (“CookStyle — static coverage of Ruby removals is
incomplete”).

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
