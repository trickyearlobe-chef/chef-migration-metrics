# NPM Security Vulnerability Fix

**Date:** 2026-03-08  
**Component:** Frontend (npm dependencies)  
**Branch:** `fix/npm-security-vulnerabilities`

---

## Context

Running `make run` reported **2 moderate severity vulnerabilities** during the `npm install` step of the frontend build. Both traced back to the same root cause: **esbuild ≤ 0.24.2** ([GHSA-67mh-4wv8-2f99](https://github.com/advisories/GHSA-67mh-4wv8-2f99)), which allows any website to send requests to the development server and read responses. Vite 5.x depended on this vulnerable esbuild version.

## What Was Done

Upgraded two frontend npm packages to resolve both vulnerabilities:

| Package                  | Before   | After    |
|--------------------------|----------|----------|
| `vite`                   | `^5.3.1` | `^6.4.1` |
| `@vitejs/plugin-react`   | `^4.3.1` | `^5.1.4` |

The `@vitejs/plugin-react` v5 explicitly supports Vite 4, 5, 6, and 7, so the upgrade is straightforward. No changes were needed to `vite.config.ts` or any application code.

## Final State

- `npm audit` reports **0 vulnerabilities**
- `npm run build` (tsc + vite) succeeds with Vite 6.4.1
- Output bundle sizes are virtually unchanged (~316 KB JS, ~36.5 KB CSS)
- Application starts and runs normally via `make run`

## Known Gaps

- None — this was a clean dependency upgrade with no breaking changes to the frontend.

## Files Modified

**Production:**
- `frontend/package.json` — bumped `vite` and `@vitejs/plugin-react` versions
- `frontend/package-lock.json` — regenerated lockfile

**Documentation:**
- `.claude/summaries/2026-03-08-22-26-npm-security-vulns.md` — this file
- `.claude/Structure.md` — added summary entry

## Recommended Next Steps

Refer to the previous summary (`2026-03-08-13-25-cookstyle-json-and-build-fixes.md`) for the broader project roadmap. This fix was a small housekeeping task and does not change the overall plan.