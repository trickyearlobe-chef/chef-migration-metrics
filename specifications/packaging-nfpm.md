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

The packages ship only the Go binary, config file, systemd unit, and env-file. No Ruby tree is bundled. Cookbook compatibility testing requires **Chef Workstation** installed on the host; the application resolves `cookstyle` and `kitchen` from `PATH`.
