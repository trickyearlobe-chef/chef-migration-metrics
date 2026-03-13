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
	PingFn                                    func(ctx context.Context) error
	ListOrganisationsFn                       func(ctx context.Context) ([]datastore.Organisation, error)
	GetOrganisationByNameFn                   func(ctx context.Context, name string) (datastore.Organisation, error)
	GetLatestCollectionRunFn                  func(ctx context.Context, organisationID string) (datastore.CollectionRun, error)
	ListCollectionRunsFn                      func(ctx context.Context, organisationID string, limit int) ([]datastore.CollectionRun, error)
	ListNodeSnapshotsByOrganisationFn         func(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error)
	ListNodeSnapshotsByCollectionRunFn        func(ctx context.Context, collectionRunID string) ([]datastore.NodeSnapshot, error)
	GetNodeSnapshotByNameFn                   func(ctx context.Context, organisationID, nodeName string) (datastore.NodeSnapshot, error)
	ListNodeReadinessForSnapshotFn            func(ctx context.Context, nodeSnapshotID string) ([]datastore.NodeReadiness, error)
	CountNodeReadinessFn                      func(ctx context.Context, organisationID, targetChefVersion string) (int, int, int, error)
	ListCookbooksByOrganisationFn             func(ctx context.Context, organisationID string) ([]datastore.Cookbook, error)
	ListCookbooksByNameFn                     func(ctx context.Context, name string) ([]datastore.Cookbook, error)
	ListGitCookbooksFn                        func(ctx context.Context) ([]datastore.Cookbook, error)
	ListCookbookComplexitiesForCookbookFn     func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error)
	ListCookbookComplexitiesForOrganisationFn func(ctx context.Context, organisationID string) ([]datastore.CookbookComplexity, error)
	ListCookstyleResultsForCookbookFn         func(ctx context.Context, cookbookID string) ([]datastore.CookstyleResult, error)
	GetCookstyleResultFn                      func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error)
	DeleteCookstyleResultsForCookbookFn       func(ctx context.Context, cookbookID string) error
	DeleteCookbookComplexitiesForCookbookFn   func(ctx context.Context, cookbookID string) error
	DeleteAutocorrectPreviewsForCookbookFn    func(ctx context.Context, cookbookID string) error
	GetAutocorrectPreviewFn                   func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error)
	ListAutocorrectPreviewsForCookbookFn      func(ctx context.Context, cookbookID string) ([]datastore.AutocorrectPreview, error)
	GetLatestTestKitchenResultFn              func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.TestKitchenResult, error)
	ListLogEntriesFn                          func(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error)
	CountLogEntriesFn                         func(ctx context.Context, filter datastore.LogEntryFilter) (int, error)
	GetLogEntryFn                             func(ctx context.Context, id string) (datastore.LogEntry, error)
	ListRoleDependenciesByOrgFn               func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error)
	CountDependenciesByRoleFn                 func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error)
	CountRolesPerCookbookFn                   func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error)
	InsertExportJobFn                         func(ctx context.Context, p datastore.InsertExportJobParams) (*datastore.ExportJob, error)
	GetExportJobFn                            func(ctx context.Context, id string) (*datastore.ExportJob, error)
	UpdateExportJobStatusFn                   func(ctx context.Context, id, status string, rowCount int, filePath string, fileSizeBytes int64, errorMessage string) error
	UpdateExportJobExpiredFn                  func(ctx context.Context, id string) error
	ListExportJobsByStatusFn                  func(ctx context.Context, status string) ([]datastore.ExportJob, error)
	ListExpiredExportJobsFn                   func(ctx context.Context, now time.Time) ([]datastore.ExportJob, error)
	ListOwnersFn                              func(ctx context.Context, f datastore.OwnerListFilter) ([]datastore.Owner, int, error)
	ListOwnersWithSummaryFn                   func(ctx context.Context, f datastore.OwnerListFilter, targetChefVersion string) ([]datastore.OwnerWithSummary, int, error)
	GetOwnerByNameFn                          func(ctx context.Context, name string) (datastore.Owner, error)
	InsertOwnerFn                             func(ctx context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error)
	UpdateOwnerFn                             func(ctx context.Context, name string, p datastore.UpdateOwnerParams) (datastore.Owner, error)
	DeleteOwnerFn                             func(ctx context.Context, name string) (int, error)
	CountAssignmentsByOwnerFn                 func(ctx context.Context, ownerName string) (map[string]int, error)
	InsertAssignmentFn                        func(ctx context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error)
	ListAssignmentsByOwnerFn                  func(ctx context.Context, f datastore.AssignmentListFilter) ([]datastore.OwnershipAssignment, int, error)
	GetAssignmentFn                           func(ctx context.Context, id string) (datastore.OwnershipAssignment, error)
	DeleteAssignmentFn                        func(ctx context.Context, id string) error
	ReassignOwnershipFn                       func(ctx context.Context, fromOwnerID, toOwnerID string, entityType, organisationID string) (int, int, error)
	LookupOwnershipFn                         func(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error)
	GetOwnerReadinessSummaryFn                func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerReadinessSummary, error)
	GetOwnerCookbookSummaryFn                 func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerCookbookSummary, error)
	GetOwnerGitRepoSummaryFn                  func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerGitRepoSummary, error)
	InsertAuditEntryFn                        func(ctx context.Context, p datastore.InsertAuditEntryParams) error
	ListAuditLogFn                            func(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error)
	GetGitRepoURLForCookbookFn                func(ctx context.Context, cookbookName string) (string, error)
	ListCommittersByRepoFn                    func(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error)
	DeleteGitCookbooksByNameFn                func(ctx context.Context, cookbookName string) (datastore.DeleteGitCookbookResult, error)
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

