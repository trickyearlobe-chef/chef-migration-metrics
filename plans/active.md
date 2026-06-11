# Active — Deprecate embedded tooling + dokken (require Chef Workstation)

The Docker-based embedded-Ruby build was abandoned (too unreliable to build).
cookstyle/kitchen now come from **Chef Workstation on PATH**. **dokken** (the
Docker driver) is not selectable and not on the roadmap. The **code already
reflects this** — no dokken built-in profile (built-ins are
vcenter/vra/ec2/vagrant/proxmox), `ValidateAll` no longer checks Docker, tool
resolution falls back to PATH. Stale leftovers: the `embedded_bin_dir`
mechanism, the Makefile embedded build, packaging/config/driver specs, the
README, and the example config.

Verified state (2026-06-11): `builtinProfiles` (driver_profiles.go) has no
dokken; `embedded.Resolver` checks `embedded_bin_dir` then PATH — dir never
exists, so always PATH; `embedded.go` `ValidateDocker` is unused (`ValidateAll`
dropped it); `Makefile:186–243` still builds the embedded Ruby tree incl
kitchen-dokken; README + ~18 specs still present dokken as default/built-in and
tools as embedded.

Each chunk = its own branch, doc-only ones low-risk. Code/build chunks TDD.

## C1 — README (doc-only, do first — explicit ask)
- Drop the Docker + `kitchen-dokken` prerequisite and the "embedded Ruby" build
  prereq; state Chef Workstation is required and tools resolve from PATH.
- Remove `analysis_tools.embedded_bin_dir` mention.
- Test Kitchen Driver Configuration: no default driver — operator chooses
  (vcenter/ec2/vra/vagrant/proxmox); drop the dokken minimal example, keep the
  vCenter example as the primary.

## C2 — Code: remove `embedded_bin_dir` (TDD)
- Remove `EmbeddedBinDir` config field + default + env override + validation
  warning (config.go) and the `diagnostic_config.go` report field.
- Simplify `embedded.Resolver` to PATH-only (drop the dir-first arg/logic);
  update `embedded.NewResolver` call site (main.go:856) and `embedded_test.go`.
- Remove the now-unused `ValidateDocker` (confirm no callers).

## C3 — Build: strip embedded Ruby build
- Remove `build-embedded*` / `_build-embedded` targets + RUBY_BUILD_IMAGE etc.
  from the Makefile. Confirm `release.yml` / package targets don't depend on it.

## C4 — Packaging specs (doc)
- Rewrite `packaging.md`, `packaging-nfpm.md`, and the embedded section of
  `configuration-schema-collection.md` → "Chef Workstation required; tools via
  PATH; no embedded Ruby/tools shipped."

## C5 — Driver/dokken specs (doc, large — ~18 files)
- Remove dokken as default/built-in/selectable; built-in profile list →
  vcenter/vra/ec2/vagrant/proxmox; "no default — operator must choose".
- Keep dokken ONLY where it's about *detecting customer cookbook* driver usage
  (kitchen-analyser `driver_name`), not our own runner.
- Files incl: test-kitchen-config-ui.md, test-kitchen-drivers*.md,
  analysis-compatibility-testing.md, analysis-startup-validation.md,
  configuration-*.md, kitchen-refactor.md, kitchen-run-queue.md,
  test-kitchen-drivers-overlay-generation.md, packaging-nfpm.md, overview.md.
- `deploy/pkg/config.yml.example`: drop dokken examples.

## Dependencies / notes
- C1 independent. C2 before C4/C5 embedded_bin_dir wording.
- **Merge `specification/tk-concurrency-reconcile` (commit 1576043) to main first**
  — it already edits several TK specs; doing so avoids C5 conflicts, then branch
  C5 from updated main.
