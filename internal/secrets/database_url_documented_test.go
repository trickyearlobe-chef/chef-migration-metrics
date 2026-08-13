// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"regexp"
	"testing"
)

// The refusal has to recommend a shape it would accept.
//
// This file used to hold a second check as well: it read the connection formats
// displayed on screen and put each through the validator, after a colleague was
// given an on-screen example, asked whether extra options could be added, was
// told to try it, and was refused — by a check that named the driver when the
// driver was right.
//
// That check has moved rather than gone. The screen no longer displays a format
// as a literal string: it proposes a whole connection, joined round the password
// marker so that no line of it has the shape of a real connection carrying a
// password. Reading it needs to know how it is written, so the check lives next
// to the code that composes it —
// TestTheConnectionsTheScreenProposesCanActuallyBeSent in internal/ownershipsql,
// which puts each proposal back together and requires it to compose and to name
// its database. That is a stronger question than the one asked here, and it is
// asked of the thing people actually copy.

// documentedExample matches a connection string as written in prose, stopping
// at a quote, whitespace or a "\n" escape.
var documentedExample = regexp.MustCompile(
	`(?:jdbc:)?(?:postgres|postgresql|sqlserver|mssql)://[^"'` + "`" + `\s\\]+`)

// The refusal names two shapes as the way to fix it. Somebody reading it will
// copy one, so both have to be accepted.
func TestDatabaseURL_AcceptsTheShapesItsOwnRefusalRecommends(t *testing.T) {
	recommended := documentedExample.FindAllString(ErrDatabaseURLNamesNoDatabase.Error(), -1)
	if len(recommended) == 0 {
		t.Fatal("the refusal recommends no shape, so it tells the reader nothing to do")
	}
	for _, dsn := range recommended {
		if err := ValidateDatabaseURL(dsn); err != nil {
			t.Errorf("the refusal recommends a shape it would refuse\n  shows: %s\n  error: %v", dsn, err)
		}
	}
}
