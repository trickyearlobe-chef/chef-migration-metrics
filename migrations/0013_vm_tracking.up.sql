-- SPDX-License-Identifier: Apache-2.0

CREATE TABLE vm_tracking (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_name             TEXT NOT NULL,
    hypervisor_id       TEXT,
    cookbook_name        TEXT NOT NULL,
    suite_name          TEXT NOT NULL,
    platform_name       TEXT NOT NULL,
    batch_id            UUID,
    status              TEXT NOT NULL DEFAULT 'creating',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    expected_destroy_at TIMESTAMPTZ,
    actual_destroy_at   TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_vm_tracking_status ON vm_tracking (status);
CREATE INDEX idx_vm_tracking_vm_name ON vm_tracking (vm_name);
CREATE INDEX idx_vm_tracking_cookbook ON vm_tracking (cookbook_name);
