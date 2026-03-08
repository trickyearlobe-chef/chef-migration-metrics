# Chef Migration Metrics — ToDo Summary

> **This file is a summary index only.** The authoritative task lists live in the per-component files under `todo/`. When completing tasks, update the relevant component file — do **not** duplicate tasks here.
>
> For recommended next steps, read the most recent summary in `.claude/summaries/` (sorted by filename date prefix). That is the single source of truth for what to work on next.

## Progress Overview

| File | Area | Done | Total | % |
|------|------|-----:|------:|--:|
| [`todo/specification.md`](todo/specification.md) | Specification writing | 32 | 32 | 100% |
| [`todo/project-setup.md`](todo/project-setup.md) | Project setup and tooling | 16 | 16 | 100% |
| [`todo/data-collection.md`](todo/data-collection.md) | Node collection, cookbook fetching, role graph | 68 | 68 | 100% |
| [`todo/analysis.md`](todo/analysis.md) | Usage analysis, compatibility testing, remediation, readiness | 61 | 61 | 100% |
| [`todo/visualisation.md`](todo/visualisation.md) | Dashboard, dependency graph, exports, notifications, log viewer | 67 | 86 | 77% |
| [`todo/logging.md`](todo/logging.md) | Logging infrastructure | 12 | 12 | 100% |
| [`todo/auth.md`](todo/auth.md) | Authentication and authorisation | 0 | 5 | 0% |
| [`todo/configuration.md`](todo/configuration.md) | Configuration and TLS | 58 | 83 | 69% |
| [`todo/secrets-storage.md`](todo/secrets-storage.md) | Secrets and credential management | 85 | 150 | 56% |
| [`todo/packaging.md`](todo/packaging.md) | Build tooling, embedded Ruby, RPM/DEB, container, Compose, ELK, Helm, CI/CD | 37 | 101 | 36% |
| [`todo/testing.md`](todo/testing.md) | Unit, integration, and end-to-end tests | 11 | 40 | 27% |
| [`todo/documentation.md`](todo/documentation.md) | User and developer documentation | 0 | 25 | 0% |
| **Total** | | **447** | **679** | **65%** |

## Regenerating Counts

Run from the project root to refresh the progress numbers above:

```sh
for f in .claude/specifications/todo/*.md; do
  name=$(basename "$f" .md)
  done=$(grep -cE '^\s*-\s+\[x\]' "$f")
  total=$(grep -cE '^\s*-\s+\[[ x~]\]' "$f")
  pct=$( [ "$total" -gt 0 ] && echo "$((done * 100 / total))%" || echo "—" )
  printf "%-20s %3d / %3d  %s\n" "$name" "$done" "$total" "$pct"
done
```
