#!/usr/bin/env python3
"""Ask a running instance what it actually does, so the description does not
have to guess.

Reads only. Sends GET and nothing else — never POST/PUT/PATCH/DELETE — so it is
safe to point at anything you can already read.

It exists because three attempts to derive pagination from the code all
over-reported, and the description would have told callers to send parameters
that are ignored. What it settles:

  * which addresses honour per_page, tested behaviourally by asking twice —
    once plainly, once with per_page=1 — rather than by looking for a
    "pagination" key, which misses an address that pages without any metadata;
  * what shape each readable address answers with, which is the recording
    needed before anything can fail when a field changes meaning under a
    caller.

VALUES ARE NEVER RECORDED. Every leaf collapses to its JSON type before it
reaches the output, so what this writes is a shape catalogue with no data in
it — safe to read, keep and share. Do not change that without thinking about
where the output ends up.

Path parameters are filled from the nearest ancestor collection. The identity
field is not named consistently (/api/v1/roles answers with role_name while its
detail address is /{name}), so several candidates are tried before giving up;
anything still unfilled is reported as unprobed rather than guessed at.

Usage:

    CMM_API_TOKEN=<a token from your account page> \\
        python3 tools/api-probe/probe.py https://127.0.0.1 out.json

The token is read from the environment on purpose. Do not pass it on the
command line, where it lands in shell history.
"""

import json
import os
import ssl
import sys
import urllib.error
import urllib.request

ssl_ctx = ssl.create_default_context()
ssl_ctx.check_hostname = False
ssl_ctx.verify_mode = ssl.CERT_NONE

BASE = sys.argv[1] if len(sys.argv) > 1 else "https://127.0.0.1"
OUT = sys.argv[2] if len(sys.argv) > 2 else "probe2-out.json"
TOKEN = os.environ.get("CMM_API_TOKEN", "").strip()
if not TOKEN:
    sys.exit("CMM_API_TOKEN is not set")


def get(path, timeout=25):
    req = urllib.request.Request(BASE + path, method="GET")
    req.add_header("Authorization", "Bearer " + TOKEN)
    req.add_header("Accept", "application/json")
    try:
        with urllib.request.urlopen(req, context=ssl_ctx, timeout=timeout) as r:
            return r.status, r.headers.get("Content-Type", ""), r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.headers.get("Content-Type", ""), e.read()
    except Exception as e:  # noqa: BLE001
        return 0, "", str(e).encode()


def decode(raw):
    try:
        return json.loads(raw)
    except Exception:  # noqa: BLE001
        return None


def shape_of(value, depth=0):
    if depth > 8:
        return "..."
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "boolean"
    if isinstance(value, int):
        return "integer"
    if isinstance(value, float):
        return "number"
    if isinstance(value, str):
        return "string"
    if isinstance(value, list):
        if not value:
            return {"type": "array", "items": "unknown-empty-in-sample"}
        merged, scalar = {}, None
        for item in value[:25]:
            s = shape_of(item, depth + 1)
            if isinstance(s, dict) and s.get("type") == "object":
                for k, v in s["properties"].items():
                    merged.setdefault(k, set()).add(json.dumps(v, sort_keys=True))
            else:
                scalar = s
        if scalar is not None and not merged:
            return {"type": "array", "items": scalar}
        return {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {k: merge(v) for k, v in sorted(merged.items())},
            },
        }
    if isinstance(value, dict):
        return {
            "type": "object",
            "properties": {k: shape_of(v, depth + 1) for k, v in sorted(value.items())},
        }
    return "unknown"


def merge(encoded):
    decoded = [json.loads(e) for e in sorted(encoded)]
    real = [d for d in decoded if d != "null"]
    if not real:
        return "null"
    if len(real) == 1:
        return real[0] if len(decoded) == 1 else {"oneOf": ["null", real[0]]}
    return {"oneOf": decoded}


def rows_of(body):
    """The list an answer carries, if it carries one."""
    if isinstance(body, list):
        return body
    if isinstance(body, dict):
        for key in ("data", "items", "results", "entries", "nodes", "tokens"):
            if isinstance(body.get(key), list):
                return body[key]
    return None


def params_in(path):
    return [s[1:-1] for s in path.split("/") if s.startswith("{") and s.endswith("}")]


def ancestor_collections(path):
    """Collection paths that could hold an id for this path, nearest first."""
    segs = path.split("/")
    out = []
    for i in range(len(segs) - 1, 1, -1):
        if segs[i].startswith("{"):
            candidate = "/".join(segs[:i])
            if candidate and not any(s.startswith("{") for s in candidate.split("/")):
                out.append(candidate)
    return out


CACHE = {}


