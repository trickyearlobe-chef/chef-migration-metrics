// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// CleanupRemovedAutoRules deletes ownership assignments from auto-derivation
// rules that are no longer present in the configuration. This should be called
// once at application startup.
func CleanupRemovedAutoRules(ctx context.Context, db *datastore.DB, cfg config.OwnershipConfig, logger *logging.Logger) error {
	if !cfg.Enabled {
		return nil
	}

	log := logger.WithScope(logging.ScopeOwnership)

	// Build a set of configured rule names.
	configuredRules := make(map[string]bool)
	for _, rule := range cfg.AutoRules {
		configuredRules[rule.Name] = true
	}

	// Find all distinct auto_rule_name values in the DB.
	ruleNames, err := db.ListDistinctAutoRuleNames(ctx)
	if err != nil {
		return fmt.Errorf("listing auto rule names: %w", err)
	}

	totalDeleted := 0
	for _, name := range ruleNames {
		if configuredRules[name] {
			continue // Rule still configured; skip.
		}

		deleted, err := db.DeleteAutoRuleAssignmentsByName(ctx, name)
		if err != nil {
			log.Error(fmt.Sprintf("failed to clean up assignments for removed rule %q: %v", name, err))
			continue
		}

		if deleted > 0 {
			log.Info(fmt.Sprintf("cleaned up %d stale assignments from removed auto-rule %q", deleted, name))
			totalDeleted += deleted

			// Audit the cleanup.
			_ = db.InsertAuditEntry(ctx, datastore.InsertAuditEntryParams{
				Action:    "assignment_deleted",
				Actor:     "system",
				OwnerName: "", // Rule-level cleanup doesn't have a single owner.
				Details:   json.RawMessage(fmt.Sprintf(`{"auto_rule_name":%q,"count":%d,"reason":"rule_removed_from_config"}`, name, deleted)),
			})
		}
	}

	if totalDeleted > 0 {
		log.Info(fmt.Sprintf("startup auto-rule cleanup complete: deleted %d stale assignments from removed rules", totalDeleted))
	}

	return nil
}
