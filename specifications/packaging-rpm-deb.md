# Packaging — RPM Package, DEB Package

## 2. RPM Package

### 2.1 Tooling

RPM packages are built using [nFPM](https://nfpm.goreleaser.com/), a Go-based packager that does not require `rpmbuild` or a full RPM toolchain. The nFPM configuration is maintained in a `nfpm.yaml` file at the repository root.

### 2.2 Package Metadata

| Field | Value |
|-------|-------|
| Name | `chef-migration-metrics` |
| Version | Injected from the build version string |
| Release | `1` (incremented for packaging-only changes) |
| Architecture | `x86_64` or `aarch64` (matches `GOARCH`) |
| License | `Apache-2.0` |
| Vendor | Project maintainers |
| Description | Tool for planning and tracking Chef Client upgrade projects |
| URL | Repository URL |

### 2.3 Dependencies

| Dependency | Type | Reason |
|------------|------|--------|
| `git` | Requires | Cookbook repository clone and pull operations |
| `shadow-utils` | Requires | Provides `useradd` / `groupadd` for the service account |

Test Kitchen, CookStyle, and their Ruby runtime are **embedded** in the package under `/opt/chef-migration-metrics/embedded/`. This self-contained Ruby environment eliminates external dependencies on Chef Workstation or system Ruby. See section 2.4 for the filesystem layout.

### 2.4 Filesystem Layout

```
/usr/bin/chef-migration-metrics                          # Application binary
/etc/chef-migration-metrics/config.yml                   # Default configuration file (noreplace)
/etc/chef-migration-metrics/keys/                        # Directory for Chef API private keys (0700)
/var/lib/chef-migration-metrics/                         # Working directory for git clones and cookbook downloads
/var/log/chef-migration-metrics/                         # Optional file-based log output (stdout preferred)
/usr/lib/systemd/system/chef-migration-metrics.service   # systemd unit file
/opt/chef-migration-metrics/embedded/                    # Self-contained Ruby environment
/opt/chef-migration-metrics/embedded/bin/ruby            # Embedded Ruby interpreter
/opt/chef-migration-metrics/embedded/bin/cookstyle       # Embedded CookStyle binary
/opt/chef-migration-metrics/embedded/bin/kitchen         # Embedded Test Kitchen binary
/opt/chef-migration-metrics/embedded/lib/                # Ruby standard library and installed gems
```

Configuration files are marked `%config(noreplace)` so that upgrades do not overwrite user-customised files.

The embedded Ruby tree is fully self-contained and does not interfere with any system Ruby installation. The application resolves `cookstyle` and `kitchen` from `/opt/chef-migration-metrics/embedded/bin/` by default (see [Configuration Specification](configuration.md) for the `embedded_bin_dir` setting), falling back to `PATH` lookup if the embedded directory does not exist.

### 2.5 systemd Unit File

```ini
[Unit]
Description=Chef Migration Metrics
Documentation=https://github.com/trickyearlobe-chef/chef-migration-metrics
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=chef-migration-metrics
Group=chef-migration-metrics
ExecStart=/usr/bin/chef-migration-metrics --config /etc/chef-migration-metrics/config.yml
Restart=on-failure
RestartSec=10
EnvironmentFile=-/etc/sysconfig/chef-migration-metrics
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/chef-migration-metrics /var/log/chef-migration-metrics
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

The `EnvironmentFile` directive points to `/etc/sysconfig/chef-migration-metrics` (RPM convention) where operators can set environment variable overrides such as `DATABASE_URL` and `CMM_CREDENTIAL_ENCRYPTION_KEY` without modifying the config file.

### 2.6 Pre/Post Install Scripts

**Pre-install:**

```bash
#!/bin/bash
# Create the service account if it does not exist
getent group chef-migration-metrics >/dev/null || groupadd -r chef-migration-metrics
getent passwd chef-migration-metrics >/dev/null || \
    useradd -r -g chef-migration-metrics -d /var/lib/chef-migration-metrics \
    -s /sbin/nologin -c "Chef Migration Metrics" chef-migration-metrics
```

**Post-install:**

```bash
#!/bin/bash
# Set ownership on data and log directories
chown -R chef-migration-metrics:chef-migration-metrics /var/lib/chef-migration-metrics
chown -R chef-migration-metrics:chef-migration-metrics /var/log/chef-migration-metrics
chown -R chef-migration-metrics:chef-migration-metrics /etc/chef-migration-metrics/keys

# Reload systemd and enable the service (but do not start — let the operator configure first)
systemctl daemon-reload
systemctl enable chef-migration-metrics.service

echo "Chef Migration Metrics installed. Edit /etc/chef-migration-metrics/config.yml, then run:"
echo "  systemctl start chef-migration-metrics"
```

**Pre-uninstall:**

```bash
#!/bin/bash
# Stop and disable the service on removal (not on upgrade)
if [ "$1" = "0" ]; then
    systemctl stop chef-migration-metrics.service || true
    systemctl disable chef-migration-metrics.service || true
fi
```

---

## 3. DEB Package

### 3.1 Tooling

DEB packages are also built using nFPM. The same `nfpm.yaml` file supports both RPM and DEB output formats.

### 3.2 Package Metadata

| Field | Value |
|-------|-------|
| Name | `chef-migration-metrics` |
| Version | Injected from the build version string |
| Architecture | `amd64` or `arm64` |
| Section | `admin` |
| Priority | `optional` |
| License | `Apache-2.0` |
| Maintainer | Project maintainers |
| Description | Tool for planning and tracking Chef Client upgrade projects |
| Homepage | Repository URL |

### 3.3 Dependencies

| Dependency | Type | Reason |
|------------|------|--------|
| `git` | Depends | Cookbook repository clone and pull operations |
| `adduser` | Pre-Depends | Service account creation |

Test Kitchen, CookStyle, and their Ruby runtime are **embedded** in the package under `/opt/chef-migration-metrics/embedded/`, identical to the RPM layout (section 2.4).

### 3.4 Filesystem Layout

Identical to the RPM layout (section 2.4) with one exception:

- The environment file is at `/etc/default/chef-migration-metrics` (Debian convention) instead of `/etc/sysconfig/chef-migration-metrics`.
- The systemd unit file references this path in `EnvironmentFile`.

### 3.5 systemd Unit File

Identical to the RPM unit file (section 2.5) except the `EnvironmentFile` line:

```ini
EnvironmentFile=-/etc/default/chef-migration-metrics
```

### 3.6 Maintainer Scripts

The DEB package uses `preinst`, `postinst`, and `prerm` scripts that are functionally identical to the RPM scripts in section 2.6, adapted for Debian conventions:

- `preinst` creates the service account using `adduser --system --group --no-create-home`.
- `postinst` sets ownership and enables the service.
- `prerm` stops and disables the service on purge or remove.
