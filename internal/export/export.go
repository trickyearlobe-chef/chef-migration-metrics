// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package export implements data export generation for Chef Migration Metrics.
// It provides generators for ready node, blocked node, and cookbook remediation
// exports in CSV, JSON, and Chef search query formats.
//
// Export generation code is decoupled from the web API handlers so it can be
// invoked both synchronously (small result sets streamed inline) and
// asynchronously (large result sets written to disk with job tracking).
package export

import (
	"context"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ExportResult holds the outcome of an export generation operation.
// The caller uses these values to update the export_jobs table and serve
// the file to the requester.
type ExportResult struct {
	// RowCount is the number of data rows written (excluding headers for CSV).
	RowCount int

	// FilePath is the absolute path to the generated export file on disk.
	// Empty when the export was streamed inline (synchronous small exports).
	FilePath string

	// FileSizeBytes is the size of the generated file in bytes.
	FileSizeBytes int64

	// ContentType is the MIME type for the export output, e.g. "text/csv",
	// "application/json", or "text/plain".
	ContentType string

	// Filename is the suggested download filename including extension,
	// e.g. "ready_nodes_2025-06-15.csv".
	Filename string

	// Data holds the generated export content when the export is small
	// enough to be served inline (synchronous mode). For async exports
	// written to disk, this is nil.
	Data []byte
}

// DataStore is the interface consumed by export generators. It mirrors the
// subset of datastore.DB methods needed to assemble export data. Using an
// interface allows generators to be tested with in-memory stubs.
type DataStore interface {
	// ListOrganisations returns all organisations ordered by name.
	ListOrganisations(ctx context.Context) ([]datastore.Organisation, error)

	// ListNodeSnapshotsByOrganisation returns the latest node snapshots for
	// the given organisation.
	ListNodeSnapshotsByOrganisation(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error)

	// ListNodeReadinessForSnapshot returns all readiness records for the
	// given node snapshot, ordered by target_chef_version.
	ListNodeReadinessForSnapshot(ctx context.Context, nodeSnapshotID string) ([]datastore.NodeReadiness, error)

	// ListCookbooksByOrganisation returns all cookbooks belonging to the
	// given organisation.
	ListCookbooksByOrganisation(ctx context.Context, organisationID string) ([]datastore.Cookbook, error)

	// ListCookbookComplexitiesForOrganisation returns all complexity records
	// for cookbooks belonging to the given organisation.
	ListCookbookComplexitiesForOrganisation(ctx context.Context, organisationID string) ([]datastore.CookbookComplexity, error)

	// CountNodeReadiness returns the total, ready, and blocked counts for
	// the given organisation and target Chef version.
	CountNodeReadiness(ctx context.Context, organisationID, targetChefVersion string) (total, ready, blocked int, err error)
}

// Compile-time assertion: *datastore.DB satisfies the export DataStore.
var _ DataStore = (*datastore.DB)(nil)
