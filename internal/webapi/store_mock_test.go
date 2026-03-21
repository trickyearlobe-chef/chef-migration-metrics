// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// mockStore implements DataStore for handler tests. Each method delegates to
// a function field. If the field is nil the method returns zero values (and
// nil error) so tests only need to set the stubs they care about.
type mockStore struct {
	PingFn                                              func(ctx context.Context) error
	ListOrganisationsFn                                 func(ctx context.Context) ([]datastore.Organisation, error)
	GetOrganisationByNameFn                             func(ctx context.Context, name string) (datastore.Organisation, error)
	GetLatestCollectionRunFn                            func(ctx context.Context, organisationID string) (datastore.CollectionRun, error)
	ListCollectionRunsFn                                func(ctx context.Context, organisationID string, limit int) ([]datastore.CollectionRun, error)
	ListNodeSnapshotsByOrganisationFn                   func(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error)
	ListNodeSnapshotsByCollectionRunFn                  func(ctx context.Context, collectionRunID string) ([]datastore.NodeSnapshot, error)
	CountChefVersionsByCollectionRunFn                  func(ctx context.Context, collectionRunID string) (map[string]int, error)
	CountChefVersionsByCollectionRunFilteredFn          func(ctx context.Context, collectionRunID string, allowedNodes []string) (map[string]int, error)
	CountStaleFreshByCollectionRunFn                    func(ctx context.Context, collectionRunID string) (int, int, int, error)
	ListMetricSnapshotsByOrganisationFn                 func(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error)
	GetNodeSnapshotByNameFn                             func(ctx context.Context, organisationID, nodeName string) (datastore.NodeSnapshot, error)
	ListNodeReadinessForSnapshotFn                      func(ctx context.Context, nodeSnapshotID string) ([]datastore.NodeReadiness, error)
	CountNodeReadinessFn                                func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error)
	ListServerCookbooksByOrganisationFn                 func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error)
	ListServerCookbooksByNameFn                         func(ctx context.Context, name string) ([]datastore.ServerCookbook, error)
	ResetServerCookbookDownloadStatusFn                 func(ctx context.Context, id string) (datastore.ServerCookbook, error)
	ResetAllServerCookbookDownloadStatusesFn            func(ctx context.Context) (int, error)
	GetGitRepoFn                                        func(ctx context.Context, id string) (datastore.GitRepo, error)
	ListGitReposFn                                      func(ctx context.Context) ([]datastore.GitRepo, error)
	ListGitReposByNameFn                                func(ctx context.Context, name string) ([]datastore.GitRepo, error)
	DeleteGitReposByNameFn                              func(ctx context.Context, name string) (datastore.DeleteGitRepoResult, error)
	ListServerCookbookComplexitiesByCookbookFn          func(ctx context.Context, serverCookbookID string) ([]datastore.ServerCookbookComplexity, error)
	ListServerCookbookComplexitiesByOrganisationFn      func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error)
	ListServerCookbookCookstyleResultsFn                func(ctx context.Context, serverCookbookID string) ([]datastore.ServerCookbookCookstyleResult, error)
	GetServerCookbookCookstyleResultFn                  func(ctx context.Context, serverCookbookID, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error)
	GetServerCookbookAutocorrectPreviewFn               func(ctx context.Context, cookstyleResultID string) (*datastore.ServerCookbookAutocorrectPreview, error)
	DeleteServerCookbookCookstyleResultsByCookbookFn    func(ctx context.Context, serverCookbookID string) error
	DeleteServerCookbookComplexitiesByCookbookFn        func(ctx context.Context, serverCookbookID string) error
	DeleteServerCookbookAutocorrectPreviewsByCookbookFn func(ctx context.Context, serverCookbookID string) error
	DeleteAllServerCookbookCookstyleResultsFn           func(ctx context.Context) error
	DeleteAllServerCookbookComplexitiesFn               func(ctx context.Context) error
	DeleteAllServerCookbookAutocorrectPreviewsFn        func(ctx context.Context) error
	ListGitRepoCookstyleResultsFn                       func(ctx context.Context, gitRepoID string) ([]datastore.GitRepoCookstyleResult, error)
	GetGitRepoCookstyleResultFn                         func(ctx context.Context, gitRepoID, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error)
	ListGitRepoComplexitiesByRepoFn                     func(ctx context.Context, gitRepoID string) ([]datastore.GitRepoComplexity, error)
	ListAllGitRepoComplexitiesFn                        func(ctx context.Context) ([]datastore.GitRepoComplexity, error)
	GetGitRepoAutocorrectPreviewFn                      func(ctx context.Context, cookstyleResultID string) (*datastore.GitRepoAutocorrectPreview, error)
	GetLatestGitRepoTestKitchenResultFn                 func(ctx context.Context, gitRepoID, targetChefVersion string) (*datastore.GitRepoTestKitchenResult, error)
	ListGitRepoTestKitchenResultsFn                     func(ctx context.Context, gitRepoID string) ([]datastore.GitRepoTestKitchenResult, error)
	DeleteGitRepoCookstyleResultsByRepoFn               func(ctx context.Context, gitRepoID string) error
	DeleteGitRepoComplexitiesByRepoFn                   func(ctx context.Context, gitRepoID string) error
	DeleteGitRepoAutocorrectPreviewsByRepoFn            func(ctx context.Context, gitRepoID string) error
	DeleteAllGitRepoCookstyleResultsFn                  func(ctx context.Context) error
	DeleteAllGitRepoComplexitiesFn                      func(ctx context.Context) error
	DeleteAllGitRepoAutocorrectPreviewsFn               func(ctx context.Context) error
	ListLogEntriesFn                                    func(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error)
	CountLogEntriesFn                                   func(ctx context.Context, filter datastore.LogEntryFilter) (int, error)
	GetLogEntryFn                                       func(ctx context.Context, id string) (datastore.LogEntry, error)
	ListRoleDependenciesByOrgFn                         func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error)
	CountDependenciesByRoleFn                           func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error)
	CountRolesPerCookbookFn                             func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error)
	InsertExportJobFn                                   func(ctx context.Context, p datastore.InsertExportJobParams) (*datastore.ExportJob, error)
	GetExportJobFn                                      func(ctx context.Context, id string) (*datastore.ExportJob, error)
	UpdateExportJobStatusFn                             func(ctx context.Context, id, status string, rowCount int, filePath string, fileSizeBytes int64, errorMessage string) error
	UpdateExportJobExpiredFn                            func(ctx context.Context, id string) error
	ListExportJobsByStatusFn                            func(ctx context.Context, status string) ([]datastore.ExportJob, error)
	ListExpiredExportJobsFn                             func(ctx context.Context, now time.Time) ([]datastore.ExportJob, error)
	ListOwnersFn                                        func(ctx context.Context, f datastore.OwnerListFilter) ([]datastore.Owner, int, error)
	ListOwnersWithSummaryFn                             func(ctx context.Context, f datastore.OwnerListFilter, targetChefVersion string) ([]datastore.OwnerWithSummary, int, error)
	GetOwnerByNameFn                                    func(ctx context.Context, name string) (datastore.Owner, error)
	InsertOwnerFn                                       func(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error)
	UpdateOwnerFn                                       func(ctx context.Context, name string, p datastore.UpdateOwnerParams) (datastore.Owner, error)
	DeleteOwnerFn                                       func(ctx context.Context, name string) (int, error)
	CountAssignmentsByOwnerFn                           func(ctx context.Context, ownerName string) (map[string]int, error)
	InsertAssignmentFn                                  func(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error)
	ListAssignmentsByOwnerFn                            func(ctx context.Context, f datastore.AssignmentListFilter) ([]datastore.OwnershipAssignment, int, error)
	GetAssignmentFn                                     func(ctx context.Context, id string) (datastore.OwnershipAssignment, error)
	DeleteAssignmentFn                                  func(ctx context.Context, id string) error
	ReassignOwnershipFn                                 func(ctx context.Context, fromOwnerID, toOwnerID string, entityType, organisationID string) (int, int, error)
	LookupOwnershipFn                                   func(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error)
	GetOwnerReadinessSummaryFn                          func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerReadinessSummary, error)
	GetOwnerCookbookSummaryFn                           func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerCookbookSummary, error)
	GetOwnerGitRepoSummaryFn                            func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerGitRepoSummary, error)
	GetGitRepoURLForCookbookFn                          func(ctx context.Context, cookbookName string) (string, error)
	ListCommittersByRepoFn                              func(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error)
	GetOwnerEmailsForGitRepoFn                          func(ctx context.Context, gitRepoURL string) (map[string]bool, error)
	InsertAuditEntryFn                                  func(ctx context.Context, p datastore.InsertAuditEntryParams) error
	ListAuditLogFn                                      func(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error)
}

