// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// DataStore is the interface consumed by the web API handlers. It abstracts
// the concrete *datastore.DB so that handlers can be tested with in-memory
// stubs instead of a live PostgreSQL database.
//
// Every method listed here is called by at least one handler or by the
// health-check endpoint. The signatures match the corresponding methods on
// *datastore.DB exactly.
type DataStore interface {
	// Ping verifies the database is reachable (used by handleHealth).
	Ping(ctx context.Context) error

	// -----------------------------------------------------------------
	// Organisations
	// -----------------------------------------------------------------

	// ListOrganisations returns all organisations ordered by name.
	ListOrganisations(ctx context.Context) ([]datastore.Organisation, error)

	// GetOrganisationByName returns the organisation with the given name.
	// Returns datastore.ErrNotFound if no such organisation exists.
	GetOrganisationByName(ctx context.Context, name string) (datastore.Organisation, error)

	// -----------------------------------------------------------------
	// Collection runs
	// -----------------------------------------------------------------

	// GetLatestCollectionRun returns the most recent collection run for the
	// given organisation. Returns datastore.ErrNotFound if none exist.
	GetLatestCollectionRun(ctx context.Context, organisationID string) (datastore.CollectionRun, error)

	// ListCollectionRuns returns collection runs for an organisation ordered
	// by started_at descending. If limit > 0 at most limit rows are returned.
	ListCollectionRuns(ctx context.Context, organisationID string, limit int) ([]datastore.CollectionRun, error)

	// -----------------------------------------------------------------
	// Node snapshots
	// -----------------------------------------------------------------

	// ListNodeSnapshotsByOrganisation returns the latest node snapshots for
	// the given organisation.
	ListNodeSnapshotsByOrganisation(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error)

	// ListNodeSnapshotsByCollectionRun returns all node snapshots captured
	// during the given collection run.
	ListNodeSnapshotsByCollectionRun(ctx context.Context, collectionRunID string) ([]datastore.NodeSnapshot, error)

	// CountChefVersionsByCollectionRun returns a map of chef_version → count
	// for all node snapshots in the given collection run.
	CountChefVersionsByCollectionRun(ctx context.Context, collectionRunID string) (map[string]int, error)

	// CountChefVersionsByCollectionRunFiltered returns a map of chef_version → count
	// for node snapshots in the given collection run whose node_name is in allowedNodes.
	CountChefVersionsByCollectionRunFiltered(ctx context.Context, collectionRunID string, allowedNodes []string) (map[string]int, error)

	// CountStaleFreshByCollectionRun returns the total, stale, and fresh
	// node counts for the given collection run.
	CountStaleFreshByCollectionRun(ctx context.Context, collectionRunID string) (total, stale, fresh int, err error)

	// GetNodeSnapshotByName returns the most recent snapshot for a node
	// identified by organisation ID and node name. Returns
	// datastore.ErrNotFound if no such snapshot exists.
	GetNodeSnapshotByName(ctx context.Context, organisationID, nodeName string) (datastore.NodeSnapshot, error)

	// -----------------------------------------------------------------
	// Node readiness
	// -----------------------------------------------------------------

	// ListNodeReadinessForSnapshot returns all readiness records for the
	// given node snapshot, ordered by target_chef_version.
	ListNodeReadinessForSnapshot(ctx context.Context, nodeSnapshotID string) ([]datastore.NodeReadiness, error)

	// CountNodeReadiness returns the total, ready, and blocked counts for
	// the given organisation and target Chef version.
	CountNodeReadiness(ctx context.Context, organisationID, targetChefVersion string) (total, ready, blocked int, err error)

	// -----------------------------------------------------------------
	// Cookbooks
	// -----------------------------------------------------------------

	// ListCookbooksByOrganisation returns all cookbooks belonging to the
	// given organisation.
	ListCookbooksByOrganisation(ctx context.Context, organisationID string) ([]datastore.Cookbook, error)

	// ListCookbooksByName returns all cookbook versions with the given name
	// across all organisations and sources.
	ListCookbooksByName(ctx context.Context, name string) ([]datastore.Cookbook, error)

	// ListGitCookbooks returns all git-sourced cookbooks.
	ListGitCookbooks(ctx context.Context) ([]datastore.Cookbook, error)

	// -----------------------------------------------------------------
	// Cookbook analysis results
	// -----------------------------------------------------------------

	// ListCookbookComplexitiesForCookbook returns all complexity records for
	// the given cookbook ID, ordered by target_chef_version.
	ListCookbookComplexitiesForCookbook(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error)

	// ListCookbookComplexitiesForOrganisation returns all complexity records
	// for cookbooks belonging to the given organisation, ordered by cookbook
	// name, version, and target Chef version.
	ListCookbookComplexitiesForOrganisation(ctx context.Context, organisationID string) ([]datastore.CookbookComplexity, error)

	// ListCookstyleResultsForCookbook returns all cookstyle results for the
	// given cookbook ID, ordered by target_chef_version.
	ListCookstyleResultsForCookbook(ctx context.Context, cookbookID string) ([]datastore.CookstyleResult, error)

	// GetCookstyleResult returns the cookstyle result for the given cookbook
	// ID and target Chef version. Returns (nil, nil) if no result exists.
	GetCookstyleResult(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error)

	// ResetCookbookDownloadStatus resets the download_status to 'pending'
	// for a server cookbook, forcing the streaming pipeline to re-download
	// and re-scan it on the next collection cycle.
	ResetCookbookDownloadStatus(ctx context.Context, id string) (datastore.Cookbook, error)

	// ResetAllServerCookbookDownloadStatuses resets download_status to
	// 'pending' for all server cookbooks with status 'ok', forcing the
	// streaming pipeline to re-download and re-scan them all.
	ResetAllServerCookbookDownloadStatuses(ctx context.Context) (int, error)

	// DeleteCookstyleResultsForCookbook removes all cookstyle results for
	// the given cookbook ID. Forces a rescan on the next collection cycle.
	DeleteCookstyleResultsForCookbook(ctx context.Context, cookbookID string) error

	// DeleteCookbookComplexitiesForCookbook removes all complexity records
	// for the given cookbook ID. Forces recomputation on the next cycle.
	DeleteCookbookComplexitiesForCookbook(ctx context.Context, cookbookID string) error

	// DeleteAutocorrectPreviewsForCookbook removes all autocorrect previews
	// for the given cookbook ID. Forces regeneration on the next cycle.
	DeleteAutocorrectPreviewsForCookbook(ctx context.Context, cookbookID string) error

	// DeleteAllCookstyleResults removes all cookstyle results. Forces a
	// full rescan on the next collection cycle.
	DeleteAllCookstyleResults(ctx context.Context) error

	// DeleteAllCookbookComplexities removes all cookbook complexity records.
	DeleteAllCookbookComplexities(ctx context.Context) error

	// DeleteAllAutocorrectPreviews removes all autocorrect previews.
	DeleteAllAutocorrectPreviews(ctx context.Context) error

	// GetAutocorrectPreview returns the autocorrect preview for the given
	// cookstyle result ID. Returns (nil, nil) if no preview exists.
	GetAutocorrectPreview(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error)

	// GetLatestTestKitchenResult returns the most recent test kitchen result
	// for the given cookbook ID and target Chef version. Returns (nil, nil)
	// if no result exists.
	GetLatestTestKitchenResult(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.TestKitchenResult, error)

	// ListTestKitchenResultsForCookbook returns all test kitchen results for
	// the given cookbook ID, ordered by target_chef_version then started_at desc.
	ListTestKitchenResultsForCookbook(ctx context.Context, cookbookID string) ([]datastore.TestKitchenResult, error)

	// CountCookbookCompatibility returns aggregated compatibility counts
	// across the given organisations and target Chef versions in a single
	// query. If cookbookNames is non-nil only cookbooks whose name is in the
	// map are considered.
	CountCookbookCompatibility(ctx context.Context, organisationIDs []string, targetVersions []string, cookbookNames map[string]bool) ([]datastore.CookbookCompatibilitySummary, error)

	// -----------------------------------------------------------------
	// Log entries
	// -----------------------------------------------------------------

	// ListLogEntries returns log entries matching the given filter.
	ListLogEntries(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error)

	// CountLogEntries returns the number of log entries matching the given
	// filter.
	CountLogEntries(ctx context.Context, filter datastore.LogEntryFilter) (int, error)

	// GetLogEntry retrieves a single log entry by ID. Returns
	// datastore.ErrNotFound if no such entry exists.
	GetLogEntry(ctx context.Context, id string) (datastore.LogEntry, error)

	// -----------------------------------------------------------------
	// Role dependencies (used by dependency graph handlers)
	// -----------------------------------------------------------------

	// ListRoleDependenciesByOrg returns all dependency records for the given
	// organisation, ordered by role_name, dependency_type, dependency_name.
	ListRoleDependenciesByOrg(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error)

	// CountDependenciesByRole returns the number of cookbook and role
	// dependencies for each role in the given organisation, ordered by
	// total dependency count descending.
	CountDependenciesByRole(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error)

	// CountRolesPerCookbook returns the number of distinct roles that depend
	// on each cookbook within the given organisation.
	CountRolesPerCookbook(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error)

	// -----------------------------------------------------------------
	// Export jobs
	// -----------------------------------------------------------------

	// InsertExportJob creates a new export job in pending status and returns it.
	InsertExportJob(ctx context.Context, p datastore.InsertExportJobParams) (*datastore.ExportJob, error)

	// GetExportJob returns a single export job by its primary key UUID.
	// Returns datastore.ErrNotFound if no such job exists.
	GetExportJob(ctx context.Context, id string) (*datastore.ExportJob, error)

	// UpdateExportJobStatus updates a job's status and associated result fields.
	UpdateExportJobStatus(ctx context.Context, id, status string, rowCount int, filePath string, fileSizeBytes int64, errorMessage string) error

	// UpdateExportJobExpired marks a completed export job as expired.
	UpdateExportJobExpired(ctx context.Context, id string) error

	// ListExportJobsByStatus returns all export jobs with the given status,
	// ordered by requested_at descending.
	ListExportJobsByStatus(ctx context.Context, status string) ([]datastore.ExportJob, error)

	// ListExpiredExportJobs returns completed export jobs whose expires_at
	// is before the given time.
	ListExpiredExportJobs(ctx context.Context, now time.Time) ([]datastore.ExportJob, error)

	// -----------------------------------------------------------------
	// Owners
	// -----------------------------------------------------------------

	// ListOwners returns owners matching the given filter, ordered by name.
	ListOwners(ctx context.Context, f datastore.OwnerListFilter) ([]datastore.Owner, int, error)

	// GetOwnerByName returns the owner with the given name. Returns
	// datastore.ErrNotFound if no such owner exists.
	GetOwnerByName(ctx context.Context, name string) (datastore.Owner, error)

	// InsertOwner creates a new owner. Returns datastore.ErrAlreadyExists
	// if the name is taken.
	InsertOwner(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error)

	// UpdateOwner updates an existing owner by name. Returns
	// datastore.ErrNotFound if no such owner exists.
	UpdateOwner(ctx context.Context, name string, p datastore.UpdateOwnerParams) (datastore.Owner, error)

	// DeleteOwner removes an owner by name. Returns datastore.ErrNotFound
	// if no such owner exists. Returns the number of cascaded assignments.
	DeleteOwner(ctx context.Context, name string) (int, error)

	// CountAssignmentsByOwner returns the assignment count per entity type
	// for the given owner name.
	CountAssignmentsByOwner(ctx context.Context, ownerName string) (map[string]int, error)

	// ListOwnersWithSummary returns owners with pre-computed assignment
	// counts and readiness data in a single query. Pass targetChefVersion
	// as "" to skip readiness enrichment.
	ListOwnersWithSummary(ctx context.Context, f datastore.OwnerListFilter, targetChefVersion string) ([]datastore.OwnerWithSummary, int, error)

	// -----------------------------------------------------------------
	// Ownership assignments
	// -----------------------------------------------------------------

	// InsertAssignment creates a new ownership assignment. Returns
	// datastore.ErrAlreadyExists if a duplicate assignment exists.
	InsertAssignment(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error)

	// ListAssignmentsByOwner returns assignments for the given owner
	// matching the filter.
	ListAssignmentsByOwner(ctx context.Context, f datastore.AssignmentListFilter) ([]datastore.OwnershipAssignment, int, error)

	// GetAssignment returns a single assignment by ID. Returns
	// datastore.ErrNotFound if no such assignment exists.
	GetAssignment(ctx context.Context, id string) (datastore.OwnershipAssignment, error)

	// DeleteAssignment removes an assignment by ID. Returns
	// datastore.ErrNotFound if no such assignment exists.
	DeleteAssignment(ctx context.Context, id string) error

	// ReassignOwnership moves assignments from one owner to another.
	// Returns the number reassigned and the number skipped (duplicates).
	ReassignOwnership(ctx context.Context, fromOwnerID, toOwnerID string, entityType, organisationID string) (reassigned, skipped int, err error)

	// LookupOwnership returns the owners of a given entity, including
	// inherited ownership.
	LookupOwnership(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error)

	// -----------------------------------------------------------------
	// Owner detail summaries
	// -----------------------------------------------------------------

	// GetOwnerReadinessSummary computes migration readiness data for all
	// nodes assigned to the given owner for the specified target version.
	GetOwnerReadinessSummary(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerReadinessSummary, error)

	// GetOwnerCookbookSummary computes compatibility data for cookbooks
	// assigned to the given owner for the specified target version.
	GetOwnerCookbookSummary(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerCookbookSummary, error)

	// GetOwnerGitRepoSummary computes compatibility data for git repos
	// assigned to the given owner for the specified target version.
	GetOwnerGitRepoSummary(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerGitRepoSummary, error)

	// -----------------------------------------------------------------
	// Git repo committers
	// -----------------------------------------------------------------

	// GetGitRepoURLForCookbook looks up the git_repo_url for a git-sourced
	// cookbook by name. Returns datastore.ErrNotFound if no git-sourced
	// cookbook exists with that name.
	GetGitRepoURLForCookbook(ctx context.Context, cookbookName string) (string, error)

	// ListCommittersByRepo returns committers for the given git repo URL,
	// with sorting, pagination, and an optional since filter. Returns the
	// matching rows and the total count for pagination.
	ListCommittersByRepo(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error)

	// DeleteGitCookbooksByName removes all git-sourced cookbook rows for the
	// given cookbook name and deletes associated committer data. Returns
	// datastore.ErrNotFound if no git-sourced cookbook exists with that name.
	DeleteGitCookbooksByName(ctx context.Context, cookbookName string) (datastore.DeleteGitCookbookResult, error)

	// -----------------------------------------------------------------
	// Ownership audit log
	// -----------------------------------------------------------------

	// InsertAuditEntry creates a new ownership audit log entry.
	InsertAuditEntry(ctx context.Context, p datastore.InsertAuditEntryParams) error

	// ListAuditLog returns audit log entries matching the given filter,
	// in reverse chronological order.
	ListAuditLog(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error)
}

// Compile-time assertion: *datastore.DB satisfies DataStore.
var _ DataStore = (*datastore.DB)(nil)
