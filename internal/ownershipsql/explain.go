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

	// The number, the severity and the line are what somebody searches for and
	// what a DBA asks for first. They cost a few characters and they are the
	// difference between a message and a lead.
	detail := fmt.Sprintf("%s (SQL Server error %d, severity %d, line %d)",
		strings.TrimSuffix(inner.Message, "."), inner.Number, inner.Class, inner.LineNo)

	return explainedError{message: err.Error() + ": " + detail, cause: err}
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