// compile-time check
var _ DataStore = (*mockStore)(nil)

func (m *mockStore) Ping(ctx context.Context) error {
	if m.PingFn != nil {
		return m.PingFn(ctx)
	}
	return nil
}

func (m *mockStore) ListOrganisations(ctx context.Context) ([]datastore.Organisation, error) {
	if m.ListOrganisationsFn != nil {
		return m.ListOrganisationsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) GetOrganisationByName(ctx context.Context, name string) (datastore.Organisation, error) {
	if m.GetOrganisationByNameFn != nil {
		return m.GetOrganisationByNameFn(ctx, name)
	}
	return datastore.Organisation{}, nil
}

func (m *mockStore) GetLatestCollectionRun(ctx context.Context, organisationID string) (datastore.CollectionRun, error) {
	if m.GetLatestCollectionRunFn != nil {
		return m.GetLatestCollectionRunFn(ctx, organisationID)
	}
	return datastore.CollectionRun{}, nil
}

func (m *mockStore) ListCollectionRuns(ctx context.Context, organisationID string, limit int) ([]datastore.CollectionRun, error) {
	if m.ListCollectionRunsFn != nil {
		return m.ListCollectionRunsFn(ctx, organisationID, limit)
	}
	return nil, nil
}

func (m *mockStore) ListNodeSnapshotsByOrganisation(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error) {
	if m.ListNodeSnapshotsByOrganisationFn != nil {
		return m.ListNodeSnapshotsByOrganisationFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) ListNodeSnapshotsByCollectionRun(ctx context.Context, collectionRunID string) ([]datastore.NodeSnapshot, error) {
	if m.ListNodeSnapshotsByCollectionRunFn != nil {
		return m.ListNodeSnapshotsByCollectionRunFn(ctx, collectionRunID)
	}
	return nil, nil
}

func (m *mockStore) CountChefVersionsByCollectionRun(ctx context.Context, collectionRunID string) (map[string]int, error) {
	if m.CountChefVersionsByCollectionRunFn != nil {
		return m.CountChefVersionsByCollectionRunFn(ctx, collectionRunID)
	}
	return nil, nil
}

func (m *mockStore) CountChefVersionsByCollectionRunFiltered(ctx context.Context, collectionRunID string, allowedNodes []string) (map[string]int, error) {
	if m.CountChefVersionsByCollectionRunFilteredFn != nil {
		return m.CountChefVersionsByCollectionRunFilteredFn(ctx, collectionRunID, allowedNodes)
	}
	return nil, nil
}

func (m *mockStore) CountStaleFreshByCollectionRun(ctx context.Context, collectionRunID string) (total, stale, fresh int, err error) {
	if m.CountStaleFreshByCollectionRunFn != nil {
		return m.CountStaleFreshByCollectionRunFn(ctx, collectionRunID)
	}
	return 0, 0, 0, nil
}

func (m *mockStore) ListMetricSnapshotsByOrganisation(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error) {
	if m.ListMetricSnapshotsByOrganisationFn != nil {
		return m.ListMetricSnapshotsByOrganisationFn(ctx, organisationID, snapshotType, limit)
	}
	return nil, nil
}

func (m *mockStore) GetNodeSnapshotByName(ctx context.Context, organisationID, nodeName string) (datastore.NodeSnapshot, error) {
	if m.GetNodeSnapshotByNameFn != nil {
		return m.GetNodeSnapshotByNameFn(ctx, organisationID, nodeName)
	}
	return datastore.NodeSnapshot{}, nil
}

func (m *mockStore) ListNodeReadinessForSnapshot(ctx context.Context, nodeSnapshotID string) ([]datastore.NodeReadiness, error) {
	if m.ListNodeReadinessForSnapshotFn != nil {
		return m.ListNodeReadinessForSnapshotFn(ctx, nodeSnapshotID)
	}
	return nil, nil
}

func (m *mockStore) CountNodeReadiness(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error) {
	if m.CountNodeReadinessFn != nil {
		return m.CountNodeReadinessFn(ctx, organisationID, targetChefVersion)
	}
	return 0, 0, 0, nil
}

// -----------------------------------------------------------------
// Server cookbooks
// -----------------------------------------------------------------

func (m *mockStore) ListServerCookbooksByOrganisation(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error) {
	if m.ListServerCookbooksByOrganisationFn != nil {
		return m.ListServerCookbooksByOrganisationFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) ListServerCookbooksByName(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
	if m.ListServerCookbooksByNameFn != nil {
		return m.ListServerCookbooksByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockStore) ResetServerCookbookDownloadStatus(ctx context.Context, id string) (datastore.ServerCookbook, error) {
	if m.ResetServerCookbookDownloadStatusFn != nil {
		return m.ResetServerCookbookDownloadStatusFn(ctx, id)
	}
	return datastore.ServerCookbook{}, nil
}

func (m *mockStore) ResetAllServerCookbookDownloadStatuses(ctx context.Context) (int, error) {
	if m.ResetAllServerCookbookDownloadStatusesFn != nil {
		return m.ResetAllServerCookbookDownloadStatusesFn(ctx)
	}
	return 0, nil
}

// -----------------------------------------------------------------
// Git repos
// -----------------------------------------------------------------

func (m *mockStore) GetGitRepo(ctx context.Context, id string) (datastore.GitRepo, error) {
	if m.GetGitRepoFn != nil {
		return m.GetGitRepoFn(ctx, id)
	}
	return datastore.GitRepo{}, nil
}

func (m *mockStore) ListGitRepos(ctx context.Context) ([]datastore.GitRepo, error) {
	if m.ListGitReposFn != nil {
		return m.ListGitReposFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListGitReposByName(ctx context.Context, name string) ([]datastore.GitRepo, error) {
	if m.ListGitReposByNameFn != nil {
		return m.ListGitReposByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockStore) DeleteGitReposByName(ctx context.Context, name string) (datastore.DeleteGitRepoResult, error) {
	if m.DeleteGitReposByNameFn != nil {
		return m.DeleteGitReposByNameFn(ctx, name)
	}
	return datastore.DeleteGitRepoResult{}, datastore.ErrNotFound
}

// -----------------------------------------------------------------
// Server cookbook analysis results
// -----------------------------------------------------------------

func (m *mockStore) ListServerCookbookComplexitiesByCookbook(ctx context.Context, serverCookbookID string) ([]datastore.ServerCookbookComplexity, error) {
	if m.ListServerCookbookComplexitiesByCookbookFn != nil {
		return m.ListServerCookbookComplexitiesByCookbookFn(ctx, serverCookbookID)
	}
	return nil, nil
}

func (m *mockStore) ListServerCookbookComplexitiesByOrganisation(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
	if m.ListServerCookbookComplexitiesByOrganisationFn != nil {
		return m.ListServerCookbookComplexitiesByOrganisationFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) ListServerCookbookCookstyleResults(ctx context.Context, serverCookbookID string) ([]datastore.ServerCookbookCookstyleResult, error) {
	if m.ListServerCookbookCookstyleResultsFn != nil {
		return m.ListServerCookbookCookstyleResultsFn(ctx, serverCookbookID)
	}
	return nil, nil
}

func (m *mockStore) GetServerCookbookCookstyleResult(ctx context.Context, serverCookbookID, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error) {
	if m.GetServerCookbookCookstyleResultFn != nil {
		return m.GetServerCookbookCookstyleResultFn(ctx, serverCookbookID, targetChefVersion)
	}
	return nil, nil
}

func (m *mockStore) GetServerCookbookAutocorrectPreview(ctx context.Context, cookstyleResultID string) (*datastore.ServerCookbookAutocorrectPreview, error) {
	if m.GetServerCookbookAutocorrectPreviewFn != nil {
		return m.GetServerCookbookAutocorrectPreviewFn(ctx, cookstyleResultID)
	}
	return nil, nil
}

func (m *mockStore) DeleteServerCookbookCookstyleResultsByCookbook(ctx context.Context, serverCookbookID string) error {
	if m.DeleteServerCookbookCookstyleResultsByCookbookFn != nil {
		return m.DeleteServerCookbookCookstyleResultsByCookbookFn(ctx, serverCookbookID)
	}
	return nil
}

func (m *mockStore) DeleteServerCookbookComplexitiesByCookbook(ctx context.Context, serverCookbookID string) error {
	if m.DeleteServerCookbookComplexitiesByCookbookFn != nil {
		return m.DeleteServerCookbookComplexitiesByCookbookFn(ctx, serverCookbookID)
	}
	return nil
}

func (m *mockStore) DeleteServerCookbookAutocorrectPreviewsByCookbook(ctx context.Context, serverCookbookID string) error {
	if m.DeleteServerCookbookAutocorrectPreviewsByCookbookFn != nil {
		return m.DeleteServerCookbookAutocorrectPreviewsByCookbookFn(ctx, serverCookbookID)
	}
	return nil
}

func (m *mockStore) DeleteAllServerCookbookCookstyleResults(ctx context.Context) error {
	if m.DeleteAllServerCookbookCookstyleResultsFn != nil {
		return m.DeleteAllServerCookbookCookstyleResultsFn(ctx)
	}
	return nil
}

func (m *mockStore) DeleteAllServerCookbookComplexities(ctx context.Context) error {
	if m.DeleteAllServerCookbookComplexitiesFn != nil {
		return m.DeleteAllServerCookbookComplexitiesFn(ctx)
	}
	return nil
}

func (m *mockStore) DeleteAllServerCookbookAutocorrectPreviews(ctx context.Context) error {
	if m.DeleteAllServerCookbookAutocorrectPreviewsFn != nil {
		return m.DeleteAllServerCookbookAutocorrectPreviewsFn(ctx)
	}
	return nil
}

// -----------------------------------------------------------------
// Git repo analysis results
// -----------------------------------------------------------------

func (m *mockStore) ListGitRepoCookstyleResults(ctx context.Context, gitRepoID string) ([]datastore.GitRepoCookstyleResult, error) {
	if m.ListGitRepoCookstyleResultsFn != nil {
		return m.ListGitRepoCookstyleResultsFn(ctx, gitRepoID)
	}
	return nil, nil
}

func (m *mockStore) GetGitRepoCookstyleResult(ctx context.Context, gitRepoID, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error) {
	if m.GetGitRepoCookstyleResultFn != nil {
		return m.GetGitRepoCookstyleResultFn(ctx, gitRepoID, targetChefVersion)
	}
	return nil, nil
}

func (m *mockStore) ListGitRepoComplexitiesByRepo(ctx context.Context, gitRepoID string) ([]datastore.GitRepoComplexity, error) {
	if m.ListGitRepoComplexitiesByRepoFn != nil {
		return m.ListGitRepoComplexitiesByRepoFn(ctx, gitRepoID)
	}
	return nil, nil
}

func (m *mockStore) ListAllGitRepoComplexities(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
	if m.ListAllGitRepoComplexitiesFn != nil {
		return m.ListAllGitRepoComplexitiesFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) GetGitRepoAutocorrectPreview(ctx context.Context, cookstyleResultID string) (*datastore.GitRepoAutocorrectPreview, error) {
	if m.GetGitRepoAutocorrectPreviewFn != nil {
		return m.GetGitRepoAutocorrectPreviewFn(ctx, cookstyleResultID)
	}
	return nil, nil
}

func (m *mockStore) GetLatestGitRepoTestKitchenResult(ctx context.Context, gitRepoID, targetChefVersion string) (*datastore.GitRepoTestKitchenResult, error) {
	if m.GetLatestGitRepoTestKitchenResultFn != nil {
		return m.GetLatestGitRepoTestKitchenResultFn(ctx, gitRepoID, targetChefVersion)
	}
	return nil, nil
}

func (m *mockStore) ListGitRepoTestKitchenResults(ctx context.Context, gitRepoID string) ([]datastore.GitRepoTestKitchenResult, error) {
	if m.ListGitRepoTestKitchenResultsFn != nil {
		return m.ListGitRepoTestKitchenResultsFn(ctx, gitRepoID)
	}
	return nil, nil
}

func (m *mockStore) DeleteGitRepoCookstyleResultsByRepo(ctx context.Context, gitRepoID string) error {
	if m.DeleteGitRepoCookstyleResultsByRepoFn != nil {
		return m.DeleteGitRepoCookstyleResultsByRepoFn(ctx, gitRepoID)
	}
	return nil
}

func (m *mockStore) DeleteGitRepoComplexitiesByRepo(ctx context.Context, gitRepoID string) error {
	if m.DeleteGitRepoComplexitiesByRepoFn != nil {
		return m.DeleteGitRepoComplexitiesByRepoFn(ctx, gitRepoID)
	}
	return nil
}

func (m *mockStore) DeleteGitRepoAutocorrectPreviewsByRepo(ctx context.Context, gitRepoID string) error {
	if m.DeleteGitRepoAutocorrectPreviewsByRepoFn != nil {
		return m.DeleteGitRepoAutocorrectPreviewsByRepoFn(ctx, gitRepoID)
	}
	return nil
}

func (m *mockStore) DeleteAllGitRepoCookstyleResults(ctx context.Context) error {
	if m.DeleteAllGitRepoCookstyleResultsFn != nil {
		return m.DeleteAllGitRepoCookstyleResultsFn(ctx)
	}
	return nil
}

func (m *mockStore) DeleteAllGitRepoComplexities(ctx context.Context) error {
	if m.DeleteAllGitRepoComplexitiesFn != nil {
		return m.DeleteAllGitRepoComplexitiesFn(ctx)
	}
	return nil
}

func (m *mockStore) DeleteAllGitRepoAutocorrectPreviews(ctx context.Context) error {
	if m.DeleteAllGitRepoAutocorrectPreviewsFn != nil {
		return m.DeleteAllGitRepoAutocorrectPreviewsFn(ctx)
	}
	return nil
}

// -----------------------------------------------------------------
// Log entries
// -----------------------------------------------------------------

func (m *mockStore) ListLogEntries(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
	if m.ListLogEntriesFn != nil {
		return m.ListLogEntriesFn(ctx, filter)
	}
	return nil, nil
}

func (m *mockStore) CountLogEntries(ctx context.Context, filter datastore.LogEntryFilter) (int, error) {
	if m.CountLogEntriesFn != nil {
		return m.CountLogEntriesFn(ctx, filter)
	}
	return 0, nil
}

func (m *mockStore) GetLogEntry(ctx context.Context, id string) (datastore.LogEntry, error) {
	if m.GetLogEntryFn != nil {
		return m.GetLogEntryFn(ctx, id)
	}
	return datastore.LogEntry{}, nil
}

// -----------------------------------------------------------------
// Role dependencies
// -----------------------------------------------------------------

func (m *mockStore) ListRoleDependenciesByOrg(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error) {
	if m.ListRoleDependenciesByOrgFn != nil {
		return m.ListRoleDependenciesByOrgFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) CountDependenciesByRole(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error) {
	if m.CountDependenciesByRoleFn != nil {
		return m.CountDependenciesByRoleFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) CountRolesPerCookbook(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error) {
	if m.CountRolesPerCookbookFn != nil {
		return m.CountRolesPerCookbookFn(ctx, organisationID)
	}
	return nil, nil
}

// -----------------------------------------------------------------
// Export jobs
// -----------------------------------------------------------------

func (m *mockStore) InsertExportJob(ctx context.Context, p datastore.InsertExportJobParams) (*datastore.ExportJob, error) {
	if m.InsertExportJobFn != nil {
		return m.InsertExportJobFn(ctx, p)
	}
	return nil, nil
}

func (m *mockStore) GetExportJob(ctx context.Context, id string) (*datastore.ExportJob, error) {
	if m.GetExportJobFn != nil {
		return m.GetExportJobFn(ctx, id)
	}
	return nil, nil
}

func (m *mockStore) UpdateExportJobStatus(ctx context.Context, id, status string, rowCount int, filePath string, fileSizeBytes int64, errorMessage string) error {
	if m.UpdateExportJobStatusFn != nil {
		return m.UpdateExportJobStatusFn(ctx, id, status, rowCount, filePath, fileSizeBytes, errorMessage)
	}
	return nil
}

func (m *mockStore) UpdateExportJobExpired(ctx context.Context, id string) error {
	if m.UpdateExportJobExpiredFn != nil {
		return m.UpdateExportJobExpiredFn(ctx, id)
	}
	return nil
}

func (m *mockStore) ListExportJobsByStatus(ctx context.Context, status string) ([]datastore.ExportJob, error) {
	if m.ListExportJobsByStatusFn != nil {
		return m.ListExportJobsByStatusFn(ctx, status)
	}
	return nil, nil
}

func (m *mockStore) ListExpiredExportJobs(ctx context.Context, now time.Time) ([]datastore.ExportJob, error) {
	if m.ListExpiredExportJobsFn != nil {
		return m.ListExpiredExportJobsFn(ctx, now)
	}
	return nil, nil
}

// -----------------------------------------------------------------
// Owners
// -----------------------------------------------------------------

func (m *mockStore) ListOwners(ctx context.Context, f datastore.OwnerListFilter) ([]datastore.Owner, int, error) {
	if m.ListOwnersFn != nil {
		return m.ListOwnersFn(ctx, f)
	}
	return nil, 0, nil
}

func (m *mockStore) ListOwnersWithSummary(ctx context.Context, f datastore.OwnerListFilter, targetChefVersion string) ([]datastore.OwnerWithSummary, int, error) {
	if m.ListOwnersWithSummaryFn != nil {
		return m.ListOwnersWithSummaryFn(ctx, f, targetChefVersion)
	}
	return nil, 0, nil
}

func (m *mockStore) GetOwnerByName(ctx context.Context, name string) (datastore.Owner, error) {
	if m.GetOwnerByNameFn != nil {
		return m.GetOwnerByNameFn(ctx, name)
	}
	return datastore.Owner{}, nil
}

func (m *mockStore) InsertOwner(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
	if m.InsertOwnerFn != nil {
		return m.InsertOwnerFn(ctx, p)
	}
	return datastore.Owner{}, nil
}

func (m *mockStore) UpdateOwner(ctx context.Context, name string, p datastore.UpdateOwnerParams) (datastore.Owner, error) {
	if m.UpdateOwnerFn != nil {
		return m.UpdateOwnerFn(ctx, name, p)
	}
	return datastore.Owner{}, nil
}

func (m *mockStore) DeleteOwner(ctx context.Context, name string) (int, error) {
	if m.DeleteOwnerFn != nil {
		return m.DeleteOwnerFn(ctx, name)
	}
	return 0, nil
}

func (m *mockStore) CountAssignmentsByOwner(ctx context.Context, ownerName string) (map[string]int, error) {
	if m.CountAssignmentsByOwnerFn != nil {
		return m.CountAssignmentsByOwnerFn(ctx, ownerName)
	}
	return nil, nil
}

// -----------------------------------------------------------------
// Ownership assignments
// -----------------------------------------------------------------

func (m *mockStore) InsertAssignment(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
	if m.InsertAssignmentFn != nil {
		return m.InsertAssignmentFn(ctx, p)
	}
	return datastore.OwnershipAssignment{}, nil
}

func (m *mockStore) ListAssignmentsByOwner(ctx context.Context, f datastore.AssignmentListFilter) ([]datastore.OwnershipAssignment, int, error) {
	if m.ListAssignmentsByOwnerFn != nil {
		return m.ListAssignmentsByOwnerFn(ctx, f)
	}
	return nil, 0, nil
}

func (m *mockStore) GetAssignment(ctx context.Context, id string) (datastore.OwnershipAssignment, error) {
	if m.GetAssignmentFn != nil {
		return m.GetAssignmentFn(ctx, id)
	}
	return datastore.OwnershipAssignment{}, nil
}

func (m *mockStore) DeleteAssignment(ctx context.Context, id string) error {
	if m.DeleteAssignmentFn != nil {
		return m.DeleteAssignmentFn(ctx, id)
	}
	return nil
}

func (m *mockStore) ReassignOwnership(ctx context.Context, fromOwnerID, toOwnerID string, entityType, organisationID string) (int, int, error) {
	if m.ReassignOwnershipFn != nil {
		return m.ReassignOwnershipFn(ctx, fromOwnerID, toOwnerID, entityType, organisationID)
	}
	return 0, 0, nil
}

func (m *mockStore) LookupOwnership(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error) {
	if m.LookupOwnershipFn != nil {
		return m.LookupOwnershipFn(ctx, entityType, entityKey, organisationID)
	}
	return nil, nil
}

// -----------------------------------------------------------------
// Owner detail summaries
// -----------------------------------------------------------------

func (m *mockStore) GetOwnerReadinessSummary(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerReadinessSummary, error) {
	if m.GetOwnerReadinessSummaryFn != nil {
		return m.GetOwnerReadinessSummaryFn(ctx, ownerName, targetChefVersion)
	}
	return datastore.OwnerReadinessSummary{BlockingCookbooks: []datastore.BlockingCookbookSummary{}}, nil
}

func (m *mockStore) GetOwnerCookbookSummary(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerCookbookSummary, error) {
	if m.GetOwnerCookbookSummaryFn != nil {
		return m.GetOwnerCookbookSummaryFn(ctx, ownerName, targetChefVersion)
	}
	return datastore.OwnerCookbookSummary{}, nil
}

func (m *mockStore) GetOwnerGitRepoSummary(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerGitRepoSummary, error) {
	if m.GetOwnerGitRepoSummaryFn != nil {
		return m.GetOwnerGitRepoSummaryFn(ctx, ownerName, targetChefVersion)
	}
	return datastore.OwnerGitRepoSummary{}, nil
}

// -----------------------------------------------------------------
// Git repo committers
// -----------------------------------------------------------------

func (m *mockStore) GetGitRepoURLForCookbook(ctx context.Context, cookbookName string) (string, error) {
	if m.GetGitRepoURLForCookbookFn != nil {
		return m.GetGitRepoURLForCookbookFn(ctx, cookbookName)
	}
	return "", datastore.ErrNotFound
}

func (m *mockStore) ListCommittersByRepo(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error) {
	if m.ListCommittersByRepoFn != nil {
		return m.ListCommittersByRepoFn(ctx, f)
	}
	return nil, 0, nil
}

func (m *mockStore) GetOwnerEmailsForGitRepo(ctx context.Context, gitRepoURL string) (map[string]bool, error) {
	if m.GetOwnerEmailsForGitRepoFn != nil {
		return m.GetOwnerEmailsForGitRepoFn(ctx, gitRepoURL)
	}
	return nil, nil
}

// -----------------------------------------------------------------
// Ownership audit log
// -----------------------------------------------------------------

func (m *mockStore) InsertAuditEntry(ctx context.Context, p datastore.InsertAuditEntryParams) error {
	if m.InsertAuditEntryFn != nil {
		return m.InsertAuditEntryFn(ctx, p)
	}
	return nil
}

func (m *mockStore) ListAuditLog(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error) {
	if m.ListAuditLogFn != nil {
		return m.ListAuditLogFn(ctx, f)
	}
	return nil, 0, nil
}

// ---------------------------------------------------------------------------
// Test-helper constructors
// ---------------------------------------------------------------------------

// newTestRouterWithMock builds a Router backed by the given mockStore and a
// default config. The EventHub is started automatically. Use this for tests
// that exercise database-calling handler paths.
func newTestRouterWithMock(store *mockStore) *Router {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}

// newTestRouterWithMockAndConfig is like newTestRouterWithMock but accepts a
// custom *config.Config (e.g. to set TargetChefVersions).
func newTestRouterWithMockAndConfig(store *mockStore, cfg *config.Config) *Router {
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}
