-- SPDX-License-Identifier: Apache-2.0

CREATE TABLE node_kitchen_runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_name           TEXT NOT NULL,
    organisation_name   TEXT NOT NULL,
    target_chef_version TEXT NOT NULL,
    cookbook_source      TEXT NOT NULL,
    platform_name       TEXT NOT NULL,
    template_used       TEXT,
    run_list            JSONB NOT NULL DEFAULT '[]',
    cookbook_versions    JSONB NOT NULL DEFAULT '{}',
    converge_passed     BOOLEAN,
    verify_passed       BOOLEAN,
    converge_output     TEXT,
    verify_output       TEXT,
    destroy_output      TEXT,
    duration_seconds    INTEGER,
    error_message       TEXT,
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    vm_tracking_id      UUID REFERENCES vm_tracking(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Upsert constraint: latest result per (node, org, version, source) combination
CREATE UNIQUE INDEX idx_node_kitchen_runs_upsert
    ON node_kitchen_runs (node_name, organisation_name, target_chef_version, cookbook_source);

CREATE INDEX idx_node_kitchen_runs_node ON node_kitchen_runs (node_name, organisation_name);
CREATE INDEX idx_node_kitchen_runs_org ON node_kitchen_runs (organisation_name);
