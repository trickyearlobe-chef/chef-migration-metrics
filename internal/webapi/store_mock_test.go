// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
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
	ListCollectionRunsFilteredFn                        func(ctx context.Context, f datastore.CollectionRunFilter) ([]datastore.CollectionRunWithOrg, error)
	CountCollectionRunsFilteredFn                       func(ctx context.Context, f datastore.CollectionRunFilter) (int, error)
	ListNodeSnapshotsByOrganisationFn                   func(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error)
	ListNodeSnapshotsFilteredFn                         func(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.NodeSnapshot, int, error)
	CountNodeVersionDistributionFn                      func(ctx context.Context, f datastore.NodeSnapshotFilter) (map[string]int, int, error)
	CountNodePlatformDistributionFn                     func(ctx context.Context, f datastore.NodeSnapshotFilter) (map[string]int, int, error)
	ListDistinctNodeValuesFn                            func(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error)
	ListDistinctNodeRolesFn                             func(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error)
	ListNodeSnapshotsByCollectionRunFn                  func(ctx context.Context, collectionRunID string) ([]datastore.NodeSnapshot, error)
	CountStaleFreshByCollectionRunFn                    func(ctx context.Context, collectionRunID string) (int, int, int, error)
	ListMetricSnapshotsByOrganisationFn                  func(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error)
	ListMetricSnapshotsByOrganisationAndVersionFn        func(ctx context.Context, organisationID, snapshotType, targetChefVersion string, limit int) ([]datastore.MetricSnapshot, error)
	ListDailyMetricSnapshotsByOrganisationFn             func(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error)
	ListDailyMetricSnapshotsByOrganisationAndVersionFn   func(ctx context.Context, organisationID, snapshotType, targetChefVersion string, limit int) ([]datastore.MetricSnapshot, error)
	GetNodeSnapshotByNameFn                              func(ctx context.Context, organisationID, nodeName string) (datastore.NodeSnapshot, error)
	ListNodeReadinessForSnapshotFn                      func(ctx context.Context, orgName, nodeName string) ([]datastore.NodeReadiness, error)
	ListNodeReadinessByNodeNameFn                       func(ctx context.Context, organisationName, nodeName string) ([]datastore.NodeReadiness, error)
	BulkListNodeReadinessByNodeNamesFn                  func(ctx context.Context, organisationName string, nodeNames []string) (map[string][]datastore.NodeReadiness, error)
	CountNodeReadinessFn                                func(ctx context.Context, organisationName, targetChefVersion string) (int, int, int, error)
	ListCookbooksFilteredFn                             func(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error)
	ListServerCookbooksByOrganisationFn                 func(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error)
	ListServerCookbooksByNameFn                         func(ctx context.Context, name string) ([]datastore.ServerCookbook, error)
	ResetServerCookbookDownloadStatusFn                 func(ctx context.Context, organisationName, name, version string) (datastore.ServerCookbook, error)
	ResetAllServerCookbookDownloadStatusesFn            func(ctx context.Context) (int, error)
	ListGitReposFn                                      func(ctx context.Context) ([]datastore.GitRepo, error)
	ListGitReposByNameFn                                func(ctx context.Context, name string) ([]datastore.GitRepo, error)
	DeleteGitReposByNameFn                              func(ctx context.Context, name string) (datastore.DeleteGitRepoResult, error)
	ListAllGitRepoCookstyleResultsFn                    func(ctx context.Context) ([]datastore.GitRepoCookstyleResult, error)
	ListServerCookbookComplexitiesByCookbookFn          func(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookComplexity, error)
	ListServerCookbookComplexitiesByOrganisationFn      func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error)
	ListServerCookbookCookstyleResultsFn                func(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookCookstyleResult, error)
	ListServerCookbookCookstyleResultsByOrganisationFn  func(ctx context.Context, organisationID string) ([]datastore.ServerCookbookCookstyleResult, error)
	GetServerCookbookCookstyleResultFn                  func(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error)
	GetServerCookbookAutocorrectPreviewFn               func(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookAutocorrectPreview, error)
	DeleteServerCookbookCookstyleResultsByCookbookFn    func(ctx context.Context, orgName, cookbookName, cookbookVersion string) error
	DeleteServerCookbookComplexitiesByCookbookFn        func(ctx context.Context, orgName, cookbookName, cookbookVersion string) error
	DeleteServerCookbookAutocorrectPreviewsByCookbookFn func(ctx context.Context, orgName, cookbookName, cookbookVersion string) error
	DeleteAllServerCookbookCookstyleResultsFn           func(ctx context.Context) error
	DeleteAllServerCookbookComplexitiesFn               func(ctx context.Context) error
	DeleteAllServerCookbookAutocorrectPreviewsFn        func(ctx context.Context) error
	ListGitRepoCookstyleResultsFn                       func(ctx context.Context, gitRepoName, gitRepoURL string) ([]datastore.GitRepoCookstyleResult, error)
	GetGitRepoCookstyleResultFn                         func(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error)
	ListGitRepoComplexitiesByRepoFn                     func(ctx context.Context, gitRepoName, gitRepoURL string) ([]datastore.GitRepoComplexity, error)
	ListAllGitRepoComplexitiesFn                        func(ctx context.Context) ([]datastore.GitRepoComplexity, error)
	GetGitRepoAutocorrectPreviewFn                      func(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*datastore.GitRepoAutocorrectPreview, error)
	DeleteGitRepoCookstyleResultsByRepoFn               func(ctx context.Context, gitRepoName, gitRepoURL string) error
	DeleteGitRepoComplexitiesByRepoFn                   func(ctx context.Context, gitRepoName, gitRepoURL string) error
	DeleteGitRepoAutocorrectPreviewsByRepoFn            func(ctx context.Context, gitRepoName, gitRepoURL string) error
	DeleteAllGitRepoCookstyleResultsFn                  func(ctx context.Context) error
	DeleteAllGitRepoComplexitiesFn                      func(ctx context.Context) error
	DeleteAllGitRepoAutocorrectPreviewsFn               func(ctx context.Context) error
	ListLogEntriesFn                                    func(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error)
	CountLogEntriesFn                                   func(ctx context.Context, filter datastore.LogEntryFilter) (int, error)
	GetLogEntryFn                                       func(ctx context.Context, id int64) (datastore.LogEntry, error)
	ListRolesFilteredFn                                 func(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error)
	GetRoleDetailFn                                     func(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error)
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
	GetAssignmentFn                                     func(ctx context.Context, id int64) (datastore.OwnershipAssignment, error)
	DeleteAssignmentFn                                  func(ctx context.Context, id int64) error
	ReassignOwnershipFn                                 func(ctx context.Context, fromOwnerName, toOwnerName string, entityType, organisationName string) (int, int, error)
	LookupOwnershipFn                                   func(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error)
	GetOwnerReadinessSummaryFn                          func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerReadinessSummary, error)
	GetOwnerCookbookSummaryFn                           func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerCookbookSummary, error)
	GetOwnerGitRepoSummaryFn                            func(ctx context.Context, ownerName, targetChefVersion string) (datastore.OwnerGitRepoSummary, error)
	GetGitRepoURLForCookbookFn                          func(ctx context.Context, cookbookName string) (string, error)
	ListCommittersByRepoFn                              func(ctx context.Context, f datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error)
	GetOwnerEmailsForGitRepoFn                          func(ctx context.Context, gitRepoURL string) (map[string]bool, error)
	InsertAuditEntryFn                                  func(ctx context.Context, p datastore.InsertAuditEntryParams) error
	ListAuditLogFn                                      func(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error)
	DatabaseSizeFn                                      func(ctx context.Context) (int64, error)
	DatabaseTableSizesFn                                func(ctx context.Context) ([]datastore.TableSize, error)
	PgStatStatementsAvailableFn                         func(ctx context.Context) bool
	TopQueryStatsFn                                     func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error)
	TableStatsFn                                        func(ctx context.Context) ([]datastore.TableStat, error)
	IndexStatsFn                                        func(ctx context.Context) ([]datastore.IndexStat, error)
	ActiveQueriesFn                                     func(ctx context.Context) ([]datastore.ActiveQuery, error)
	ResetPgStatsFn                                      func(ctx context.Context) error
	GetCookbookPlatformCoverageFn                       func(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error)
	GetKitchenAnalysisSummaryFn                         func(ctx context.Context) (*datastore.KitchenAnalysisSummary, error)
	ListKitchenAnalysisResultsFn                        func(ctx context.Context) ([]datastore.KitchenAnalysisResult, error)
	ListKitchenAnalysisResultsFilteredFn                func(ctx context.Context, driverName string, hasLocalOverride *bool) ([]datastore.KitchenAnalysisResult, error)
	GetKitchenAnalysisResultByNameFn                    func(ctx context.Context, gitRepoName string) (*datastore.KitchenAnalysisResult, error)
	ListDiscoveredPlatformsFn                           func(ctx context.Context) ([]datastore.KitchenDiscoveredPlatform, error)
	ListDiscoveredPlatformsFilteredFn                   func(ctx context.Context, osFamily string, minCount int) ([]datastore.KitchenDiscoveredPlatform, error)
	GetRuntimeSettingFn                                 func(ctx context.Context, key string) (*datastore.RuntimeSetting, error)
	SetRuntimeSettingFn                                 func(ctx context.Context, key string, value json.RawMessage, updatedBy string) error
	DeleteRuntimeSettingFn                              func(ctx context.Context, key string) error
	ListTrackedVMsFn                                    func(ctx context.Context) ([]datastore.TrackedVM, error)
	ListTrackedVMsFilteredFn                            func(ctx context.Context, status string) ([]datastore.TrackedVM, error)
	GetTrackedVMFn                                      func(ctx context.Context, id string) (*datastore.TrackedVM, error)
	ListOrphanedVMsFn                                   func(ctx context.Context) ([]datastore.TrackedVM, error)
	MarkVMDestroyedFn                                   func(ctx context.Context, id string) error
	MarkVMOrphanedFn                                    func(ctx context.Context, id string) error
	CountTrackedVMsByStatusFn                           func(ctx context.Context) (map[string]int, error)
	ListNodeKitchenRunsFn                               func(ctx context.Context, orgName string) ([]datastore.NodeKitchenRun, error)
	ListNodeKitchenRunsByNodeFn                         func(ctx context.Context, orgName, nodeName string) ([]datastore.NodeKitchenRun, error)
	GetNodeKitchenRunFn                                 func(ctx context.Context, id string) (*datastore.NodeKitchenRun, error)
	DeleteNodeKitchenRunFn                              func(ctx context.Context, id string) error

	// Kitchen Batches
	CreateKitchenBatchFn       func(ctx context.Context, p datastore.CreateKitchenBatchParams) (datastore.KitchenBatch, error)
	GetKitchenBatchFn          func(ctx context.Context, id string) (datastore.KitchenBatch, error)
	ListKitchenBatchesFn       func(ctx context.Context) ([]datastore.KitchenBatch, error)
	UpdateKitchenBatchFn       func(ctx context.Context, id string, p datastore.UpdateKitchenBatchParams) (datastore.KitchenBatch, error)
	UpdateKitchenBatchStatusFn func(ctx context.Context, id string, status string, now time.Time) (datastore.KitchenBatch, error)
	DeleteKitchenBatchFn       func(ctx context.Context, id string) error
	UpdateKitchenBatchStatusIfCurrentFn func(ctx context.Context, id string, expectedStatus string, newStatus string, now time.Time) (datastore.KitchenBatch, error)

	// Kitchen Batch Instances
	CreateBatchInstanceFn          func(ctx context.Context, p datastore.CreateBatchInstanceParams) (datastore.KitchenBatchInstance, error)
	CreateBatchInstancesFn         func(ctx context.Context, params []datastore.CreateBatchInstanceParams) ([]datastore.KitchenBatchInstance, error)
	ListBatchInstancesFn           func(ctx context.Context, batchID string) ([]datastore.KitchenBatchInstance, error)
	UpdateBatchInstanceStatusFn    func(ctx context.Context, id string, status string, errorMessage string, now time.Time) error
	CountBatchInstancesByStatusFn  func(ctx context.Context, batchID string) (map[string]int, error)
	CancelPendingBatchInstancesFn  func(ctx context.Context, batchID string) (int, error)

	// Git Repo Kitchen Exclusions
	SetGitRepoKitchenExclusionFn   func(ctx context.Context, name string, reason string, excludedBy string) error
	ClearGitRepoKitchenExclusionFn func(ctx context.Context, name string) error
	ListExcludedGitReposFn         func(ctx context.Context) ([]datastore.GitRepo, error)

	// Git Kitchen Results (per-instance)
	UpsertGitKitchenResultFn         func(ctx context.Context, p datastore.UpsertGitKitchenResultParams) (datastore.GitKitchenResult, error)
	GetGitKitchenResultFn            func(ctx context.Context, id string) (datastore.GitKitchenResult, error)
	ListGitKitchenResultsFn          func(ctx context.Context) ([]datastore.GitKitchenResult, error)
	ListGitKitchenResultsByRepoFn    func(ctx context.Context, gitRepoName string) ([]datastore.GitKitchenResult, error)
	DeleteGitKitchenResultsByRepoFn  func(ctx context.Context, gitRepoName string) error
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

