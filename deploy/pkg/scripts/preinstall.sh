#!/bin/bash
# =============================================================================
# Chef Migration Metrics — Pre-install Script
# =============================================================================
# Creates the service account and group if they do not already exist.
# Runs before package files are laid down on disk.
#
# See: .claude/specifications/packaging/Specification.md § 2.6
# =============================================================================

set -e

SERVICE_USER="chef-migration-metrics"
SERVICE_GROUP="chef-migration-metrics"

# Create the group if it does not exist
getent group "${SERVICE_GROUP}" >/dev/null || groupadd -r "${SERVICE_GROUP}"

# Create the user if it does not exist
getent passwd "${SERVICE_USER}" >/dev/null || \
    useradd -r -g "${SERVICE_GROUP}" \
        -d /var/lib/chef-migration-metrics \
        -s /sbin/nologin \
        -c "Chef Migration Metrics" \
        "${SERVICE_USER}"

exit 0
