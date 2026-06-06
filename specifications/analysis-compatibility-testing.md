# Analysis — Cookbook Compatibility Testing

### 2. Cookbook Compatibility Testing

Tests cookbooks against each configured target Chef Client version and records results.

#### Git-sourced Cookbooks

- Tested using both **CookStyle** and **Test Kitchen** against multiple configured target Chef Client versions
- Only the HEAD commit of the default branch (`main` or `master`) is tested
- The HEAD commit SHA is recorded with each test result (both CookStyle and Test Kitchen)
- A CookStyle scan is skipped if the HEAD commit SHA is unchanged since the last scan for the same cookbook + target Chef Client version. When the HEAD commit changes, the cookbook is rescanned and the previous result is overwritten.
- A Test Kitchen run is skipped if the HEAD commit SHA is unchanged since the last run for the same cookbook + target Chef Client version
- Both Test Kitchen pass criteria must be met for a cookbook to be considered fully compatible:
  1. The cookbook **converges** successfully
  2. The cookbook's **tests pass**
- CookStyle results provide deprecation detection and remediation guidance (auto-correct previews, migration doc links) independently of Test Kitchen pass/fail

#### Chef Server-sourced Cookbooks

- No test suite is available; tested with **CookStyle** only for linting and deprecation warnings
- Cookbook versions are immutable on the Chef server — CookStyle scanning runs once per organisation + cookbook name + version
- Subsequent collection runs skip versions already scanned
- A manual rescan option must be provided

#### CookStyle Version Profiles

Rather than running CookStyle with its full default rule set, the analysis component should enable only the cops relevant to the **specific target Chef Client versions** being tested. CookStyle organises its cops into version-specific channels that correspond to deprecations introduced in each Chef release.

- For each configured target Chef Client version, determine the applicable set of CookStyle cops. CookStyle cops in the `ChefDeprecations` namespace include comments indicating which Chef version introduced the deprecation.
- When scanning a cookbook for a specific target version, enable only the cops that are relevant to that version and earlier. This prevents false positives from cops that flag deprecations not yet applicable to the target version.
- If the target version cannot be mapped to a specific CookStyle profile (e.g. because the version is very new), fall back to running the full `ChefDeprecations` and `ChefCorrectness` namespaces.
- The CookStyle profile mapping is maintained as a configuration data structure within the application, updated when new Chef Client versions are released.

#### Concurrency

- Test Kitchen runs are independent per cookbook + target Chef Client version combination. Each run must be dispatched as a goroutine, bounded by the `concurrency.test_kitchen_run` worker pool setting (see [Configuration Specification](configuration.md)).
- CookStyle scans are independent per cookbook version. Scans must run in parallel using goroutines, bounded by the `concurrency.cookstyle_scan` worker pool setting (see [Configuration Specification](configuration.md)).
- Each goroutine must capture stdout/stderr from the external process and return it alongside the pass/fail result to the coordinator. Errors must not be silently discarded.

---

#### Design: Test Kitchen Invocation

Test Kitchen is invoked as an external process. The application does **not** link against Test Kitchen as a library — it shells out to the `kitchen` CLI.

> **JSON output policy:** Where possible, external tools run by batch processes should emit JSON for easy parsing and ingestion. Test Kitchen's `list` and `diagnose` subcommands support `--format json`; these must always be invoked with that flag. The action subcommands (`converge`, `verify`, `destroy`) do **not** support a JSON formatter — they produce free-form log output. For these commands, the application captures stdout/stderr as opaque text, relying on the exit code for pass/fail determination and storing the raw output for troubleshooting.

**Embedded Tools**

Test Kitchen, multiple kitchen drivers (`kitchen-dokken`, `kitchen-vcenter`, `kitchen-vra`, `kitchen-ec2`, `kitchen-azurerm`, and others), and a self-contained Ruby runtime are **embedded** in all packaging formats under `/opt/chef-migration-metrics/embedded/`. The application resolves the `kitchen` binary from this embedded directory by default (configurable via the `embedded_bin_dir` setting — see [Configuration Specification](configuration.md)).

External prerequisites depend on the configured driver: Docker for `dokken`, vCenter API access for `vcenter`, AWS credentials for `ec2`, etc. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) for the full driver list and requirements.

> **Note:** The embedded Ruby environment is fully self-contained and does not interfere with any system Ruby, Chef Workstation, or other gem installation on the host. See the [Packaging Specification](packaging.md) for details on how the embedded environment is built and laid out.

