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

SERVICE_USER="chef-migration-metrics"
SERVICE_GROUP="chef-migration-metrics"

# Determine if this is a fresh install
# RPM: $1=1 fresh install, $1>=2 upgrade
# DEB: $1="configure" with no $2 = fresh install, with $2 = upgrade
FRESH_INSTALL=false
case "$1" in
    1)          FRESH_INSTALL=true ;;
    configure)  [ -z "$2" ] && FRESH_INSTALL=true ;;
esac

# Config file ownership (always, fast)
chown "${SERVICE_USER}:${SERVICE_GROUP}" /etc/chef-migration-metrics/config.yml 2>/dev/null || true

# Data directory ownership
# Recurse only on fresh install; on upgrade the service already owns its files
# and recursing a large data directory causes multi-minute delays during dnf/apt.
if [ "$FRESH_INSTALL" = "true" ]; then
    chown -R "${SERVICE_USER}:${SERVICE_GROUP}" /var/lib/chef-migration-metrics
    chown -R "${SERVICE_USER}:${SERVICE_GROUP}" /var/log/chef-migration-metrics
    chown -R "${SERVICE_USER}:${SERVICE_GROUP}" /etc/chef-migration-metrics/keys
else
    chown "${SERVICE_USER}:${SERVICE_GROUP}" /var/lib/chef-migration-metrics
    chown "${SERVICE_USER}:${SERVICE_GROUP}" /var/log/chef-migration-metrics
    chown "${SERVICE_USER}:${SERVICE_GROUP}" /etc/chef-migration-metrics/keys
fi

# Reload systemd and enable the service (but do not start — let the operator configure first)
systemctl daemon-reload
systemctl enable chef-migration-metrics.service

echo "Chef Migration Metrics installed. Edit /etc/chef-migration-metrics/config.yml, then run:"
echo "  systemctl start chef-migration-metrics"
