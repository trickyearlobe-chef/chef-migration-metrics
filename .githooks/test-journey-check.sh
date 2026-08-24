#!/usr/bin/env bash
# =============================================================================
# Tests for the "specs are journeys" pre-commit check.
# =============================================================================
# Run: .githooks/test-journey-check.sh
#
# The check is the only thing keeping technical claims out of journeys/,
# and a claim that gets in is one nothing re-validates. So it gets a test.
#
# Each case writes a spec, stages it, runs the hook, and asserts on the exit
# status. The staged fixture is always removed again, including on failure.
# =============================================================================

set -uo pipefail
cd "$(git rev-parse --show-toplevel)"

FIXTURE=journeys/_journey_check_fixture.md
# The fixture journey needs a suite like any other, or every case below blocks
# for that reason rather than the one it is testing. It lives under .githooks/
# and starts with an underscore, both of which the Go tool ignores, so it is
# never compiled — the hook only greps for the journey's name inside it.
SUITE_FIXTURE=.githooks/_journey_check_fixture_journey_test.go
PLAN_FIXTURE=plans/_plan_check_fixture.md
PASSED=0
FAILED=0

cleanup() {
  git rm -q --cached --ignore-unmatch "$FIXTURE" >/dev/null 2>&1 || true
  git rm -q --cached --ignore-unmatch "$PLAN_FIXTURE" >/dev/null 2>&1 || true
  rm -f "$FIXTURE" "$SUITE_FIXTURE" "$PLAN_FIXTURE" "$SUITE_FIXTURE.parked"
}
trap cleanup EXIT

printf '// Suite for %s — see test-journey-check.sh\n' "$FIXTURE" > "$SUITE_FIXTURE"

# assert_plan <"block"|"pass"> <description> <<< plan body on stdin
# Plans are checked by a different rule from journeys, so they need their own
# fixture: a plan written into the journey fixture would be checked as a journey.
assert_plan() {
  local want="$1" desc="$2"
  cat > "$PLAN_FIXTURE"
  git add -f "$PLAN_FIXTURE" >/dev/null 2>&1
  local out status
  out=$(.githooks/pre-commit 2>&1); status=$?
  git rm -q --cached --ignore-unmatch "$PLAN_FIXTURE" >/dev/null 2>&1
  rm -f "$PLAN_FIXTURE"

  local got="pass"; [[ $status -ne 0 ]] && got="block"
  if [[ "$got" != "$want" ]]; then
    printf '  FAIL  %s (wanted %s, got %s)\n' "$desc" "$want" "$got"; FAILED=$((FAILED + 1))
    printf '%s\n' "$out" | sed 's/^/        /'
    return
  fi
  printf '  ok    %s\n' "$desc"; PASSED=$((PASSED + 1))
}

# assert <"block"|"pass"> <description> [expected substring] <<< spec body on stdin
# The optional third argument must appear in the hook's output. Use it when the
# exit status alone would pass for the wrong reason — a report pointing at the
# wrong line is still a block.
assert() {
  local want="$1" desc="$2" expect="${3:-}"
  cat > "$FIXTURE"
  git add -f "$FIXTURE" >/dev/null 2>&1
  local out status
  out=$(.githooks/pre-commit 2>&1); status=$?
  git rm -q --cached --ignore-unmatch "$FIXTURE" >/dev/null 2>&1

  local got="pass"; [[ $status -ne 0 ]] && got="block"
  if [[ "$got" != "$want" ]]; then
    printf '  FAIL  %s (wanted %s, got %s)\n' "$desc" "$want" "$got"; FAILED=$((FAILED + 1))
    printf '%s\n' "$out" | sed 's/^/        /'
    return
  fi
  if [[ -n "$expect" ]] && ! printf '%s\n' "$out" | grep -qF -- "$expect"; then
    printf '  FAIL  %s (blocked as expected, but output lacks "%s")\n' "$desc" "$expect"
    FAILED=$((FAILED + 1))
    printf '%s\n' "$out" | sed 's/^/        /'
    return
  fi
  printf '  ok    %s\n' "$desc"; PASSED=$((PASSED + 1))
}

echo "spec journey check:"

assert pass "a journey in the person's own words" <<'EOF'
# Knowing which cookbooks block the upgrade

An engineer wants one answer: will this break when we move to the new Chef
version, and if so what do I fix first. They see a verdict — safe, needs
review, or blocked — with the reasons underneath, worst first.

They know it worked when the list is short enough to work through and nothing
on it turns out to be a lab failure rather than a real one.

Proven by [the verdict contract](internal/analysis/semantic_contracts_test.go).
Nothing can prove the list is short enough to work through.
EOF

