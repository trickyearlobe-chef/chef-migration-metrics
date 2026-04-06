# Test Kitchen MVP — Specification

> **TL;DR:** Scope the Test Kitchen driver abstraction MVP to three targets: **vSphere** (primary customer target), **Proxmox** (developer lab testing), and **EC2** (fallback if political problems force a pivot). The driver abstraction framework is already built — this spec defines what we validate end-to-end, what we trim, and what we defer. Node Kitchen Archive is parked entirely.

## MVP Drivers

| Driver | Purpose | Priority |
|--------|---------|----------|
| `proxmox` | Developer lab — available now, primary validation target for proving the overlay → TK → VM flow | **P0** — validate first |
| `vcenter` | Customer delivery target — own vSphere environment being stood up (not dependent on customer access) | **P0** — validate once environment is ready |
| `ec2` | Political fallback if customer can't/won't use vSphere | **P2** — config-ready, validate later |

### What dokken?

`dokken` (Docker) remains the default and is already fully working. It is not an MVP target because it's done. The MVP is about proving the **non-dokken path** works end-to-end.

### Validation Order

Proxmox is available today in the developer lab, so it's the first environment we validate against. This lets us iterate on any overlay format issues, gem quirks, or template requirements without coordinating access to vSphere. Once the own vSphere environment is up, we validate the same flow there — the overlay generation is driver-agnostic, so most issues found on Proxmox will already be fixed.

---

## Scope

### In Scope

- **End-to-end validation** of overlay generation → `kitchen converge` → `kitchen verify` → `kitchen destroy` for `vcenter` and `proxmox` drivers against real infrastructure.
- **Driver profiles** for the three MVP targets plus `dokken` (retained as default).
- **Credential injection** for vSphere (`vcenter_password`), Proxmox (`proxmox_token_secret`), and EC2 (`aws_secret_access_key`).
- **Platform image mapping** for VM templates (vSphere/Proxmox) and AMIs (EC2).
- **Transport credentials** — SSH username/password for VM-based targets.
- **Config UI** — the existing admin page at `/admin/test-kitchen` already supports configuring any driver. No changes needed for MVP.
- **Deployment reference documentation** — getting-started guides for vSphere and Proxmox.
- **EC2 config example** — documented and tested in overlay generation, but live EC2 validation deferred to post-MVP unless needed.

### Out of Scope (Parked)

| Feature | Reason | Where Tracked |
|---------|--------|---------------|
| Node Kitchen Archive | Separate feature — may become a TK plugin instead | Parked in `node-kitchen-archive.md` |
| Drivers: `vra`, `azurerm`, `google`, `vagrant`, `openstack` | Not needed for MVP — can be re-added as config-only changes later | Profiles remain in code but are not validated or documented |
| Platform Coverage Analysis | Second-order concern — useful but not blocking the "can we run TK against real VMs?" question | Already implemented, just not MVP-critical |
| Config UI polish (e2e test, credential warnings, per-platform KV editor) | Nice-to-have, not blocking | `todo-test-kitchen-config-ui.md` |

---

## What's Already Built

The driver abstraction framework is **fully implemented and tested**:

- ✅ `.kitchen.local.yml` overlay generation for any driver profile
- ✅ Credential injection via `CMM_TK_SECRET_*` and `CMM_TK_TRANSPORT_*` env vars
- ✅ Platform image mapping with per-platform `driver_settings` and `transport`
- ✅ 9 built-in driver profiles (we keep all in code, validate 3)
- ✅ Startup validation (secret resolution, platform map checks)
- ✅ Config UI for editing driver settings in browser
- ✅ Platform coverage analysis
- ✅ Database migration (`driver` and `platform_name` columns on `git_repo_test_kitchen_results`, `cookbook_platform_coverage` table)
- ✅ Tests: overlay generation, credential handling, profile lookup, coverage analysis

### What's NOT Built / Validated

- ❌ Nobody has run `kitchen converge` with a generated overlay against a real vCenter
- ❌ Nobody has run `kitchen converge` against a real Proxmox host
- ❌ No deployment reference for Proxmox
- ❌ No "getting started" walkthrough for operators
- ❌ No validation that `kitchen-vcenter` and `kitchen-proxmox` gems actually work with the overlay format we generate
- ❌ No validation of VM template requirements (VMware Tools, SSH config, etc.)

