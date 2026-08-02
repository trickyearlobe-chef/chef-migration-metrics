// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import "errors"

var errBoom = errors.New("source exploded")

// failingSource yields a fixed set of rows and then reports an error, standing
// in for a stream that dies part way through.
type failingSource struct {
	columns []string
	rows    []Row
	pos     int
	err     error
}

func (f *failingSource) Columns() []string { return f.columns }

func (f *failingSource) Next() bool {
	if f.pos >= len(f.rows) {
		return false
	}
	f.pos++
	return true
}

func (f *failingSource) Row() Row { return f.rows[f.pos-1] }

func (f *failingSource) Err() error { return f.err }

func (f *failingSource) Close() error { return nil }
