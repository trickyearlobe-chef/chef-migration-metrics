#!/bin/bash
# =============================================================================
# Chef Migration Metrics — Pre-remove Script
# =============================================================================
# Stops and disables the service on package removal (not on upgrade).
#
# RPM: $1 = 0 means final removal, $1 = 1 means upgrade
# DEB: $1 = "remove" or "purge" means removal, $1 = "upgrade" means upgrade
#
# See: .claude/specifications/packaging/Specification.md § 2.6
# =============================================================================

set -e

case "$1" in
    # RPM: 0 = final removal
    0)
        systemctl stop chef-migration-metrics.service || true
        systemctl disable chef-migration-metrics.service || true
        ;;
    # DEB: "remove" or "purge" = final removal
    remove|purge)
        systemctl stop chef-migration-metrics.service || true
        systemctl disable chef-migration-metrics.service || true
        ;;
    # RPM upgrade (1) or DEB upgrade — leave the service alone
    *)
        ;;
esac

exit 0
