# Test Kitchen Drivers — Overview

## Overview

The Test Kitchen driver abstraction was originally generalised from a Docker-only (`kitchen-dokken`) model into a pluggable driver architecture. Two drivers are wired to a hypervisor backend and supported today: **vcenter** (production) and **proxmox** (proof-of-concept). Additional drivers — **vra**, **ec2**, **vagrant** — exist as UI dropdown placeholders and overlay-profile stubs but are not yet wired to a backend (planned roadmap items). `kitchen` and the relevant driver gems are provided by Chef Workstation on the CMM host; there is no embedded Ruby tree.

### Execution Model

Test Kitchen runs on the CMM host as a child process. It is Test Kitchen — not the application — that provisions test targets, converges cookbooks, runs verification, and destroys everything afterwards. The application's role is to generate the `.kitchen.local.yml` overlay, inject credentials, invoke `kitchen`, and record results. This is the same regardless of driver.

The end-to-end flow:

1. **The application generates a `.kitchen.local.yml` overlay** — overrides the cookbook's `.kitchen.yml` with the configured driver, remaps platforms to available images via the lookup table, and wires in credentials.
2. **Test Kitchen provisions the test target** — a vSphere VM cloned from a template (vcenter) or a Proxmox VE VM (proxmox); planned drivers would provision an EC2 instance from an AMI (ec2), a vRA catalog deployment (vra), or a Vagrant box (vagrant). Test Kitchen handles all provisioning through the driver plugin.
3. **Test Kitchen converges the cookbook** — connects to the target over SSH (Linux) or WinRM (Windows), installs the target Chef Client version, and runs the cookbook's run list.
4. **Test Kitchen runs verification** — InSpec tests execute against the live target.
5. **Test Kitchen destroys the target** — VM terminated, instance destroyed. Ephemeral — nothing persists between runs.
6. **The application records the result** — exit codes, captured output, timing, driver, and platform name are persisted.

What changes between drivers is what Test Kitchen provisions and how it connects:

| Driver | Status | Target | Provisioned From | Transport |
|--------|--------|--------|-----------------|-----------|
| `vcenter` | Supported (production) | VM on vSphere | VM template | SSH / WinRM |
| `proxmox` | Supported (PoC) | VM on Proxmox VE | VM template | SSH |
| `ec2` | Planned (UI placeholder, not wired) | EC2 instance on AWS | AMI | SSH |
| `vra` | Planned (UI placeholder, not wired) | VM via vRealize Automation | Catalog item | SSH / WinRM |
| `vagrant` | Planned (UI placeholder, not wired) | Local Vagrant VM | Box | SSH |

### Driver Prerequisites

These apply to all VM drivers (vCenter, Proxmox, and the planned ec2/vra/vagrant).

- **Images or templates** in the target infrastructure for each platform in the platform map. Linux images need SSH configured; Windows images need WinRM configured.
- **Service account or API credentials** with permissions to create, connect to, and destroy machines.
- **Network access** from the CMM host to the infrastructure API and to the provisioned machines (SSH port 22 / WinRM port 5985–5986).
- **`kitchen` binary and the relevant driver gem** available on the CMM host (provided by Chef Workstation; there is no embedded Ruby tree).

### What This Spec Adds

1. **Driver override** — generate `.kitchen.local.yml` overlays that replace the driver block for any supported driver, so existing cookbook repos need no reconfiguration.
2. **Credential injection** — driver passwords, access keys, and transport secrets come from the encrypted credentials table, never hardcoded.
3. **Platform image mapping** — a lookup table that translates kitchen platform names (e.g. `ubuntu-22.04`) to driver-specific image identifiers (vSphere template names, AMI IDs, Azure images, etc.) available in the target infrastructure.
4. **Platform coverage analysis** — cross-references kitchen platforms against production node data to find untested gaps.
5. **Driver migration path** — switching between supported drivers (e.g. Proxmox → vCenter) is a YAML config change with no code modifications. Planned drivers (vra, ec2, vagrant) additionally require backend wiring before they can be selected.
