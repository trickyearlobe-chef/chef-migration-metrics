-- Remove complexity_summary from the allowed metric snapshot types.
-- Note: this will fail if any rows with snapshot_type='complexity_summary' exist.
ALTER TABLE metric_snapshots DROP CONSTRAINT chk_metric_snapshots_type;
ALTER TABLE metric_snapshots ADD CONSTRAINT chk_metric_snapshots_type CHECK (
    snapshot_type IN (
        'chef_version_distribution',
        'readiness_summary',
        'cookbook_compatibility'
    )
);
