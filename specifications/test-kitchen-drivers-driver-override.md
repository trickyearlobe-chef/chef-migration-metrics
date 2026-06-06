# Test Kitchen Drivers — Driver Override Mechanism

## Driver Override Mechanism

The application already generates a `.kitchen.local.yml` to override the provisioner for target Chef Client versions (analysis.md §2, step 3). This spec extends that overlay to optionally replace the entire `driver:` block and per-platform driver settings.

### Driver Model

Every Test Kitchen driver needs the same three things from the application:

1. **Connection settings** — a bag of key-value pairs injected into the overlay's top-level `driver:` block. Some values are plaintext (host, username, region), others are secrets resolved at runtime.
2. **Platform image mapping** — a per-platform lookup from the kitchen platform name to a driver-specific image identifier (template name, AMI ID, Docker image, etc.).
3. **Transport settings** — optional per-platform SSH/WinRM credentials for connecting to provisioned instances.

The application does not need to understand driver semantics. It treats driver settings as opaque key-value pairs that get serialised into the `.kitchen.local.yml`. Driver-specific knowledge lives entirely in the configuration, not in code.

### Built-in Driver Profiles

To reduce configuration burden, the application ships built-in profiles for common drivers. A profile defines which config keys to expect and which image field name to use in the platform map.

| Profile | Driver Gem | Image Field | Typical Secrets |
|---------|-----------|-------------|-----------------|
| `dokken` | kitchen-dokken | `docker_image` | None (Docker socket) |
| `vcenter` | kitchen-vcenter | `template` | `vcenter_password` |
| `vra` | kitchen-vra | `password` | `password` |
| `ec2` | kitchen-ec2 | `ami` | `aws_secret_access_key` |
| `azurerm` | kitchen-azurerm | `image_urn` | `client_secret` |
| `google` | kitchen-google | `image_family` | `service_account_json` |
| `vagrant` | kitchen-vagrant | `box` | None |
| `openstack` | kitchen-openstack | `image_ref` | `os_password` |
| `proxmox` | kitchen-proxmox | `template_id` | `proxmox_token_secret` |
| `custom` | any | configurable | configurable |

The `custom` profile allows any driver gem shipped in the embedded Ruby environment to be used without a built-in profile. The operator supplies all field mappings in config.

### Override Rules

- The overlay MUST replace the entire `driver:` block from the cookbook's `.kitchen.yml`, including any per-platform driver settings.
- The overlay MUST preserve the cookbook's `suites:` and `verifier:` configuration (Test Kitchen merges `.kitchen.local.yml` on top of `.kitchen.yml`; suites and verifier are not included in the overlay).
- The overlay MUST include the provisioner override for the target Chef Client version (existing behaviour).
- Platforms in the cookbook's `.kitchen.yml` that are not present in the platform map are excluded from the overlay and skipped with a `WARN` log.
- For `dokken` with no platform map configured, behaviour is unchanged (all platforms pass through, Docker images used directly).