**Invocation sequence per cookbook + target Chef Client version**

1. **Check skip condition** — Query the datastore for the most recent test result for this cookbook + target Chef Client version. If it exists and its `commit_sha` matches the current HEAD commit SHA recorded by the data collection component, skip the test run and log at `INFO` severity.

2. **Prepare workspace** — The git repository has already been cloned/pulled by the data collection component. The analysis component operates directly on the local clone directory. No additional file copying is needed.

3. **Generate environment overlay** — Create a temporary `.kitchen.local.yml` file in the cookbook directory that overrides the Chef Client version provisioner attribute. The overlay generation is driver-aware. For `dokken`, the overlay contains the provisioner override only (existing behaviour):

   ```yaml
   # .kitchen.local.yml — generated by chef-migration-metrics
   # DO NOT EDIT — this file is overwritten on each test run
   provisioner:
     product_version: "<TARGET_CHEF_VERSION>"
   ```

   For `kitchen-dokken`, the overlay also sets the Chef Docker image:

   ```yaml
   provisioner:
     chef_version: "<TARGET_CHEF_VERSION>"
   ```

   For all other drivers, the overlay additionally replaces the entire `driver:` block, remaps platforms using the platform image map, and injects credentials from the secret store. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) for the full overlay specification including credential injection, platform mapping, and driver-specific settings.

   The overlay file format (standard provisioner vs. dokken) is determined by inspecting the cookbook's `.kitchen.yml` to identify the driver in use.

4. **Discover instances** — Before running converge, enumerate the available Test Kitchen instances to validate the workspace:

   ```
   kitchen list --format json
   ```

   The `--format json` flag produces machine-parseable output — an array of objects with keys such as `instance`, `driver`, `provisioner`, `verifier`, `transport`, and `last_action`. Parse this to confirm at least one instance is defined and to log the instance names. If the command fails or returns an empty list, log at `ERROR` severity and skip the test run.

5. **Run converge** — Execute:

   ```
   kitchen converge --concurrency=1 --log-level=info
   ```

   `--concurrency=1` is set because parallelism is managed at the application level (across cookbooks), not within Test Kitchen. This prevents resource contention.

   Capture combined stdout/stderr into a buffer. Set a configurable timeout (default: 30 minutes). If the process times out, record the result as a failure with a `timeout` flag.

   Record the exit code:
   - Exit 0 → converge passed
   - Non-zero → converge failed

6. **Run verify** (only if converge passed) — Execute:

   ```
   kitchen verify --concurrency=1 --log-level=info
   ```

   Capture combined stdout/stderr. Same timeout and exit code handling as converge.

7. **Run destroy** (always, regardless of pass/fail) — Execute:

   ```
   kitchen destroy --concurrency=1
   ```

   This cleans up instances. Failure to destroy is logged at `WARN` but does not affect the test result.

8. **Clean up overlay** — Remove the generated `.kitchen.local.yml` file.

9. **Persist result** — Write the test result to the datastore:

   | Field | Value |
   |-------|-------|
   | `cookbook_name` | Name of the cookbook |
   | `commit_sha` | HEAD commit SHA at time of test |
   | `target_chef_version` | The target Chef Client version tested |
   | `converge_passed` | Boolean |
   | `verify_passed` | Boolean |
   | `timed_out` | Boolean |
   | `converge_output` | Captured stdout/stderr from converge |
   | `verify_output` | Captured stdout/stderr from verify |
   | `destroy_output` | Captured stdout/stderr from destroy |
   | `duration_seconds` | Wall-clock time for the full run |
   | `tested_at` | UTC timestamp |

10. **Log result** — Log the outcome at `INFO` (pass) or `ERROR` (fail) severity, including the cookbook name, target version, commit SHA, and duration. The full process output is stored in the `process_output` field of the log entry.

11. **Platform coverage analysis** — After all Test Kitchen and CookStyle runs complete, compute platform coverage for each git-sourced cookbook by comparing the platforms in its `.kitchen.yml` against the platforms of production nodes consuming the cookbook. Persist results to the `cookbook_platform_coverage` table. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § Platform Coverage Analysis.

**Error handling**

- If the cookbook directory does not contain a `.kitchen.yml`, the cookbook is logged as `not testable` at `WARN` severity and skipped. No result is recorded.
- If Test Kitchen is not installed, the application must fail at startup (see Startup Validation).
- If an individual test run fails, the error is captured and persisted. It does not cancel other test runs.
