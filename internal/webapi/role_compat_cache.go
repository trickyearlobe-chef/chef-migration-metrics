// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

type roleCompatCacheEntry struct {
	summary   datastore.RoleFilterSummary
	compatMap map[string]string // role_name → compat_status
	expiresAt time.Time
}

// cachedRoleCompatSummary returns the compat summary from cache or fetches it.
// Cache TTL is 60 seconds. The cache key is orgs+name+targetVersion (NOT compat filter).
func (r *Router) cachedRoleCompatSummary(ctx context.Context, f datastore.RoleFilter) (datastore.RoleFilterSummary, map[string]string) {
	key := roleCompatCacheKey(f)
	if v, ok := r.roleCompatCache.Load(key); ok {
		entry := v.(*roleCompatCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.summary, entry.compatMap
		}
	}
	summary, compatMap, err := r.db.GetRoleCompatSummary(ctx, f)
	if err != nil {
		r.logf("WARN", "computing role compat summary: %v", err)
		return datastore.RoleFilterSummary{}, nil
	}
	r.roleCompatCache.Store(key, &roleCompatCacheEntry{
		summary:   summary,
		compatMap: compatMap,
		expiresAt: time.Now().Add(60 * time.Second),
	})
	return summary, compatMap
}

func roleCompatCacheKey(f datastore.RoleFilter) string {
	orgs := append([]string(nil), f.OrganisationNames...)
	sort.Strings(orgs)
	return strings.Join(orgs, ",") + "|" + f.Name + "|" + f.TargetChefVersion
}
