# Test Kitchen Drivers — Credential Model

## Credential Model

### Design Principles

Driver credentials use the existing `generic` credential type in the credentials table. There is no per-driver credential type — a password is a password regardless of whether it authenticates to vCenter, vRA, AWS, or Azure. The credential's `name` and `metadata` JSONB field provide context (e.g. `{"driver": "vcenter", "host": "vcenter.example.com"}`).

This avoids credential type proliferation as new drivers are added. The `generic` type already validates as non-empty string, which is sufficient for all driver secrets.

### Connection Secrets

Each driver configuration block has a `secrets` map. Keys are driver setting names; values are credential references.

```
test_kitchen:
  driver_settings:
    vcenter_host: vcenter.example.com
    vcenter_username: user@vsphere.local
  driver_secrets:
    vcenter_password: vcenter-password        # → credentials.name
```

At runtime each secret is resolved via the credential resolver (secrets-storage.md § Credential Resolution Precedence): database → environment variable → error. Resolved values are set as process environment variables named `CMM_TK_SECRET_<UPPER_KEY>` (e.g. `CMM_TK_SECRET_VCENTER_PASSWORD`). The overlay references them via ERB: `<%= ENV['CMM_TK_SECRET_VCENTER_PASSWORD'] %>`.

Environment variable names are derived deterministically: prefix `CMM_TK_SECRET_`, then the `driver_secrets` key uppercased with hyphens replaced by underscores.

### Transport Secrets

Per-platform transport credentials (SSH/WinRM passwords) are referenced in the platform map entry's `transport` block. They follow the same pattern — a credential name resolved at runtime, injected via environment variable.

Environment variable naming for transport secrets: `CMM_TK_TRANSPORT_<NORMALIZED_PLATFORM>` where the platform name is uppercased with hyphens and dots replaced by underscores (e.g. `ubuntu-22.04` → `CMM_TK_TRANSPORT_UBUNTU_22_04`).

### Plaintext Handling

All credential values are zeroed from memory after the `.kitchen.local.yml` is written and the environment variables are set. The environment variables persist only for the duration of the Test Kitchen child process and are cleared after the process exits (same process group cleanup as existing behaviour).
