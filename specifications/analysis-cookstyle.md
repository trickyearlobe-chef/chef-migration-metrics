# Analysis — CookStyle Invocation

#### Design: CookStyle Invocation

CookStyle is invoked as an external process against cookbooks downloaded from the Chef server.

> **JSON output policy:** CookStyle supports `--format json` which produces machine-parseable RuboCop JSON output. All CookStyle invocations (initial scan and auto-correct preview) **must** use `--format json`. Never parse CookStyle's human-readable text output.

**External Tools**

CookStyle is **provided by Chef Workstation** installed on the host. It is **not** bundled with the application — there is no embedded Ruby runtime. The application resolves the `cookstyle` binary from `PATH`.

Chef Workstation must be installed on the host for CookStyle scanning to be available.

**Invocation sequence per organisation + cookbook name + version**

1. **Check skip condition** — Query the datastore for an existing CookStyle result for this organisation + cookbook name + version. If one exists and no manual rescan has been requested, skip the scan and log at `DEBUG` severity.

2. **Prepare workspace** — The cookbook files have been downloaded by the data collection component into a directory keyed by organisation + cookbook name + version. The analysis component operates directly on this directory.

3. **Run CookStyle scan** — Execute:

   ```
   cookstyle --format json <COOKBOOK_DIRECTORY>
   ```

   The `--format json` flag produces machine-parseable output. Capture combined stdout/stderr. Set a configurable timeout (default: 10 minutes).

   **Config isolation.** The scan is driven by a **self-contained sidecar** config (`.rubocop_cmm.yml`, written into the cookbook directory and pointed at via `--config`) that sets `AllCops.TargetChefVersion`, requires `cookstyle`, and enables any operator addon cops. It **must not** inherit the cookbook's own `.rubocop.yml` / `.rubocop_todo.yml`. Those files are the team's style configuration and a deferred-violations TODO — irrelevant to a migration-readiness verdict (honouring the TODO would *hide* the offences we assess), and git-sourced cookbooks routinely carry a `.rubocop_todo.yml` referencing renamed/obsolete cops (e.g. `Metrics/LineLength` → `Layout/LineLength`) that make CookStyle abort with **exit 2** ("obsolete configuration found"). Every cookbook is therefore assessed against one consistent, tool-controlled ruleset for the target version, immune to stale in-repo configs. The cookbook's own files are left untouched — only the sidecar is added.

4. **Parse JSON output** — The CookStyle JSON output follows the RuboCop JSON formatter structure:

   ```json
   {
     "metadata": {
       "rubocop_version": "...",
       "ruby_engine": "...",
       "ruby_version": "..."
     },
     "files": [
       {
         "path": "recipes/default.rb",
         "offenses": [
           {
             "severity": "convention|warning|error|fatal",
             "message": "...",
             "cop_name": "ChefDeprecations/ResourceWithoutUnifiedTrue",
             "corrected": false,
             "location": {
               "start_line": 10,
               "start_column": 1,
               "last_line": 10,
               "last_column": 30
             }
           }
         ]
       }
     ],
     "summary": {
       "offense_count": 3,
       "target_file_count": 5,
       "inspected_file_count": 5
     }
   }
   ```

   Extract the following from the parsed output:

   | Field | Source |
   |-------|--------|
   | Total offense count | `summary.offense_count` |
   | Deprecation warnings | Offenses where `cop_name` starts with `ChefDeprecations/` |
   | Correctness errors | Offenses where `cop_name` starts with `ChefCorrectness/` |
   | All offenses (full list) | `files[*].offenses[*]` |
   | Rollup status + complexity | Derived from `(offenses + resolved classification)` — see below |

   **Status & complexity are classification-derived (single source of truth).**
   The persisted **CookStyle rollup status** (Ready / Needs review / Blocked) and
   the classification-weighted complexity score are derived by one function over
   the offenses and the resolved cop classification for the target version, **not**
   by severity, which is never a source of a verdict. The same derivation runs on
   initial scan and on every criteria-change
   re-evaluation. See [cop-classification.md](cop-classification.md) (CookStyle
   Rollup Status, Re-evaluation & Propagation) for the rules.

5. **Persist result** — Write the CookStyle result to the datastore:

   | Field | Value |
   |-------|-------|
   | `organisation` | Chef server organisation name |
   | `cookbook_name` | Cookbook name |
   | `cookbook_version` | Cookbook version |
   | `status` | CookStyle rollup status (Ready / Needs review / Blocked) — classification-derived |
   | `passed` | Boolean — derived convenience = `status != Blocked` (back-compat) |
   | `offense_count` | Total number of offenses |
   | `deprecation_count` | Number of `ChefDeprecations/*` offenses |
   | `correctness_count` | Number of `ChefCorrectness/*` offenses |
   | `offenses_json` | Full offense list as JSON (for detail display in dashboard) |
   | `raw_output` | Raw stdout/stderr from the CookStyle process |
   | `scanned_at` | UTC timestamp |

6. **Log result** — Log the outcome at `INFO` (pass) or `WARN` (fail with warnings only) or `ERROR` (fail with errors) severity. Include organisation, cookbook name, version, offense count, and deprecation count.

**Deprecation detection**

CookStyle cops in the `ChefDeprecations` namespace directly correspond to features removed or changed in newer Chef Client versions. The dashboard must display these prominently as they are the primary signal that a cookbook will fail against a target Chef Client version.

The following cop namespaces are tracked:

| Namespace | Relevance |
|-----------|-----------|
| `ChefDeprecations/*` | Features removed in newer Chef versions — high relevance to migration |
| `ChefCorrectness/*` | Incorrect usage that may cause runtime failures |
| `ChefStyle/*` | Style issues — low relevance, displayed but do not affect pass/fail |
| `ChefModernize/*` | Modernisation suggestions — informational, do not affect pass/fail |
| Other | Inherited RuboCop cops — informational |

**Error handling**

- If `cookstyle` exits with a non-zero code but produces valid JSON output, the result is still parsed and recorded. CookStyle exits non-zero when offenses are found — this is normal.
- If `cookstyle` exits with a non-zero code and produces no valid JSON output (e.g. crash), log at `ERROR` severity with the raw output and record the scan as failed.
- If the cookbook directory is empty or missing, log at `ERROR` and skip.
