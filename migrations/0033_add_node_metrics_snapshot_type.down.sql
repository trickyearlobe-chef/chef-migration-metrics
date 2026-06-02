-- Remove node_metrics from the allowed metric snapshot types.
ALTER TABLE metric_snapshots DROP CONSTRAINT chk_metric_snapshots_type;
ALTER TABLE metric_snapshots ADD CONSTRAINT chk_metric_snapshots_type CHECK (
    snapshot_type IN (
        'chef_version_distribution',
        'readiness_summary',
        'cookbook_compatibility',
        'complexity_summary'
    )
);
