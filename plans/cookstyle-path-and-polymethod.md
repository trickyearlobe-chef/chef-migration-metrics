# CookStyle path chop + poly-method cop mismatch

Two independent bugs found via the CookStyle detail view. Both surfaced after
`fbbaa89` dropped `--only` (full-ruleset flood made offences/paths visible), but
neither is caused by it.

## Root cause A — offence file paths lose leading 2 chars (`/var/lib` → `ar/lib`)

Verified against customer data + reproduced with the real cookstyle binary.

- cookstyle runs with the process CWD, which on the packaged systemd install is
  `/` (unit has no `WorkingDirectory=`; app never `os.Chdir`s; exec never sets
  `cmd.Dir`). Confirmed live: `readlink /proc/<pid>/cwd` → `/`.
- RuboCop's JSON formatter reports each path via `smart_path` = path relative to
  CWD when CWD is a string prefix. With CWD `/`, it strips `len("/")+1 = 2` chars
  off every absolute path → `/var/lib/...` becomes `ar/lib/...`.
- `relativeCookstylePath` (`internal/analysis/cookstyle.go`) then can't match its
  `repoDir` prefix against the already-mangled path, so it stores the corrupt path.
- Repro: `cd / && cookstyle --format json /var/lib/.../<cb>` → `"path":"ar/lib/..."`;
  from the cookbook dir → `"path":"Dangerfile"`. The chop is lossy — `/v` is gone.
- Affects git repos AND server cookbooks (same `/var/lib` prefix). Dev/`make run`
  is unaffected (CWD = repo dir, not a prefix, so cookstyle emits full absolute
  paths and `relativeCookstylePath` strips them correctly).

### Fix A (branch `fix/cookstyle-scan-cwd`)

- Thread a working directory into the cookstyle exec and set `cmd.Dir` to the
  cookbook/repo dir being scanned. RuboCop then reports paths relative to the
  cookbook root regardless of process CWD; `relativeCookstylePath` stays as a
  safety net.
- Scope: `CookstyleExecutor.Run` (`internal/analysis/cookstyle.go`) + default
  executor + `makeCommand` (`exec.go`); the two scan call sites in
  `runScanWithAddonIsolation` (pass `cookbookDir`); `--show-cops` in
  `cop_registry.go` passes no dir. Mirror in `AutocorrectExecutor`
  (`internal/remediation/autocorrect.go`) for the preview run.
- Sidecar `--config` path and addon `require:` paths are absolute → unaffected by
  the CWD change.
- Tests: fake executor asserts `cmd.Dir` set to the scanned dir; a test that a
  file-based cookstyle payload with absolute `path` under the scan dir persists a
  cookbook-relative offence file.
- Acceptance: new git/server scans store cookbook-relative paths (`recipes/x.rb`);
  no leading-char loss with CWD `/`.
- Existing rows keep corrupt paths (lossy) → require a re-scan to repair. Note in
  the fix summary; no data migration can recover the lost chars.

## Root cause B — multi-method cop shows wrong remediation + false Blocker

- `Lint/DeprecatedClassMethods` is one cop that flags several unrelated
  deprecations (e.g. `File.exists?` — removed in Ruby 3, a real Blocker; and
  `Socket.gethostbyname` — deprecation only, not removed). Confirmed in customer
  data: one cop group, mixed messages.
- Remediation (`internal/remediation/copmapping.go`) and classification are keyed
  on cop NAME only, so every offence in the group inherits one `File.exists?`
  guidance block AND one "Blocker / removed in 18.0" verdict — wrong for the
  `Socket.*` offences (misleading guidance + false-positive Blocker).

### Fix B (separate branch, fresh context)

- Key remediation/classification on the offence MESSAGE for the small set of
  known poly-method cops; fall back to cop-name keying otherwise.
- Scope: mapping lookup + resolver + the git/server remediation handlers that
  build offence groups. Needs a spec check first (message-level keying is a model
  change, not a data tweak).
- Acceptance: `Socket.gethostbyname` offences show Addrinfo guidance and do NOT
  count as Blockers; `File.exists?` offences unchanged.