func (m *mockStore) ListCookbooksByOrganisation(ctx context.Context, organisationID string) ([]datastore.Cookbook, error) {
	if m.ListCookbooksByOrganisationFn != nil {
		return m.ListCookbooksByOrganisationFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) ListCookbooksByName(ctx context.Context, name string) ([]datastore.Cookbook, error) {
	if m.ListCookbooksByNameFn != nil {
		return m.ListCookbooksByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockStore) ListGitCookbooks(ctx context.Context) ([]datastore.Cookbook, error) {
	if m.ListGitCookbooksFn != nil {
		return m.ListGitCookbooksFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListCookbookComplexitiesForCookbook(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
	if m.ListCookbookComplexitiesForCookbookFn != nil {
		return m.ListCookbookComplexitiesForCookbookFn(ctx, cookbookID)
	}
	return nil, nil
}

func (m *mockStore) ListCookbookComplexitiesForOrganisation(ctx context.Context, organisationID string) ([]datastore.CookbookComplexity, error) {
	if m.ListCookbookComplexitiesForOrganisationFn != nil {
		return m.ListCookbookComplexitiesForOrganisationFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) ListCookstyleResultsForCookbook(ctx context.Context, cookbookID string) ([]datastore.CookstyleResult, error) {
	if m.ListCookstyleResultsForCookbookFn != nil {
		return m.ListCookstyleResultsForCookbookFn(ctx, cookbookID)
	}
	return nil, nil
}

func (m *mockStore) GetCookstyleResult(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
	if m.GetCookstyleResultFn != nil {
		return m.GetCookstyleResultFn(ctx, cookbookID, targetChefVersion)
	}
	return nil, nil
}

func (m *mockStore) DeleteCookstyleResultsForCookbook(ctx context.Context, cookbookID string) error {
	if m.DeleteCookstyleResultsForCookbookFn != nil {
		return m.DeleteCookstyleResultsForCookbookFn(ctx, cookbookID)
	}
	return nil
}

func (m *mockStore) DeleteCookbookComplexitiesForCookbook(ctx context.Context, cookbookID string) error {
	if m.DeleteCookbookComplexitiesForCookbookFn != nil {
		return m.DeleteCookbookComplexitiesForCookbookFn(ctx, cookbookID)
	}
	return nil
}

func (m *mockStore) DeleteAutocorrectPreviewsForCookbook(ctx context.Context, cookbookID string) error {
	if m.DeleteAutocorrectPreviewsForCookbookFn != nil {
		return m.DeleteAutocorrectPreviewsForCookbookFn(ctx, cookbookID)
	}
	return nil
}

func (m *mockStore) GetAutocorrectPreview(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
	if m.GetAutocorrectPreviewFn != nil {
		return m.GetAutocorrectPreviewFn(ctx, cookstyleResultID)
	}
	return nil, nil
}

func (m *mockStore) ListAutocorrectPreviewsForCookbook(ctx context.Context, cookbookID string) ([]datastore.AutocorrectPreview, error) {
	if m.ListAutocorrectPreviewsForCookbookFn != nil {
		return m.ListAutocorrectPreviewsForCookbookFn(ctx, cookbookID)
	}
	return nil, nil
}

func (m *mockStore) GetLatestTestKitchenResult(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.TestKitchenResult, error) {
	if m.GetLatestTestKitchenResultFn != nil {
		return m.GetLatestTestKitchenResultFn(ctx, cookbookID, targetChefVersion)
	}
	return nil, nil
}

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

func (m *mockStore) DeleteGitCookbooksByName(ctx context.Context, cookbookName string) (datastore.DeleteGitCookbookResult, error) {
	if m.DeleteGitCookbooksByNameFn != nil {
		return m.DeleteGitCookbooksByNameFn(ctx, cookbookName)
	}
	return datastore.DeleteGitCookbookResult{}, datastore.ErrNotFound
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
