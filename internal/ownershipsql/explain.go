// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"errors"
	"fmt"
	"strings"

	mssql "github.com/microsoft/go-mssqldb"
)

// Making a driver say what it already knows.
//
// See journeys/ownership-connection.md: what refused has to reach the person
// reading, in its own words, because a message tidied into "could not connect"
// has thrown away the only thing in it worth having. That rule was written
// about US tidying. This is the same failure committed by the driver.
//
// Measured 2026-08-14, after a customer read a table and got six words: when
// SQL Server aborts the process — severity 20 and above — go-mssqldb returns a
// ServerError whose Error() is the constant "SQL Server had internal error",
// with the real message wrapped inside where nothing on a screen can reach it.
// Their whole diagnosis, "Cannot continue the execution because the session is
// in the kill state", number 596, was one unwrap away.
//
// So the message is dug out and put back on the end. Nothing is rewritten and
// nothing is dropped: what we were doing stays, and the server's own words are
// added to it.

// explainDriverError returns err with anything the driver hid appended to it.
//
// It only ever adds. An error that already carries its message is returned
// untouched, because rewriting the ones that read well is how the useful ones
// get lost.
func explainDriverError(err error) error {
	if err == nil {
		return nil
	}
	var inner mssql.Error
	if !errors.As(err, &inner) {
		return err
	}
	// Already legible: the message is in the text somewhere, so there is
	// nothing hidden and nothing to do.
	if inner.Message == "" || strings.Contains(err.Error(), inner.Message) {
		return err
	}

	return explainedError{message: err.Error() + ": " + describeAll(inner), cause: err}
}

// describeAll renders everything the server said, in the order it said it.
//
// SQL Server sends a list and the driver reports the last of it. For a killed
// process the last is "the session is in the kill state", which is what
// happened AFTER the thing that went wrong — so reporting it alone tells
// somebody their query stopped and not why. Measured 2026-08-14 against a
// running server: a terminated process arrived as three errors, the reason
// first and that consequence last, and a customer read only the consequence.
//
// So the whole list goes out, cause first. It is longer than one line and that
// is the point: the first sentence is the answer, and the rest is what a DBA
// will ask about next.
func describeAll(inner mssql.Error) string {
	said := inner.All
	if len(said) == 0 {
		said = []mssql.Error{inner}
	}
	parts := make([]string, 0, len(said))
	for _, e := range said {
		if strings.TrimSpace(e.Message) == "" {
			continue
		}
		parts = append(parts, describeOne(e))
	}
	if len(parts) == 0 {
		return describeOne(inner)
	}
	return strings.Join(parts, "; then ")
}

// describeOne is one of the server's errors with what a DBA asks for first: the
// number, the severity and the line.
func describeOne(e mssql.Error) string {
	return fmt.Sprintf("%s (SQL Server error %d, severity %d, line %d)",
		strings.TrimSuffix(strings.TrimSpace(e.Message), "."), e.Number, e.Class, e.LineNo)
}

// explainedError carries the fuller message and keeps the original reachable,
// so errors.Is and errors.As still find what they were finding — including the
// driver's own types, which the outcome classifier reads.
type explainedError struct {
	message string
	cause   error
}

func (e explainedError) Error() string { return e.message }
func (e explainedError) Unwrap() error { return e.cause }
