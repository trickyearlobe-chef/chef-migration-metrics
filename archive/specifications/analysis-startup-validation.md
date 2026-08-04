# Analysis — Startup Validation

## Startup Validation

The analysis component must validate that required tools are available before accepting work. The following checks run at application startup. `kitchen` and `cookstyle` are provided by Chef Workstation installed on the host and are resolved from `PATH`; they are not bundled.

> **JSON output policy:** Where a tool supports JSON output for its version or info command, use it. This makes startup validation output machine-parseable and simplifies version extraction. Where JSON is not available (e.g. `git --version`), parse the single-line text output.

| Tool | Check | Failure behaviour |
|------|-------|-------------------|
| `kitchen` | Run `kitchen version` (resolved from `PATH`) and verify exit code 0. Parse the version string from stdout. | Log `ERROR` and disable Test Kitchen testing. CookStyle-only analysis continues. |
| `cookstyle` | Run `cookstyle --format json --version` (resolved from `PATH`) and verify exit code 0. Parse the version from the output. | Log `ERROR` and disable CookStyle scanning. Test Kitchen testing continues. |
| `git` | Run `git version` and verify exit code 0. Parse the version string from stdout (format: `git version X.Y.Z`). | Fatal — the application must refuse to start, as git is required by the data collection component. |
| `driver credentials` | When a hypervisor driver (`vcenter`, `proxmox`) is configured, verify all `driver_secrets` reference credentials that exist and can be decrypted. | `ERROR` — disable Test Kitchen testing. CookStyle-only analysis continues. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § Startup Validation. |

If both `kitchen` and `cookstyle` are unavailable, the analysis component logs a `WARN` that no compatibility testing is possible. The application continues to run (data collection and dashboard still function) but all cookbooks will be reported as `untested`.

> **Expected state:** Chef Workstation must be installed on the host so that `kitchen` and `cookstyle` are present on `PATH`. When Chef Workstation is absent, the corresponding tool check fails and that capability is disabled (Test Kitchen and/or CookStyle), but the application continues to run.