def sample_row(collection):
    if collection in CACHE:
        return CACHE[collection]
    status, ctype, raw = get(collection)
    row = None
    if status == 200 and "json" in ctype:
        rows = rows_of(decode(raw))
        if rows and isinstance(rows[0], dict):
            row = rows[0]
    CACHE[collection] = row
    return row


def fill(path):
    needed = params_in(path)
    if not needed:
        return path, []
    values, missing = {}, []
    for collection in ancestor_collections(path):
        row = sample_row(collection)
        if not row:
            continue
        # The identity field is not consistently named: /api/v1/roles answers
        # with role_name while its detail address is /{name}, and the same
        # split shows up for cookbooks and cops. So try the obvious names, then
        # anything that looks like an identifier, rather than giving up.
        singular = collection.rstrip("/").split("/")[-1].rstrip("s").replace("-", "_")
        for p in needed:
            if p in values:
                continue
            candidates = [p, f"{singular}_name", f"{singular}_id", "name", "id"]
            candidates += [k for k in sorted(row) if k.endswith(("_name", "_id"))]
            for key in candidates:
                if key in row and isinstance(row[key], (str, int)) and row[key] != "":
                    values[p] = row[key]
                    break
        if len(values) == len(needed):
            break
    out = path
    for p in needed:
        if p in values:
            out = out.replace("{" + p + "}", str(values[p]))
        else:
            missing.append(p)
    return out, missing


def main():
    status, _, raw = get("/api/v1/openapi.json")
    if status != 200:
        sys.exit(f"cannot read the description: HTTP {status}")
    paths = json.loads(raw).get("paths", {})

    results = {}
    for path, item in sorted(paths.items()):
        if "get" not in item:
            continue
        key = "GET " + path
        probe, missing = fill(path)
        if missing:
            results[key] = {"outcome": "unprobed", "missing_params": missing}
            continue

        status, ctype, raw = get(probe)
        entry = {"status": status}
        if status != 200:
            entry["outcome"] = "not-ok"
            body = decode(raw)
            if isinstance(body, dict):
                # The message, not the data — it says what the address wants.
                entry["message"] = str(body.get("message") or body.get("error"))[:160]
            results[key] = entry
            continue
        if "json" not in ctype:
            entry["outcome"] = "not-json"
            entry["content_type"] = ctype.split(";")[0]
            results[key] = entry
            continue
        body = decode(raw)
        if body is None:
            entry["outcome"] = "undecodable"
            results[key] = entry
            continue

        entry["outcome"] = "ok"
        entry["shape"] = shape_of(body)
        entry["echoes_pagination"] = isinstance(body, dict) and "pagination" in body

        # Behavioural: does per_page actually do anything here?
        base_rows = rows_of(body)
        entry["rows"] = len(base_rows) if base_rows is not None else None
        entry["honours_per_page"] = False
        if base_rows is not None and len(base_rows) > 1:
            sep = "&" if "?" in probe else "?"
            s2, c2, r2 = get(probe + sep + "per_page=1")
            if s2 == 200 and "json" in c2:
                limited = rows_of(decode(r2))
                if limited is not None and len(limited) == 1:
                    entry["honours_per_page"] = True
        elif base_rows is not None:
            # Too few rows to tell. Recording that beats recording "no".
            entry["honours_per_page"] = "undetermined-too-few-rows"
        results[key] = entry

    def where(pred):
        return sorted(k for k, r in results.items() if pred(r))

    summary = {
        "probed": len(results),
        "ok": len(where(lambda r: r.get("outcome") == "ok")),
        "echoes_pagination": where(lambda r: r.get("echoes_pagination")),
        "honours_per_page": where(lambda r: r.get("honours_per_page") is True),
        "undetermined": where(
            lambda r: r.get("honours_per_page") == "undetermined-too-few-rows"
        ),
        "disagreement": where(
            lambda r: r.get("outcome") == "ok"
            and r.get("honours_per_page") is True
            and not r.get("echoes_pagination")
        ),
        "unprobed": where(lambda r: r.get("outcome") == "unprobed"),
        "not_ok": sorted(
            f"{k} [{r.get('status')}] {r.get('message','')}"
            for k, r in results.items()
            if r.get("outcome") == "not-ok"
        ),
    }
    with open(OUT, "w") as f:
        json.dump({"summary": summary, "operations": results}, f, indent=2, sort_keys=True)

    print(f"probed={summary['probed']} ok={summary['ok']}")
    print(f"echoes pagination metadata = {len(summary['echoes_pagination'])}")
    print(f"provably honours per_page  = {len(summary['honours_per_page'])}")
    print(f"undetermined (too few rows)= {len(summary['undetermined'])}")
    print(f"honours but does NOT echo  = {len(summary['disagreement'])}")
    print(f"unprobed={len(summary['unprobed'])} not-ok={len(summary['not_ok'])}")
    print(f"written to {OUT}")


main()