---

## MVP Validation Plan

### Phase 1: Proxmox Lab Validation (P0 — available now, do first)

Proxmox is available in the developer lab today. This is where we prove the non-dokken path works end-to-end and shake out any overlay format or gem issues before touching vSphere.

**Goal:** Prove the full cycle works: overlay → provision VM → converge cookbook → verify → destroy.

#### Prerequisites
- Proxmox VE host accessible from the CMM development machine
- VM template(s) with SSH configured (e.g. `ubuntu-2204-template`, `rocky-9-template`)
- API token or username/password for Proxmox API
- `kitchen-proxmox` gem available in the Ruby environment

#### Steps
1. Create Proxmox API credentials in the credential store
2. Configure `test_kitchen` with `driver: proxmox` and a platform map pointing to Proxmox templates
3. Run a collection against a git repo with a simple cookbook + `.kitchen.yml`
4. Verify:
   - `.kitchen.local.yml` is generated with correct `driver: proxmox` block
   - `template:` field points to the Proxmox template name
   - Credentials are injected via env vars (not plaintext in overlay)
   - `kitchen converge` provisions a VM from the template
   - `kitchen verify` runs InSpec tests on the VM
   - `kitchen destroy` removes the VM
   - Results are recorded in `git_repo_test_kitchen_results` with `driver=proxmox`
5. Document any overlay format issues, template requirements, or gem quirks

#### Proxmox Config Example

```yaml
analysis_tools:
  test_kitchen:
    enabled: true
    driver: proxmox
    timeout_minutes: 45
    driver_settings:
      proxmox_url: "https://proxmox.lab.local:8006/api2/json"
      proxmox_token_id: "kitchen@pam!mytoken"
      node: "pve-node-01"
    driver_secrets:
      proxmox_token_secret: proxmox-kitchen-token
    images:
      - name: alma10
        id: "100"
        transport:
          username: kitchen
        chef_download_urls:
          "19.2.12": "https://packages.example.com/chef-19.2.12-1.el9.x86_64.rpm"
    platform_map:
      - kitchen_name: almalinux-10
        image: alma10
```

### Phase 2: vSphere Validation (P0 — own environment being stood up)

Once the Proxmox flow is proven, apply the same pattern to vSphere on the dedicated vSphere environment (being set up independently from the customer). This avoids any dependency on customer access for development and testing. Customer deployment follows once validated.

**Goal:** vSphere integration validated end-to-end on own infrastructure, ready for customer deployment.

#### Prerequisites
- Own vSphere environment accessible from the CMM development host
- VM templates with VMware Tools installed and SSH/WinRM configured
- Service account with permissions: clone VM, power on/off, destroy, guest operations
- Resource pool and folder for ephemeral Kitchen VMs (recommended)
- `kitchen-vcenter` gem available in the Ruby environment

#### Steps
1. Store vCenter password in credential store
2. Configure `test_kitchen` with `driver: vcenter` and platform map
3. Run collection against a test cookbook (own environment, not customer)
4. Validate full cycle (same checks as Proxmox, plus vSphere-specific):
   - VMware Tools detection of VM IP address
   - Correct `clone_type` (linked vs full)
   - Resource pool / folder isolation
   - Windows VM support (WinRM transport) if applicable
5. Validate cleanup — `kitchen destroy` removes VMs, no orphans left in vSphere

#### vSphere Config Example