assert pass "a reference that resolves" <<'EOF'
# A journey
Pinned by [the contract](internal/analysis/semantic_contracts_test.go#TestContract_CookstyleStatus_StaleIsUnknown).
EOF

assert pass "a link to a sibling journey, which is relative to this file" <<'EOF'
# A journey
See also [another journey](overview.md).
Proven by [the contract](internal/analysis/semantic_contracts_test.go).
EOF

# ---- a journey must name a test ------------------------------------------
# Status is the thing that rots, so it is not written down: a journey names the
# test that proves it, and a red test says "not proven" without anybody
# maintaining a sentence that says so. One resolving link satisfies the rule —
# the parts nothing can prove are stated in prose, because no check can tell an
# honest admission from a missing one.

assert block "a journey that names no test" <<'EOF'
# A journey

An engineer wants to know which servers are ready to move, and what is stopping
the ones that are not, without opening each machine.
EOF

# The link here resolves. The commit must still be blocked, and blocked for
# naming no test rather than for a broken reference — so this file must exist.
assert block "a journey whose only reference is to code, not to a test" <<'EOF'
# A journey
The verdict is decided in [the analyser](internal/analysis/cookstyle_recompute.go).
EOF

# A journey names tests that prove particular things; a suite enumerates
# everything the journey says must be in place, so what is OUTSTANDING is a list
# you can run rather than a paragraph somebody has to keep true.
# Checked by taking the suite away rather than by writing a journey without one,
# so the case cannot pass for the wrong reason once every other rule is satisfied.
mv "$SUITE_FIXTURE" "$SUITE_FIXTURE.parked"
assert block "a journey with no suite" <<'EOF'
# A journey

An engineer needs to know what is stopping the upgrade.

Pinned by [the derivation](internal/analysis/cookstyle_status_test.go#TestDeriveStatus_EmptyOffensesIsReady).
EOF
mv "$SUITE_FIXTURE.parked" "$SUITE_FIXTURE"

assert pass "a journey naming a frontend test" <<'EOF'
# A journey
The version ordering is proven by [the comparison test](frontend/src/semver.test.ts).
EOF

assert pass "a journey naming a test with a fragment" <<'EOF'
# A journey
Proven by [the stale contract](internal/analysis/semantic_contracts_test.go#TestContract_CookstyleStatus_StaleIsUnknown).
EOF

assert block "a journey naming a test that does not resolve" <<'EOF'
# A journey
Proven by [the contract](internal/analysis/no_such_thing_test.go).
EOF

# Prose is wrapped at around 100 columns, so a link with a long target routinely
# breaks between its text and its target. The link is still a link, and the
# target must not then be read as a bare path in prose.
assert pass "a link wrapped across two lines" <<'EOF'
# A journey
An untested cookbook is not a passing cookbook, pinned by [the verdict
contract](internal/analysis/semantic_contracts_test.go#TestContract_CookstyleStatus_NoVerdictsIsUnknown).
EOF

assert block "a wrapped link whose target does not resolve" <<'EOF'
# A journey
Pinned by [the verdict
contract](internal/analysis/no_such_thing_test.go).
EOF

# Line numbers in the report must survive link removal, or the first wrapped
# link in a journey makes every later complaint point at the wrong line.
assert block "a fault reported on the right line after a wrapped link" "line 4" <<'EOF'
# A journey
Pinned by [the verdict
contract](internal/analysis/semantic_contracts_test.go).
The runs are kept in `converge_runs` for ninety days.
EOF

assert block "a reference to a symbol that no longer exists" <<'EOF'
# A journey
Pinned by [the contract](internal/analysis/semantic_contracts_test.go#TestContract_LongSinceRenamed).
EOF

assert block "a reference to a file that no longer exists" <<'EOF'
# A journey
See [the handler](internal/webapi/handle_no_such_thing.go).
EOF

assert block "a link into the archive" <<'EOF'
# A journey
As designed in [the old spec](archive/specifications/ownership.md).
EOF

assert block "a code fence" <<'EOF'
# A journey
```go
x := 1
```
EOF

assert block "an unchecked source path" <<'EOF'
# A journey
The tier is worked out in internal/staleness/tier.go before the list renders.
EOF

assert block "SQL" <<'EOF'
# A journey
SELECT name FROM node_snapshots WHERE organisation_name = 'org-a'
EOF

assert block "an HTTP endpoint" <<'EOF'
# A journey
The list is served by GET /api/v1/nodes and paged.
EOF

assert block "an unchecked function reference" <<'EOF'
# A journey
The tier comes from staleness.ComputeTier() at read time.
EOF

assert block "a table or column name" <<'EOF'
# A journey
The runs are kept in `converge_runs` for ninety days.
EOF

assert block "a config key" <<'EOF'
# A journey
Turned on with `ingest.show_run_events`.
EOF

# ---------------------------------------------------------------------------
# Plans do not tick things off
# ---------------------------------------------------------------------------
# Only the ticked box is a claim. An empty one is a backlog item, which is the
# whole purpose of a todo file — blocking those would push a legitimate list into
# prose for no gain.

assert_plan block "a plan that ticks an item off" <<'EOF'
# A plan

- [x] Wire the collector to the new endpoint
- [ ] Backfill the old rows
EOF

assert_plan pass "a plan listing work still to do" <<'EOF'
# A plan

- [ ] Wire the collector to the new endpoint
- [ ] Backfill the old rows
EOF

assert_plan pass "a plan with no boxes at all" <<'EOF'
# A plan

Wire the collector to the new endpoint, then backfill the old rows. The order
matters: the backfill reads the column the first step adds.
EOF

echo
if [[ $FAILED -gt 0 ]]; then
  printf '%d passed, %d FAILED\n' "$PASSED" "$FAILED"
  exit 1
fi
printf '%d passed\n' "$PASSED"
