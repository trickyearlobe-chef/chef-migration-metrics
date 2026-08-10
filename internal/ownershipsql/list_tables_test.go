// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"strings"
	"testing"
)

// In a system of record the thing an administrator wants is very often a view
// somebody already built for exactly this purpose. Listing only tables sends
// them off to rebuild it badly against a schema they cannot inspect. See
// journeys/ownership-intake.md.
//
// The query is built per driver, so this checks every driver rather than the
// one that happens to be in front of us.
func TestListTablesQuery_IncludesViewsNotJustTables(t *testing.T) {
	for _, driver := range SupportedDrivers {
		query := listTablesQuery(driver)
		if query == "" {
			t.Errorf("%s: no table listing query", driver)
			continue
		}
		if !strings.Contains(strings.ToUpper(query), "VIEW") {
			t.Errorf("%s: lists tables but not views — an administrator would be sent to rebuild a view that already exists:\n%s",
				driver, query)
		}
	}
}
