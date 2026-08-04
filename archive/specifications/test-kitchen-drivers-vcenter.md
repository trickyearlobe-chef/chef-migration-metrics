# Test Kitchen Drivers — Deployment Reference: VMware vCenter

## Deployment Reference: VMware vCenter

The primary production deployment uses `kitchen-vcenter`. The execution model is the same for any VM driver (see § Execution Model): Test Kitchen runs on the CMM host, provisions real VMs by cloning vSphere templates, converges cookbooks on those VMs, and destroys them afterwards. The Proxmox PoC driver follows the same model. A planned EC2 deployment would be identical except `driver_settings` point to AWS, `image` values are AMI IDs, and the driver gem is `kitchen-ec2`; likewise vRA and Vagrant once wired.

### vCenter-Specific Prerequisites

In addition to the general driver prerequisites (§ Driver Prerequisites):

- VM templates must have **VMware Tools** installed (required by `kitchen-vcenter` to detect the VM's IP address after boot).
- A **resource pool and folder** are recommended to isolate ephemeral Kitchen VMs from production infrastructure.

### vCenter Config Example

```
analysis_tools:
  test_kitchen:
    driver: vcenter
    driver_settings:
      vcenter_host: vcenter.example.com
      vcenter_username: user@vsphere.local
      vcenter_disable_ssl_verify: false
      clone_type: full
      datacenter: "Datacenter"
    driver_secrets:
      vcenter_password: vcenter-password
    platform_map:
      - kitchen_name: ubuntu-22.04
        image: tmpl-ubuntu-2204-base
        driver_settings:
          cluster: "Cluster-01"
          resource_pool: "Kitchen"
          folder: "kitchen-vms"
        transport:
          username: kitchen
          password_credential: kitchen-vm-password
      - kitchen_name: centos-7
        image: tmpl-centos-7-base
      - kitchen_name: windows-2022
        image: tmpl-win2022-base
        driver_settings:
          vm_customization:
            numCPUs: 4
            memoryMB: 4096
        transport:
          username: Administrator
          password_credential: kitchen-win-password
```

### vCenter Credential Setup

Credentials are managed via the **Admin → Credentials** page in the web UI, or programmatically via the API:

```
POST /api/v1/admin/credentials
{
  "name": "vcenter-password",
  "credential_type": "generic",
  "value": "<password>"
}
```

### vCenter → vRA Migration (planned)

vRA is a planned driver (UI placeholder, not yet wired to a backend). Once it is wired, transitioning from vCenter to vRA is intended to be config-only:

1. Stores the vRA password via **Admin → Credentials** (or `POST /api/v1/admin/credentials`) with name `vra-password`.
2. Updates config: `driver: vra`, replaces `driver_settings` and `driver_secrets`, updates `image` values in the platform map to vRA catalog item names.
3. No code changes — the execution model is the same for all VM drivers.
