#!/bin/bash
# =============================================================================
# Chef Migration Metrics — Post-install Script
# =============================================================================
# Sets ownership on data and log directories and enables the systemd service.
# The service is NOT started automatically — the operator must configure
# /etc/chef-migration-metrics/config.yml first.
#
# See: .claude/specifications/packaging/Specification.md § 2.6
# =============================================================================

set -e

# Seed config.yml from the example file on fresh install only.
# If the operator has already created or edited config.yml, leave it alone.
if [ ! -f /etc/chef-migration-metrics/config.yml ]; then
    cp /etc/chef-migration-metrics/config.yml.example /etc/chef-migration-metrics/config.yml
    chown chef-migration-metrics:chef-migration-metrics /etc/chef-migration-metrics/config.yml
    chmod 0640 /etc/chef-migration-metrics/config.yml
    echo "Created default /etc/chef-migration-metrics/config.yml from example."
fi

# Set ownership on data and log directories
chown -R chef-migration-metrics:chef-migration-metrics /var/lib/chef-migration-metrics
chown -R chef-migration-metrics:chef-migration-metrics /var/log/chef-migration-metrics
chown -R chef-migration-metrics:chef-migration-metrics /etc/chef-migration-metrics/keys

# Reload systemd and enable the service (but do not start — let the operator configure first)
systemctl daemon-reload
systemctl enable chef-migration-metrics.service

echo "Chef Migration Metrics installed. Edit /etc/chef-migration-metrics/config.yml, then run:"
echo "  systemctl start chef-migration-metrics"
