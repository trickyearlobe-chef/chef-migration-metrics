# Test Kitchen Drivers — Overview

## Overview

The existing analysis component (analysis.md §2) hardcodes `kitchen-dokken` as the sole Test Kitchen driver. Packaging already ships 11 drivers including `kitchen-vcenter`, `kitchen-vra`, `kitchen-ec2`, `kitchen-azurerm`, `kitchen-google`, `kitchen-proxmox`, and others (packaging.md §4.5).

### Execution Model

Test Kitchen runs on the CMM host as a child process. It is Test Kitchen — not the application — that provisions test targets, converges cookbooks, runs verification, and destroys everything afterwards. The application's role is to generate the `.kitchen.local.yml` overlay, inject credentials, invoke `kitchen`, and record results. This is the same regardless of driver.

The end-to-end flow:

1. **The application generates a `.kitchen.local.yml` overlay** — overrides the cookbook's `.kitchen.yml` with the configured driver, remaps platforms to available images via the lookup table, and wires in credentials.
2. **Test Kitchen provisions the test target** — depending on the driver, this is a Docker container (dokken), a vSphere VM cloned from a template (vcenter), an EC2 instance launched from an AMI (ec2), a vRA catalog deployment (vra), etc. Test Kitchen handles all provisioning through the driver plugin.
3. **Test Kitchen converges the cookbook** — connects to the target over SSH (Linux) or WinRM (Windows), installs the target Chef Client version, and runs the cookbook's run list.
4. **Test Kitchen runs verification** — InSpec tests execute against the live target.
5. **Test Kitchen destroys the target** — container deleted, VM terminated, instance destroyed. Ephemeral — nothing persists between runs.
6. **The application records the result** — exit codes, captured output, timing, driver, and platform name are persisted.

What changes between drivers is what Test Kitchen provisions and how it connects:

| Driver | Target | Provisioned From | Transport |
|--------|--------|-----------------|-----------|
| `dokken` | Docker container on local host | Docker image | Docker exec |
| `vcenter` | VM on vSphere | VM template | SSH / WinRM |
| `proxmox` | VM on Proxmox VE | VM template | SSH |
| `ec2` | EC2 instance on AWS | AMI | SSH |
| `vra` | VM via vRealize Automation | Catalog item | SSH / WinRM |
| `azurerm` | VM on Azure | Image URN | SSH / WinRM |
| `google` | Instance on GCP | Image family | SSH |
| `openstack` | Instance on OpenStack | Image ref | SSH |

### Non-Dokken Prerequisites

These apply to any non-dokken driver (vCenter, EC2, vRA, Azure, GCP, OpenStack, Proxmox, etc.). Docker is **not** required and the Docker startup check is skipped.

- **Images or templates** in the target infrastructure for each platform in the platform map. Linux images need SSH configured; Windows images need WinRM configured.
- **Service account or API credentials** with permissions to create, connect to, and destroy machines.
- **Network access** from the CMM host to the infrastructure API and to the provisioned machines (SSH port 22 / WinRM port 5985–5986).
- **`kitchen` binary and the relevant driver gem** available on the CMM host (via embedded dir, Chef Workstation, or standalone gem install).

### What This Spec Adds

1. **Driver override** — generate `.kitchen.local.yml` overlays that replace the driver block for any supported driver, so existing cookbook repos need no reconfiguration.
2. **Credential injection** — driver passwords, access keys, and transport secrets come from the encrypted credentials table, never hardcoded.
3. **Platform image mapping** — a lookup table that translates kitchen platform names (e.g. `ubuntu-22.04`) to driver-specific image identifiers (vSphere template names, AMI IDs, Azure images, etc.) available in the target infrastructure.
4. **Platform coverage analysis** — cross-references kitchen platforms against production node data to find untested gaps.
5. **Driver migration path** — switching drivers (e.g. vCenter → vRA, or vCenter → EC2) is a YAML config change with no code modifications.
