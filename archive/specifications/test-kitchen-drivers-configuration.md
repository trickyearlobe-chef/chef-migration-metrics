# Test Kitchen Drivers — Configuration Schema

## Configuration Schema

Full YAML structure under `analysis_tools.test_kitchen`:

```
analysis_tools:
  test_kitchen:
    enabled: true
    timeout_minutes: 30

    # Driver selection — required, no default. Supported: vcenter, proxmox.
    # (vra, ec2, vagrant are planned UI placeholders, not yet wired.)
    driver: proxmox

    # Top-level driver connection settings (plaintext)
    driver_settings:
      proxmox_url: https://proxmox.lab.local:8006/api2/json
      proxmox_token_id: kitchen@pam!mytoken
      node: pve-node-01

    # Top-level driver secrets — values are credential names
    driver_secrets:
      proxmox_token_secret: proxmox-kitchen-token

    # Image field key in the overlay. Built-in profiles set this
    # automatically (e.g. template_id for proxmox, template for vcenter).
    image_field_name: template

    # Fallback Chef package credential for public chef.io downloads.
    # Used for versions that have no chef_download_urls entry in any image.
    chef_license_key_credential: chef-license-key

    # Image registry — define each infrastructure image once.
    # The `id` is the driver-specific image identifier (template ID for
    # proxmox, template name for vcenter, AMI ID for ec2, etc.).
    # Multiple kitchen aliases can reference the same image.
    images:
      - name: alma10
        id: "100"
        driver_settings: {}
        transport:
          username: kitchen
          password_credential: vm-ssh-password
        chef_download_urls:
          "19.2.12": https://packages.example.com/chef-19.2.12-1.el9.x86_64.rpm
          "18.6.0":  https://packages.example.com/chef-18.6.0-1.el9.x86_64.rpm

      - name: win2025
        id: "117"
        transport:
          username: Administrator
          password_credential: vm-winrm-password
        chef_download_urls:
          "19.2.12": https://packages.example.com/chef-19.2.12-1-x64.msi

    # Platform map — pure alias table: cookbook platform name → image name.
    # Multiple kitchen names can reference the same image.
    platform_map:
      - kitchen_name: almalinux-10
        image: alma10
      - kitchen_name: centos-7
        image: alma10
      - kitchen_name: rhel-9
        image: alma10
      - kitchen_name: windows-2025
        image: win2025
      - kitchen_name: win-2025
        image: win2025
```

### ImageEntry Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Operator-defined label. Must be unique within the config. Used as the reference value in `platform_map[].image`. |
| `id` | Yes | Driver-specific image identifier, required for all drivers. The built-in profile determines which YAML key it maps to in the overlay (e.g. `template_id` for proxmox, `template` for vcenter). |
| `driver_settings` | No | Per-image driver setting overrides, merged on top of top-level driver_settings. |
| `transport` | No | Transport credentials: `username`, `password_credential`, `ssh_key_credential`. |
| `chef_download_urls` | No | Map of `version → URL`. When set for the target version, the overlay uses `download_url` instead of `product_version`. Platforms without an entry fall back to the top-level `chef_license_key_credential`. |

### Platform Map Entry Fields

| Field | Required | Description |
|-------|----------|-------------|
| `kitchen_name` | Yes | Platform name as it appears in the cookbook's `.kitchen.yml`. |
| `image` | Yes | Name of an entry in the `images` list. |

### Defaults

| Setting | Default |
|---------|---------|
| `driver` | none (required) |
| `timeout_minutes` | `30` |
| `driver_settings` | empty map |
| `driver_secrets` | empty map |
| `images` | empty list |
| `platform_map` | empty list |
| `image_field_name` | set by built-in profile |
| `chef_license_key_credential` | empty (optional; fallback for versions without a `chef_download_urls` entry) |

### Driver Change Example: Proxmox → vCenter

Only `driver`, `driver_settings`, `driver_secrets`, and each image `id` change.
The image names, transport credentials, and platform map are unchanged.

```
# Before (Proxmox)
driver: proxmox
driver_settings:
  proxmox_url: https://proxmox.lab.local:8006/api2/json
  proxmox_token_id: kitchen@pam!mytoken
  node: pve-node-01
driver_secrets:
  proxmox_token_secret: proxmox-kitchen-token
images:
  - name: alma10
    id: "100"

# After (vCenter)
driver: vcenter
driver_settings:
  vcenter_host: vcenter.example.com
  vcenter_username: svc-kitchen@vsphere.local
driver_secrets:
  vcenter_password: vcenter-kitchen-password
images:
  - name: alma10
    id: tmpl-alma10-kitchen
```

The `platform_map` does not change at all when switching drivers.