```yaml
analysis_tools:
  test_kitchen:
    enabled: true
    driver: vcenter
    timeout_minutes: 45
    driver_settings:
      vcenter_host: vcenter.lab.local
      vcenter_username: svc-kitchen@vsphere.local
      vcenter_disable_ssl_verify: false
      clone_type: full
      datacenter: "Lab-DC"
    driver_secrets:
      vcenter_password: vcenter-kitchen-password
    platform_map:
      - kitchen_name: ubuntu-22.04
        image: tmpl-ubuntu-2204-kitchen
        driver_settings:
          cluster: "Cluster-01"
          resource_pool: "Kitchen"
          folder: "kitchen-vms"
        transport:
          username: kitchen
          password_credential: vm-ssh-password
      - kitchen_name: centos-7
        image: tmpl-centos-7-kitchen
        driver_settings:
          cluster: "Cluster-01"
          resource_pool: "Kitchen"
          folder: "kitchen-vms"
        transport:
          username: kitchen
          password_credential: vm-ssh-password
      - kitchen_name: windows-2022
        image: tmpl-win2022-kitchen
        driver_settings:
          cluster: "Cluster-01"
          resource_pool: "Kitchen"
          folder: "kitchen-vms"
          vm_customization:
            numCPUs: 4
            memoryMB: 4096
        transport:
          username: Administrator
          password_credential: vm-winrm-password
```

### Phase 3: EC2 Config Validation (P2 — prepare, don't deploy)

EC2 is the political fallback. We validate overlay generation and document the config, but defer live AWS validation unless the vSphere path is blocked.

**Goal:** Overlay generates correctly. Config is documented and ready to go.

#### Steps
1. Write overlay generation tests with EC2 config (already have some)
2. Verify `ami` field is correctly placed in platform entries
3. Document the EC2 config example and IAM requirements
4. Live validation only if vSphere is blocked

#### EC2 Config Example

```yaml
analysis_tools:
  test_kitchen:
    enabled: true
    driver: ec2
    timeout_minutes: 30
    driver_settings:
      region: eu-west-2
      instance_type: t3.medium
      associate_public_ip: true
      subnet_id: subnet-0abc123
      security_group_ids:
        - sg-0def456
      tags:
        managed-by: chef-migration-metrics
        environment: kitchen
    driver_secrets:
      aws_secret_access_key: aws-kitchen-secret
    platform_map:
      - kitchen_name: ubuntu-22.04
        image: ami-0abcdef1234567890
        transport:
          username: ubuntu
          ssh_key_credential: kitchen-ssh-key
      - kitchen_name: centos-7
        image: ami-0fedcba9876543210
        transport:
          username: centos
          ssh_key_credential: kitchen-ssh-key
      - kitchen_name: windows-2022
        image: ami-0111222333444555
        driver_settings:
          instance_type: t3.large
        transport:
          username: Administrator
          password_credential: win-password
```

---

## Code Changes for MVP

### Trim Driver Profiles (Optional — Cosmetic)

The built-in profiles map currently lists 9 drivers. All 9 can stay in code — they're informational and don't add maintenance burden. However, the **spec, documentation, and deployment references** should focus exclusively on:

| Profile | Image Field | Typical Secrets |
|---------|-------------|-----------------|
| `dokken` | `docker_image` | None |
| `vcenter` | `template` | `vcenter_password` |
| `proxmox` | `template_id` | `proxmox_token_secret` |
| `ec2` | `ami` | `aws_secret_access_key` |

The other profiles (`vra`, `azurerm`, `google`, `vagrant`, `openstack`) remain in the code and work via the `custom` path if anyone needs them, but we don't document, test, or support them in the MVP.

### No New Code Required

The overlay generation, credential injection, and platform mapping are fully implemented. The MVP is a **validation and documentation exercise**, not a coding exercise. If the Proxmox or vSphere validation reveals bugs in the overlay format or credential handling, those are point fixes — not new features.

### Potential Issues to Watch For

These are things that might surface during real-driver validation:

1. **YAML key ordering** — `kitchen-vcenter` and `kitchen-proxmox` may be sensitive to key ordering in the generated overlay. The overlay generator sorts keys alphabetically; some drivers may expect specific ordering.
2. **Nested driver_settings** — vSphere's `vm_customization` is a nested hash. Verify the overlay serialises nested structures correctly.
3. **Template clone timing** — full clones can take minutes. The default 30-minute timeout may be tight for vSphere with large templates. Recommend 45 minutes for VM-based drivers.
4. **IP detection delay** — `kitchen-vcenter` waits for VMware Tools to report the VM's IP. If Tools is slow or misconfigured, the run hangs until timeout.
5. **Proxmox API token format** — Proxmox uses `user@realm!tokenid=secret` format. Verify the credential injection handles the `!` and `=` characters correctly in env vars.
6. **SSH key format** — EC2 uses PEM keys. Verify the `CMM_TK_KEY_*` env var injection writes the key to a temp file and references the file path (not the key content inline).
7. **Orphan cleanup** — If `kitchen destroy` fails (timeout, network issue), VMs are left running. Consider documenting a manual cleanup procedure for each driver.

