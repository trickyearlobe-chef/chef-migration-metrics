# Packaging — nFPM Configuration

## 6. nFPM Configuration

The `nfpm.yaml` file at the repository root configures both RPM and DEB package builds:

```yaml
name: chef-migration-metrics
arch: ${ARCH}
platform: linux
version: ${VERSION}
release: 1
section: admin
priority: optional
maintainer: Project Maintainers
description: Tool for planning and tracking Chef Client upgrade projects
vendor: Chef Migration Metrics Project
homepage: https://github.com/trickyearlobe-chef/chef-migration-metrics
license: Apache-2.0

contents:
  - src: ./build/chef-migration-metrics
    dst: /usr/bin/chef-migration-metrics
    file_info:
      mode: 0755

  - src: ./deploy/pkg/config.yml
    dst: /etc/chef-migration-metrics/config.yml
    type: config|noreplace
    file_info:
      mode: 0640

  - dst: /etc/chef-migration-metrics/keys/
    type: dir
    file_info:
      mode: 0700

  - dst: /var/lib/chef-migration-metrics/
    type: dir
    file_info:
      mode: 0750

  - dst: /var/log/chef-migration-metrics/
    type: dir
    file_info:
      mode: 0750

  # Embedded Ruby environment with CookStyle and Test Kitchen
  - src: ./build/embedded/
    dst: /opt/chef-migration-metrics/embedded/
    file_info:
      mode: 0755

  - src: ./deploy/pkg/chef-migration-metrics.service
    dst: /usr/lib/systemd/system/chef-migration-metrics.service
    file_info:
      mode: 0644

  - src: ./deploy/pkg/env-file
    dst: /etc/default/chef-migration-metrics
    type: config|noreplace
    file_info:
      mode: 0640
    packager: deb

  - src: ./deploy/pkg/env-file
    dst: /etc/sysconfig/chef-migration-metrics
    type: config|noreplace
    file_info:
      mode: 0640
    packager: rpm

scripts:
  preinstall: ./deploy/pkg/scripts/preinstall.sh
  postinstall: ./deploy/pkg/scripts/postinstall.sh
  preremove: ./deploy/pkg/scripts/preremove.sh

depends:
  - git

rpm:
  group: Applications/System

deb:
  pre_depends:
    - adduser
```

### 6.1 Building the Embedded Ruby Environment

The embedded Ruby environment is built during the `make build-embedded` step (or as part of `make package-all`) into `./build/embedded/`. The build process:

1. Uses a Docker container (`ruby:3.1-bookworm`) to install gems into an isolated prefix, ensuring a consistent build regardless of the host system. Ruby 3.1 is used to match Chef Workstation 25.13.7.
2. Pins `ffi:1.16.3` first, then installs `cookstyle:7.32.8`, `test-kitchen:3.9.1`, `inspec-bin:5.24.7`, `kitchen-inspec:3.1.0`, all kitchen drivers (vagrant, ec2, azurerm, google, hyperv, vcenter, vra, openstack, digitalocean), and `kitchen-dokken` from the Stromweld fork — all version-pinned to match Chef Workstation 25.13.7.
3. Creates binstubs (`cookstyle`, `kitchen`, `inspec`) with shebangs pointing to `/opt/chef-migration-metrics/embedded/bin/ruby`.
4. Copies the Ruby interpreter and shared libraries into the prefix.
5. Exports the entire tree to `./build/embedded/` on the host for nFPM to package.

This produces a platform-specific artifact — the `ARCH` and `GOOS` of the Ruby build must match the target package architecture.

**Makefile targets:**

| Target | Description |
|--------|-------------|
| `build-embedded` | Build the embedded Ruby environment for the host platform |
| `build-embedded-amd64` | Build for `linux/amd64` |
| `build-embedded-arm64` | Build for `linux/arm64` |
