#!/usr/bin/env bash
# =============================================================================
# Tests for the "specs are journeys" pre-commit check.
# =============================================================================
# Run: .githooks/test-spec-journey-check.sh
#
# The check is the only thing keeping technical claims out of specifications/,
# and a claim that gets in is one nothing re-validates. So it gets a test.
#
# Each case writes a spec, stages it, runs the hook, and asserts on the exit
# status. The staged fixture is always removed again, including on failure.
# =============================================================================

set -uo pipefail
cd "$(git rev-parse --show-toplevel)"

FIXTURE=specifications/_journey_check_fixture.md
PASSED=0
FAILED=0

cleanup() {
  git rm -q --cached --ignore-unmatch "$FIXTURE" >/dev/null 2>&1 || true
  rm -f "$FIXTURE"
}
trap cleanup EXIT

# assert <"block"|"pass"> <description> <<< spec body on stdin
assert() {
  local want="$1" desc="$2"
  cat > "$FIXTURE"
  git add -f "$FIXTURE" >/dev/null 2>&1
  local out status
  out=$(.githooks/pre-commit 2>&1); status=$?
  git rm -q --cached --ignore-unmatch "$FIXTURE" >/dev/null 2>&1

  local got="pass"; [[ $status -ne 0 ]] && got="block"
  if [[ "$got" == "$want" ]]; then
    printf '  ok    %s\n' "$desc"; PASSED=$((PASSED + 1))
  else
    printf '  FAIL  %s (wanted %s, got %s)\n' "$desc" "$want" "$got"; FAILED=$((FAILED + 1))
    printf '%s\n' "$out" | sed 's/^/        /'
  fi
}

echo "spec journey check:"

assert pass "a journey in the person's own words" <<'EOF'
# Knowing which cookbooks block the upgrade

An engineer wants one answer: will this break when we move to the new Chef
version, and if so what do I fix first. They see a verdict — safe, needs
review, or blocked — with the reasons underneath, worst first.

They know it worked when the list is short enough to work through and nothing
on it turns out to be a lab failure rather than a real one.
EOF

assert pass "a reference that resolves" <<'EOF'
# A journey
Pinned by [the contract](internal/analysis/semantic_contracts_test.go#TestContract_CookstyleStatus_StaleIsUnknown).
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

echo
if [[ $FAILED -gt 0 ]]; then
  printf '%d passed, %d FAILED\n' "$PASSED" "$FAILED"
  exit 1
fi
printf '%d passed\n' "$PASSED"