---

## VM Template Requirements

### Common (All VM-Based Drivers)

- **SSH server** running and accepting connections on port 22 (Linux) or WinRM on 5985/5986 (Windows)
- **User account** matching the `transport.username` with `sudo` (Linux) or Administrator (Windows)
- **Password or SSH key** authentication enabled matching the transport credential type
- **Network connectivity** — the VM must be reachable from the CMM host after provisioning
- **Chef Client NOT pre-installed** — Test Kitchen installs the target version as part of converge

### vSphere-Specific

- **VMware Tools** installed and running — required for IP address detection
- **Template marked as "template"** in vSphere (not just a powered-off VM)
- **Thin provisioning** recommended for faster cloning if using `clone_type: linked`

### Proxmox-Specific

- **QEMU guest agent** installed — required for IP address detection
- **Cloud-init** support recommended for automated SSH key injection
- **Template created via `qm template <vmid>`** in Proxmox

### EC2-Specific

- **AMI** with SSH enabled and the expected username configured (e.g. `ubuntu` for Ubuntu AMIs, `ec2-user` for Amazon Linux)
- **Security group** allowing inbound SSH from the CMM host's IP
- **IAM permissions**: `ec2:RunInstances`, `ec2:TerminateInstances`, `ec2:DescribeInstances`, `ec2:CreateTags`, `ec2:DescribeSubnets`, `ec2:DescribeSecurityGroups`

---

## Acceptance Criteria

### P0 — vSphere (Must Have)

- [ ] `kitchen converge` provisions a VM from a vSphere template using the generated overlay
- [ ] `kitchen verify` runs InSpec tests on the VM over SSH
- [ ] `kitchen destroy` removes the VM from vSphere — no orphans
- [ ] Credentials are injected via env vars, never plaintext in the overlay file
- [ ] Results recorded in `git_repo_test_kitchen_results` with `driver=vcenter`
- [ ] Platform map correctly maps kitchen platform names to vSphere template names
- [ ] Transport credentials (SSH password) work for Linux VMs
- [ ] Deployment reference documentation for vSphere is complete

### P1 — Proxmox (Should Have)

- [ ] Same acceptance criteria as vSphere, but against Proxmox infrastructure
- [ ] Results recorded with `driver=proxmox`
- [ ] Deployment reference documentation for Proxmox is complete

### P2 — EC2 (Config Ready)

- [ ] Overlay generation tests pass with EC2 config
- [ ] `ami` field is correctly placed in platform entries
- [ ] EC2 config example is documented with IAM requirements
- [ ] Live AWS validation deferred unless vSphere is blocked

---

## Deferred Features

These are explicitly **not part of the MVP** but are tracked for later:

| Feature | Tracking |
|---------|----------|
| Node Kitchen Archive (per-node downloadable TK project) | `node-kitchen-archive.md` — parked, may become TK plugin |
| vRA driver validation | Re-add when customer adopts vRA |
| Azure/GCP/OpenStack driver validation | Re-add when needed |
| Vagrant driver validation | Low priority — local dev only |
| Platform coverage analysis documentation | Already implemented, document when MVP is stable |
| Config UI e2e test | `todo-test-kitchen-config-ui.md` |
| Config UI credential reference warnings | `todo-test-kitchen-config-ui.md` |
| Config UI per-platform driver_settings KV editor | `todo-test-kitchen-config-ui.md` |

---

## Related Specifications

| Specification | Relevance |
|---------------|-----------|
| `test-kitchen-drivers.md` | Full driver abstraction spec (superset of this MVP) |
| `test-kitchen-config-ui.md` | Admin UI for driver configuration |
| `node-kitchen-archive.md` | **Parked** — not in MVP scope |
| `analysis.md` | Parent spec for cookbook compatibility testing |
| `secrets-storage.md` | Credential encryption and resolution |
| `packaging.md` | Embedded kitchen driver gems (§4.5) |