func (m *mockStore) ListCollectionRunsFiltered(ctx context.Context, f datastore.CollectionRunFilter) ([]datastore.CollectionRunWithOrg, error) {
	if m.ListCollectionRunsFilteredFn != nil {
		return m.ListCollectionRunsFilteredFn(ctx, f)
	}
	return nil, nil
}

func (m *mockStore) CountCollectionRunsFiltered(ctx context.Context, f datastore.CollectionRunFilter) (int, error) {
	if m.CountCollectionRunsFilteredFn != nil {
		return m.CountCollectionRunsFilteredFn(ctx, f)
	}
	return 0, nil
}

func (m *mockStore) ListNodeSnapshotsByOrganisation(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error) {
	if m.ListNodeSnapshotsByOrganisationFn != nil {
		return m.ListNodeSnapshotsByOrganisationFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) ListNodeSnapshotsFiltered(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.NodeSnapshot, int, error) {
	if m.ListNodeSnapshotsFilteredFn != nil {
		return m.ListNodeSnapshotsFilteredFn(ctx, f)
	}
	return nil, 0, nil
}

func (m *mockStore) CountNodeVersionDistribution(ctx context.Context, f datastore.NodeSnapshotFilter) (map[string]int, int, error) {
	if m.CountNodeVersionDistributionFn != nil {
		return m.CountNodeVersionDistributionFn(ctx, f)
	}
	return nil, 0, nil
}

func (m *mockStore) CountNodePlatformDistribution(ctx context.Context, f datastore.NodeSnapshotFilter) (map[string]int, int, error) {
	if m.CountNodePlatformDistributionFn != nil {
		return m.CountNodePlatformDistributionFn(ctx, f)
	}
	return nil, 0, nil
}

func (m *mockStore) ListDistinctNodeValues(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error) {
	if m.ListDistinctNodeValuesFn != nil {
		return m.ListDistinctNodeValuesFn(ctx, f, columnExpr, opts)
	}
	return nil, nil
}

func (m *mockStore) ListDistinctNodeRoles(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error) {
	if m.ListDistinctNodeRolesFn != nil {
		return m.ListDistinctNodeRolesFn(ctx, f, opts)
	}
	return nil, nil
}

func (m *mockStore) ListNodeSnapshotsByCollectionRun(ctx context.Context, collectionRunID string) ([]datastore.NodeSnapshot, error) {
	if m.ListNodeSnapshotsByCollectionRunFn != nil {
		return m.ListNodeSnapshotsByCollectionRunFn(ctx, collectionRunID)
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

func (m *mockStore) ListMetricSnapshotsByOrganisationAndVersion(ctx context.Context, organisationID, snapshotType, targetChefVersion string, limit int) ([]datastore.MetricSnapshot, error) {
	if m.ListMetricSnapshotsByOrganisationAndVersionFn != nil {
		return m.ListMetricSnapshotsByOrganisationAndVersionFn(ctx, organisationID, snapshotType, targetChefVersion, limit)
	}
	return nil, nil
}

func (m *mockStore) ListDailyMetricSnapshotsByOrganisation(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error) {
	if m.ListDailyMetricSnapshotsByOrganisationFn != nil {
		return m.ListDailyMetricSnapshotsByOrganisationFn(ctx, organisationID, snapshotType, limit)
	}
	// Fall back to non-daily variant for backward-compatible tests.
	if m.ListMetricSnapshotsByOrganisationFn != nil {
		return m.ListMetricSnapshotsByOrganisationFn(ctx, organisationID, snapshotType, limit)
	}
	return nil, nil
}

func (m *mockStore) ListDailyMetricSnapshotsByOrganisationAndVersion(ctx context.Context, organisationID, snapshotType, targetChefVersion string, limit int) ([]datastore.MetricSnapshot, error) {
	if m.ListDailyMetricSnapshotsByOrganisationAndVersionFn != nil {
		return m.ListDailyMetricSnapshotsByOrganisationAndVersionFn(ctx, organisationID, snapshotType, targetChefVersion, limit)
	}
	// Fall back to non-daily variant for backward-compatible tests.
	if m.ListMetricSnapshotsByOrganisationAndVersionFn != nil {
		return m.ListMetricSnapshotsByOrganisationAndVersionFn(ctx, organisationID, snapshotType, targetChefVersion, limit)
	}
	return nil, nil
}

func (m *mockStore) GetNodeSnapshotByName(ctx context.Context, organisationID, nodeName string) (datastore.NodeSnapshot, error) {
	if m.GetNodeSnapshotByNameFn != nil {
		return m.GetNodeSnapshotByNameFn(ctx, organisationID, nodeName)
	}
	return datastore.NodeSnapshot{}, nil
}

func (m *mockStore) ListNodeReadinessForSnapshot(ctx context.Context, orgName, nodeName string) ([]datastore.NodeReadiness, error) {
	if m.ListNodeReadinessForSnapshotFn != nil {
		return m.ListNodeReadinessForSnapshotFn(ctx, orgName, nodeName)
	}
	return nil, nil
}

func (m *mockStore) ListNodeReadinessByNodeName(ctx context.Context, organisationName, nodeName string) ([]datastore.NodeReadiness, error) {
	if m.ListNodeReadinessByNodeNameFn != nil {
		return m.ListNodeReadinessByNodeNameFn(ctx, organisationName, nodeName)
	}
	return nil, nil
}

func (m *mockStore) BulkListNodeReadinessByNodeNames(ctx context.Context, organisationName string, nodeNames []string) (map[string][]datastore.NodeReadiness, error) {
	if m.BulkListNodeReadinessByNodeNamesFn != nil {
		return m.BulkListNodeReadinessByNodeNamesFn(ctx, organisationName, nodeNames)
	}
	return nil, nil
}

func (m *mockStore) CountNodeReadiness(ctx context.Context, organisationName, targetChefVersion string) (int, int, int, error) {
	if m.CountNodeReadinessFn != nil {
		return m.CountNodeReadinessFn(ctx, organisationName, targetChefVersion)
	}
	return 0, 0, 0, nil
}

// -----------------------------------------------------------------
// Server cookbooks
// -----------------------------------------------------------------

func (m *mockStore) ListCookbooksFiltered(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
	if m.ListCookbooksFilteredFn != nil {
		return m.ListCookbooksFilteredFn(ctx, f)
	}
	return nil, 0, nil
}

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

func (m *mockStore) ResetServerCookbookDownloadStatus(ctx context.Context, organisationName, name, version string) (datastore.ServerCookbook, error) {
	if m.ResetServerCookbookDownloadStatusFn != nil {
		return m.ResetServerCookbookDownloadStatusFn(ctx, organisationName, name, version)
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

func (m *mockStore) ListGitRepos(ctx context.Context) ([]datastore.GitRepo, error) {
	if m.ListGitReposFn != nil {
		return m.ListGitReposFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListAllGitRepoCookstyleResults(ctx context.Context) ([]datastore.GitRepoCookstyleResult, error) {
	if m.ListAllGitRepoCookstyleResultsFn != nil {
		return m.ListAllGitRepoCookstyleResultsFn(ctx)
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

func (m *mockStore) ListServerCookbookComplexitiesByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookComplexity, error) {
	if m.ListServerCookbookComplexitiesByCookbookFn != nil {
		return m.ListServerCookbookComplexitiesByCookbookFn(ctx, orgName, cookbookName, cookbookVersion)
	}
	return nil, nil
}

func (m *mockStore) ListServerCookbookComplexitiesByOrganisation(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error) {
	if m.ListServerCookbookComplexitiesByOrganisationFn != nil {
		return m.ListServerCookbookComplexitiesByOrganisationFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) ListServerCookbookCookstyleResults(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookCookstyleResult, error) {
	if m.ListServerCookbookCookstyleResultsFn != nil {
		return m.ListServerCookbookCookstyleResultsFn(ctx, orgName, cookbookName, cookbookVersion)
	}
	return nil, nil
}

func (m *mockStore) ListServerCookbookCookstyleResultsByOrganisation(ctx context.Context, organisationID string) ([]datastore.ServerCookbookCookstyleResult, error) {
	if m.ListServerCookbookCookstyleResultsByOrganisationFn != nil {
		return m.ListServerCookbookCookstyleResultsByOrganisationFn(ctx, organisationID)
	}
	return nil, nil
}

func (m *mockStore) GetServerCookbookCookstyleResult(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error) {
	if m.GetServerCookbookCookstyleResultFn != nil {
		return m.GetServerCookbookCookstyleResultFn(ctx, orgName, cookbookName, cookbookVersion, targetChefVersion)
	}
	return nil, nil
}

func (m *mockStore) GetServerCookbookAutocorrectPreview(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookAutocorrectPreview, error) {
	if m.GetServerCookbookAutocorrectPreviewFn != nil {
		return m.GetServerCookbookAutocorrectPreviewFn(ctx, orgName, cookbookName, cookbookVersion, targetChefVersion)
	}
	return nil, nil
}

func (m *mockStore) DeleteServerCookbookCookstyleResultsByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error {
	if m.DeleteServerCookbookCookstyleResultsByCookbookFn != nil {
		return m.DeleteServerCookbookCookstyleResultsByCookbookFn(ctx, orgName, cookbookName, cookbookVersion)
	}
	return nil
}

func (m *mockStore) DeleteServerCookbookComplexitiesByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error {
	if m.DeleteServerCookbookComplexitiesByCookbookFn != nil {
		return m.DeleteServerCookbookComplexitiesByCookbookFn(ctx, orgName, cookbookName, cookbookVersion)
	}
	return nil
}

func (m *mockStore) DeleteServerCookbookAutocorrectPreviewsByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error {
	if m.DeleteServerCookbookAutocorrectPreviewsByCookbookFn != nil {
		return m.DeleteServerCookbookAutocorrectPreviewsByCookbookFn(ctx, orgName, cookbookName, cookbookVersion)
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

func (m *mockStore) ListGitRepoCookstyleResults(ctx context.Context, gitRepoName, gitRepoURL string) ([]datastore.GitRepoCookstyleResult, error) {
	if m.ListGitRepoCookstyleResultsFn != nil {
		return m.ListGitRepoCookstyleResultsFn(ctx, gitRepoName, gitRepoURL)
	}
	return nil, nil
}

func (m *mockStore) GetGitRepoCookstyleResult(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error) {
	if m.GetGitRepoCookstyleResultFn != nil {
		return m.GetGitRepoCookstyleResultFn(ctx, gitRepoName, gitRepoURL, targetChefVersion)
	}
	return nil, nil
}

func (m *mockStore) ListGitRepoComplexitiesByRepo(ctx context.Context, gitRepoName, gitRepoURL string) ([]datastore.GitRepoComplexity, error) {
	if m.ListGitRepoComplexitiesByRepoFn != nil {
		return m.ListGitRepoComplexitiesByRepoFn(ctx, gitRepoName, gitRepoURL)
	}
	return nil, nil
}

func (m *mockStore) ListAllGitRepoComplexities(ctx context.Context) ([]datastore.GitRepoComplexity, error) {
	if m.ListAllGitRepoComplexitiesFn != nil {
		return m.ListAllGitRepoComplexitiesFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) GetGitRepoAutocorrectPreview(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*datastore.GitRepoAutocorrectPreview, error) {
	if m.GetGitRepoAutocorrectPreviewFn != nil {
		return m.GetGitRepoAutocorrectPreviewFn(ctx, gitRepoName, gitRepoURL, targetChefVersion)
	}
	return nil, nil
}

func (m *mockStore) DeleteGitRepoCookstyleResultsByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error {
	if m.DeleteGitRepoCookstyleResultsByRepoFn != nil {
		return m.DeleteGitRepoCookstyleResultsByRepoFn(ctx, gitRepoName, gitRepoURL)
	}
	return nil
}

func (m *mockStore) DeleteGitRepoComplexitiesByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error {
	if m.DeleteGitRepoComplexitiesByRepoFn != nil {
		return m.DeleteGitRepoComplexitiesByRepoFn(ctx, gitRepoName, gitRepoURL)
	}
	return nil
}

func (m *mockStore) DeleteGitRepoAutocorrectPreviewsByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error {
	if m.DeleteGitRepoAutocorrectPreviewsByRepoFn != nil {
		return m.DeleteGitRepoAutocorrectPreviewsByRepoFn(ctx, gitRepoName, gitRepoURL)
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

func (m *mockStore) GetLogEntry(ctx context.Context, id int64) (datastore.LogEntry, error) {
	if m.GetLogEntryFn != nil {
		return m.GetLogEntryFn(ctx, id)
	}
	return datastore.LogEntry{}, nil
}

// -----------------------------------------------------------------
// Role dependencies
// -----------------------------------------------------------------

func (m *mockStore) ListRolesFiltered(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
	if m.ListRolesFilteredFn != nil {
		return m.ListRolesFilteredFn(ctx, f)
	}
	return nil, 0, datastore.RoleFilterSummary{}, nil
}

func (m *mockStore) GetRoleDetail(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error) {
	if m.GetRoleDetailFn != nil {
		return m.GetRoleDetailFn(ctx, roleName, targetChefVersion)
	}
	return nil, datastore.ErrNotFound
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

func (m *mockStore) GetAssignment(ctx context.Context, id int64) (datastore.OwnershipAssignment, error) {
	if m.GetAssignmentFn != nil {
		return m.GetAssignmentFn(ctx, id)
	}
	return datastore.OwnershipAssignment{}, nil
}

func (m *mockStore) DeleteAssignment(ctx context.Context, id int64) error {
	if m.DeleteAssignmentFn != nil {
		return m.DeleteAssignmentFn(ctx, id)
	}
	return nil
}

func (m *mockStore) ReassignOwnership(ctx context.Context, fromOwnerName, toOwnerName string, entityType, organisationName string) (int, int, error) {
	if m.ReassignOwnershipFn != nil {
		return m.ReassignOwnershipFn(ctx, fromOwnerName, toOwnerName, entityType, organisationName)
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

// -----------------------------------------------------------------
// System health
// -----------------------------------------------------------------

func (m *mockStore) DatabaseSize(ctx context.Context) (int64, error) {
	if m.DatabaseSizeFn != nil {
		return m.DatabaseSizeFn(ctx)
	}
	return 0, nil
}

func (m *mockStore) DatabaseTableSizes(ctx context.Context) ([]datastore.TableSize, error) {
	if m.DatabaseTableSizesFn != nil {
		return m.DatabaseTableSizesFn(ctx)
	}
	return nil, nil
}

// -----------------------------------------------------------------
// PostgreSQL performance stats
// -----------------------------------------------------------------

func (m *mockStore) PgStatStatementsAvailable(ctx context.Context) bool {
	if m.PgStatStatementsAvailableFn != nil {
		return m.PgStatStatementsAvailableFn(ctx)
	}
	return false
}

func (m *mockStore) TopQueryStats(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
	if m.TopQueryStatsFn != nil {
		return m.TopQueryStatsFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockStore) TableStats(ctx context.Context) ([]datastore.TableStat, error) {
	if m.TableStatsFn != nil {
		return m.TableStatsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) IndexStats(ctx context.Context) ([]datastore.IndexStat, error) {
	if m.IndexStatsFn != nil {
		return m.IndexStatsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ActiveQueries(ctx context.Context) ([]datastore.ActiveQuery, error) {
	if m.ActiveQueriesFn != nil {
		return m.ActiveQueriesFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ResetPgStats(ctx context.Context) error {
	if m.ResetPgStatsFn != nil {
		return m.ResetPgStatsFn(ctx)
	}
	return nil
}

func (m *mockStore) GetCookbookPlatformCoverage(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error) {
	if m.GetCookbookPlatformCoverageFn != nil {
		return m.GetCookbookPlatformCoverageFn(ctx, cookbookName)
	}
	return nil, nil
}

func (m *mockStore) GetKitchenAnalysisSummary(ctx context.Context) (*datastore.KitchenAnalysisSummary, error) {
	if m.GetKitchenAnalysisSummaryFn != nil {
		return m.GetKitchenAnalysisSummaryFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListKitchenAnalysisResults(ctx context.Context) ([]datastore.KitchenAnalysisResult, error) {
	if m.ListKitchenAnalysisResultsFn != nil {
		return m.ListKitchenAnalysisResultsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListKitchenAnalysisResultsFiltered(ctx context.Context, driverName string, hasLocalOverride *bool) ([]datastore.KitchenAnalysisResult, error) {
	if m.ListKitchenAnalysisResultsFilteredFn != nil {
		return m.ListKitchenAnalysisResultsFilteredFn(ctx, driverName, hasLocalOverride)
	}
	return nil, nil
}

func (m *mockStore) GetKitchenAnalysisResultByName(ctx context.Context, gitRepoName string) (*datastore.KitchenAnalysisResult, error) {
	if m.GetKitchenAnalysisResultByNameFn != nil {
		return m.GetKitchenAnalysisResultByNameFn(ctx, gitRepoName)
	}
	return nil, nil
}

func (m *mockStore) ListDiscoveredPlatforms(ctx context.Context) ([]datastore.KitchenDiscoveredPlatform, error) {
	if m.ListDiscoveredPlatformsFn != nil {
		return m.ListDiscoveredPlatformsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListDiscoveredPlatformsFiltered(ctx context.Context, osFamily string, minCount int) ([]datastore.KitchenDiscoveredPlatform, error) {
	if m.ListDiscoveredPlatformsFilteredFn != nil {
		return m.ListDiscoveredPlatformsFilteredFn(ctx, osFamily, minCount)
	}
	return nil, nil
}

func (m *mockStore) GetRuntimeSetting(ctx context.Context, key string) (*datastore.RuntimeSetting, error) {
	if m.GetRuntimeSettingFn != nil {
		return m.GetRuntimeSettingFn(ctx, key)
	}
	return nil, nil
}

func (m *mockStore) SetRuntimeSetting(ctx context.Context, key string, value json.RawMessage, updatedBy string) error {
	if m.SetRuntimeSettingFn != nil {
		return m.SetRuntimeSettingFn(ctx, key, value, updatedBy)
	}
	return nil
}

func (m *mockStore) DeleteRuntimeSetting(ctx context.Context, key string) error {
	if m.DeleteRuntimeSettingFn != nil {
		return m.DeleteRuntimeSettingFn(ctx, key)
	}
	return nil
}

// ---------------------------------------------------------------------------
// VM Tracking
// ---------------------------------------------------------------------------

func (m *mockStore) ListTrackedVMs(ctx context.Context) ([]datastore.TrackedVM, error) {
	if m.ListTrackedVMsFn != nil {
		return m.ListTrackedVMsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListTrackedVMsFiltered(ctx context.Context, status string) ([]datastore.TrackedVM, error) {
	if m.ListTrackedVMsFilteredFn != nil {
		return m.ListTrackedVMsFilteredFn(ctx, status)
	}
	return nil, nil
}

func (m *mockStore) GetTrackedVM(ctx context.Context, id string) (*datastore.TrackedVM, error) {
	if m.GetTrackedVMFn != nil {
		return m.GetTrackedVMFn(ctx, id)
	}
	return nil, nil
}

func (m *mockStore) ListOrphanedVMs(ctx context.Context) ([]datastore.TrackedVM, error) {
	if m.ListOrphanedVMsFn != nil {
		return m.ListOrphanedVMsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) MarkVMDestroyed(ctx context.Context, id string) error {
	if m.MarkVMDestroyedFn != nil {
		return m.MarkVMDestroyedFn(ctx, id)
	}
	return nil
}

func (m *mockStore) MarkVMOrphaned(ctx context.Context, id string) error {
	if m.MarkVMOrphanedFn != nil {
		return m.MarkVMOrphanedFn(ctx, id)
	}
	return nil
}

func (m *mockStore) CountTrackedVMsByStatus(ctx context.Context) (map[string]int, error) {
	if m.CountTrackedVMsByStatusFn != nil {
		return m.CountTrackedVMsByStatusFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListNodeKitchenRuns(ctx context.Context, orgName string) ([]datastore.NodeKitchenRun, error) {
	if m.ListNodeKitchenRunsFn != nil {
		return m.ListNodeKitchenRunsFn(ctx, orgName)
	}
	return nil, nil
}

func (m *mockStore) ListNodeKitchenRunsByNode(ctx context.Context, orgName, nodeName string) ([]datastore.NodeKitchenRun, error) {
	if m.ListNodeKitchenRunsByNodeFn != nil {
		return m.ListNodeKitchenRunsByNodeFn(ctx, orgName, nodeName)
	}
	return nil, nil
}

func (m *mockStore) GetNodeKitchenRun(ctx context.Context, id string) (*datastore.NodeKitchenRun, error) {
	if m.GetNodeKitchenRunFn != nil {
		return m.GetNodeKitchenRunFn(ctx, id)
	}
	return nil, nil
}

func (m *mockStore) DeleteNodeKitchenRun(ctx context.Context, id string) error {
	if m.DeleteNodeKitchenRunFn != nil {
		return m.DeleteNodeKitchenRunFn(ctx, id)
	}
	return nil
}

func (m *mockStore) CreateKitchenBatch(ctx context.Context, p datastore.CreateKitchenBatchParams) (datastore.KitchenBatch, error) {
	if m.CreateKitchenBatchFn != nil {
		return m.CreateKitchenBatchFn(ctx, p)
	}
	panic("mockStore.CreateKitchenBatchFn not set")
}

func (m *mockStore) GetKitchenBatch(ctx context.Context, id string) (datastore.KitchenBatch, error) {
	if m.GetKitchenBatchFn != nil {
		return m.GetKitchenBatchFn(ctx, id)
	}
	panic("mockStore.GetKitchenBatchFn not set")
}

func (m *mockStore) ListKitchenBatches(ctx context.Context) ([]datastore.KitchenBatch, error) {
	if m.ListKitchenBatchesFn != nil {
		return m.ListKitchenBatchesFn(ctx)
	}
	panic("mockStore.ListKitchenBatchesFn not set")
}

func (m *mockStore) UpdateKitchenBatch(ctx context.Context, id string, p datastore.UpdateKitchenBatchParams) (datastore.KitchenBatch, error) {
	if m.UpdateKitchenBatchFn != nil {
		return m.UpdateKitchenBatchFn(ctx, id, p)
	}
	panic("mockStore.UpdateKitchenBatchFn not set")
}

func (m *mockStore) UpdateKitchenBatchStatus(ctx context.Context, id string, status string, now time.Time) (datastore.KitchenBatch, error) {
	if m.UpdateKitchenBatchStatusFn != nil {
		return m.UpdateKitchenBatchStatusFn(ctx, id, status, now)
	}
	panic("mockStore.UpdateKitchenBatchStatusFn not set")
}

func (m *mockStore) DeleteKitchenBatch(ctx context.Context, id string) error {
	if m.DeleteKitchenBatchFn != nil {
		return m.DeleteKitchenBatchFn(ctx, id)
	}
	panic("mockStore.DeleteKitchenBatchFn not set")
}

func (m *mockStore) UpdateKitchenBatchStatusIfCurrent(ctx context.Context, id string, expectedStatus string, newStatus string, now time.Time) (datastore.KitchenBatch, error) {
	if m.UpdateKitchenBatchStatusIfCurrentFn != nil {
		return m.UpdateKitchenBatchStatusIfCurrentFn(ctx, id, expectedStatus, newStatus, now)
	}
	panic("mockStore.UpdateKitchenBatchStatusIfCurrentFn not set")
}

func (m *mockStore) CreateBatchInstance(ctx context.Context, p datastore.CreateBatchInstanceParams) (datastore.KitchenBatchInstance, error) {
	if m.CreateBatchInstanceFn != nil {
		return m.CreateBatchInstanceFn(ctx, p)
	}
	panic("mockStore.CreateBatchInstanceFn not set")
}

func (m *mockStore) CreateBatchInstances(ctx context.Context, params []datastore.CreateBatchInstanceParams) ([]datastore.KitchenBatchInstance, error) {
	if m.CreateBatchInstancesFn != nil {
		return m.CreateBatchInstancesFn(ctx, params)
	}
	panic("mockStore.CreateBatchInstancesFn not set")
}

func (m *mockStore) ListBatchInstances(ctx context.Context, batchID string) ([]datastore.KitchenBatchInstance, error) {
	if m.ListBatchInstancesFn != nil {
		return m.ListBatchInstancesFn(ctx, batchID)
	}
	panic("mockStore.ListBatchInstancesFn not set")
}

func (m *mockStore) UpdateBatchInstanceStatus(ctx context.Context, id string, status string, errorMessage string, now time.Time) error {
	if m.UpdateBatchInstanceStatusFn != nil {
		return m.UpdateBatchInstanceStatusFn(ctx, id, status, errorMessage, now)
	}
	panic("mockStore.UpdateBatchInstanceStatusFn not set")
}

func (m *mockStore) CountBatchInstancesByStatus(ctx context.Context, batchID string) (map[string]int, error) {
	if m.CountBatchInstancesByStatusFn != nil {
		return m.CountBatchInstancesByStatusFn(ctx, batchID)
	}
	panic("mockStore.CountBatchInstancesByStatusFn not set")
}

func (m *mockStore) CancelPendingBatchInstances(ctx context.Context, batchID string) (int, error) {
	if m.CancelPendingBatchInstancesFn != nil {
		return m.CancelPendingBatchInstancesFn(ctx, batchID)
	}
	panic("mockStore.CancelPendingBatchInstancesFn not set")
}

func (m *mockStore) SetGitRepoKitchenExclusion(ctx context.Context, name string, reason string, excludedBy string) error {
	if m.SetGitRepoKitchenExclusionFn != nil {
		return m.SetGitRepoKitchenExclusionFn(ctx, name, reason, excludedBy)
	}
	panic("mockStore.SetGitRepoKitchenExclusionFn not set")
}

func (m *mockStore) ClearGitRepoKitchenExclusion(ctx context.Context, name string) error {
	if m.ClearGitRepoKitchenExclusionFn != nil {
		return m.ClearGitRepoKitchenExclusionFn(ctx, name)
	}
	panic("mockStore.ClearGitRepoKitchenExclusionFn not set")
}

func (m *mockStore) ListExcludedGitRepos(ctx context.Context) ([]datastore.GitRepo, error) {
	if m.ListExcludedGitReposFn != nil {
		return m.ListExcludedGitReposFn(ctx)
	}
	panic("mockStore.ListExcludedGitReposFn not set")
}

func (m *mockStore) UpsertGitKitchenResult(ctx context.Context, p datastore.UpsertGitKitchenResultParams) (datastore.GitKitchenResult, error) {
	if m.UpsertGitKitchenResultFn != nil {
		return m.UpsertGitKitchenResultFn(ctx, p)
	}
	return datastore.GitKitchenResult{}, nil
}

func (m *mockStore) GetGitKitchenResult(ctx context.Context, id string) (datastore.GitKitchenResult, error) {
	if m.GetGitKitchenResultFn != nil {
		return m.GetGitKitchenResultFn(ctx, id)
	}
	return datastore.GitKitchenResult{}, datastore.ErrNotFound
}

func (m *mockStore) ListGitKitchenResults(ctx context.Context) ([]datastore.GitKitchenResult, error) {
	if m.ListGitKitchenResultsFn != nil {
		return m.ListGitKitchenResultsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) ListGitKitchenResultsByRepo(ctx context.Context, gitRepoName string) ([]datastore.GitKitchenResult, error) {
	if m.ListGitKitchenResultsByRepoFn != nil {
		return m.ListGitKitchenResultsByRepoFn(ctx, gitRepoName)
	}
	return nil, nil
}

func (m *mockStore) DeleteGitKitchenResultsByRepo(ctx context.Context, gitRepoName string) error {
	if m.DeleteGitKitchenResultsByRepoFn != nil {
		return m.DeleteGitKitchenResultsByRepoFn(ctx, gitRepoName)
	}
	return nil
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
