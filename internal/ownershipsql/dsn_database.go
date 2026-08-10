// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import "github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"

// A connection must name the database it reads. See journeys/ownership-intake.md.
//
// The check itself lives in internal/secrets, next to the credential type that
// applies it when a connection is stored. This file had its own copy, and the
// two drifted immediately: a customer's SQL Server connection carrying
// `ApplicationIntent=ReadOnly;MultiSubnetFailover=True` was accepted when it was
// stored and refused when it was used, which is the worst of both — the refusal
// arrived at the person who could not fix it, about a string that had already
// been accepted.
//
// So there is one implementation and this calls it. Checked here as well as at
// storage time because a connection stored before the credential type existed,
// or stored as a generic secret, reaches the database without ever having been
// looked at.
func validateDSNNamesDatabase(driver, dsn string) error {
	return secrets.ValidateDatabaseURL(dsn)
}
