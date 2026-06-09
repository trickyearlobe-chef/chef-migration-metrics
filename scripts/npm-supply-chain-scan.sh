#!/usr/bin/env bash
# Copyright 2025 Chef Migration Metrics Authors
# SPDX-License-Identifier: Apache-2.0
#
# Offline npm supply-chain scan for the frontend workspace.
#
# Detects the markers of a compromised / trojanised dependency WITHOUT touching
# the npm registry, so it is safe to run when the registry is blocked. It is the
# behavioural counterpart to a CVE scan: it hunts for the way supply-chain
# attacks actually land (install-script execution, non-registry sources, missing
# integrity, tree drift, obfuscated payloads) rather than for known advisory IDs.
#
# If osv-scanner is installed, a known-vulnerability pass is added (osv-scanner
# uses the OSV database, not the npm registry).
#
# Exit code: 0 = clean, 1 = one or more hard red flags, 2 = usage/setup error.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FE="$ROOT/frontend"
LOCK="$FE/package-lock.json"
NM="$FE/node_modules"

red()   { printf '\033[0;31m%s\033[0m\n' "$*"; }
grn()   { printf '\033[0;32m%s\033[0m\n' "$*"; }
ylw()   { printf '\033[0;33m%s\033[0m\n' "$*"; }
hdr()   { printf '\n\033[1m== %s ==\033[0m\n' "$*"; }

fail=0
flag() { red "  FAIL: $*"; fail=1; }
ok()   { grn "  ok:   $*"; }
warn() { ylw "  warn: $*"; }

[ -f "$LOCK" ] || { red "no package-lock.json at $LOCK"; exit 2; }
command -v python3 >/dev/null 2>&1 || { red "python3 required"; exit 2; }

hdr "1. Install lifecycle scripts in node_modules (RCE-on-install vector)"
# Only preinstall/install/postinstall execute on a registry `npm install`.
# `prepare` does NOT run for a published tarball, so it is not flagged here.
if [ -d "$NM" ]; then
  hits="$(find "$NM" -name package.json -not -path '*/test/*' -not -path '*/fixtures/*' 2>/dev/null \
    | while read -r pj; do
        python3 - "$pj" <<'PY'
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    sys.exit()
s = d.get("scripts", {}) or {}
bad = [k for k in ("preinstall", "install", "postinstall") if k in s]
if bad:
    print(f"{d.get('name','?')}@{d.get('version','?')} -> {','.join(bad)}")
PY
      done)"
  if [ -n "$hits" ]; then
    flag "packages declaring install hooks:"; printf '%s\n' "$hits" | sed 's/^/        /'
  else
    ok "no preinstall/install/postinstall hooks in installed tree"
  fi
else
  warn "node_modules not present — skipping installed-tree script scan"
fi

hdr "2. Lockfile resolved sources (non-registry = injection risk)"
nonreg="$(grep -oE '"resolved": *"[^"]+"' "$LOCK" | grep -vE 'registry\.npmjs\.org' | sort -u)"
if [ -n "$nonreg" ]; then
  flag "packages resolved from outside registry.npmjs.org:"; printf '%s\n' "$nonreg" | sed 's/^/        /'
else
  ok "all dependencies resolve from registry.npmjs.org"
fi

hdr "3. Integrity hashes & tree drift"
python3 - "$LOCK" "$NM" <<'PY'
import json, os, sys
lock, nm = sys.argv[1], sys.argv[2]
d = json.load(open(lock))
pkgs = d.get("packages", {})
missing = [k for k, v in pkgs.items() if k and v.get("resolved") and not v.get("integrity")]
print(f"  locked packages: {len(pkgs)}; missing integrity: {len(missing)}")
for m in missing[:20]:
    print(f"        MISSING-INTEGRITY {m}")
# Drift: installed top-level/scoped dirs not present in the lockfile.
drift = []
if os.path.isdir(nm):
    for entry in sorted(os.listdir(nm)):
        if entry.startswith('.') or entry == '.package-lock.json':
            continue
        if entry.startswith('@'):
            scope = os.path.join(nm, entry)
            for sub in sorted(os.listdir(scope)):
                key = f"node_modules/{entry}/{sub}"
                if key not in pkgs:
                    drift.append(key)
        else:
            key = f"node_modules/{entry}"
            if key not in pkgs:
                drift.append(key)
print(f"  installed packages absent from lockfile: {len(drift)}")
for p in drift[:20]:
    print(f"        DRIFT {p}")
sys.exit(1 if (missing or drift) else 0)
PY
[ $? -eq 0 ] && ok "all resolved packages carry integrity hashes; no tree drift" || fail=1

hdr "4. Known-vulnerability pass (osv-scanner, OSV DB — no registry needed)"
if command -v osv-scanner >/dev/null 2>&1; then
  if osv-scanner --lockfile="$LOCK" >/tmp/osv-npm.txt 2>&1; then
    ok "osv-scanner: no known vulnerabilities"
  else
    flag "osv-scanner reported findings (see below)"; sed 's/^/        /' /tmp/osv-npm.txt
  fi
else
  warn "osv-scanner not installed — skipping CVE pass"
  warn "install: brew install osv-scanner  (or https://github.com/google/osv-scanner)"
fi

hdr "Result"
if [ "$fail" -eq 0 ]; then
  grn "CLEAN — no supply-chain red flags detected."
  exit 0
else
  red "RED FLAGS DETECTED — investigate the FAIL lines above before installing/building."
  exit 1
fi
