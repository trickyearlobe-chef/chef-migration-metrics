# Analysis — Startup Validation

## Startup Validation

The analysis component must validate that required tools are available before accepting work. The following checks run at application startup. For `kitchen` and `cookstyle`, the application first looks in the configured `embedded_bin_dir` (default: `/opt/chef-migration-metrics/embedded/bin/`), then falls back to `PATH` lookup.

> **JSON output policy:** Where a tool supports JSON output for its version or info command, use it. This makes startup validation output machine-parseable and simplifies version extraction. Where JSON is not available (e.g. `git --version`), parse the single-line text output.

| Tool | Check | Failure behaviour |
|------|-------|-------------------|
| `kitchen` | Run `<embedded_bin_dir>/kitchen version` (or `kitchen version` via PATH fallback) and verify exit code 0. Parse the version string from stdout. | Log `ERROR` and disable Test Kitchen testing. CookStyle-only analysis continues. |
| `cookstyle` | Run `<embedded_bin_dir>/cookstyle --format json --version` (or `cookstyle --format json --version` via PATH fallback) and verify exit code 0. Parse the version from the output. | Log `ERROR` and disable CookStyle scanning. Test Kitchen testing continues. |
| `git` | Run `git version` and verify exit code 0. Parse the version string from stdout (format: `git version X.Y.Z`). | Fatal — the application must refuse to start, as git is required by the data collection component. |
| `docker` | Run `docker info --format json` and verify exit code 0. Parse the JSON output to extract the Docker server version and confirm the daemon is responsive. | Log `WARN` — Required only when `test_kitchen.driver` is `dokken` (default). When a non-dokken driver is configured, this check is skipped. |
| `driver credentials` | When `test_kitchen.driver` is non-dokken, verify all `driver_secrets` reference credentials that exist and can be decrypted. | `ERROR` — disable Test Kitchen testing. CookStyle-only analysis continues. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § Startup Validation. |

If both `kitchen` and `cookstyle` are unavailable, the analysis component logs a `WARN` that no compatibility testing is possible. The application continues to run (data collection and dashboard still function) but all cookbooks will be reported as `untested`.

> **Expected state:** In a standard installation (RPM or DEB package), the embedded `kitchen` and `cookstyle` binaries are always present under `/opt/chef-migration-metrics/embedded/bin/` and startup validation will pass. The fallback to `PATH` lookup is provided for development environments and source builds where the embedded tree may not be present.
