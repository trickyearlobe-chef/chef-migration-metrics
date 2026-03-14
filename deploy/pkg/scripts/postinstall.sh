#!/bin/bash
# =============================================================================
# Chef Migration Metrics — Post-install Script
# =============================================================================
# Sets ownership on data and log directories and enables the systemd service.
# The service is NOT started automatically — the operator must configure
# /etc/chef-migration-metrics/config.yml first.
#
# config.yml is shipped by the package as type: config|noreplace so RPM/DEB
# lay it down on fresh install and never overwrite operator edits on upgrade.
# No seeding logic is needed here — the package manager handles it.
#
# See: .claude/specifications/packaging/Specification.md § 2.6
# =============================================================================

set -e

# Set ownership on config, data and log directories
chown chef-migration-metrics:chef-migration-metrics /etc/chef-migration-metrics/config.yml 2>/dev/null || true
chown -R chef-migration-metrics:chef-migration-metrics /var/lib/chef-migration-metrics
chown -R chef-migration-metrics:chef-migration-metrics /var/log/chef-migration-metrics
chown -R chef-migration-metrics:chef-migration-metrics /etc/chef-migration-metrics/keys

# Reload systemd and enable the service (but do not start — let the operator configure first)
systemctl daemon-reload
systemctl enable chef-migration-metrics.service

echo "Chef Migration Metrics installed. Edit /etc/chef-migration-metrics/config.yml, then run:"
echo "  systemctl start chef-migration-metrics"
