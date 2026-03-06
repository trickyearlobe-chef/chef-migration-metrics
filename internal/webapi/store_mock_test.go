// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"

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
	GetLatestTestKitchenResultFn              func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.TestKitchenResult, error)
	ListLogEntriesFn                          func(ctx context.Context, filter datastore.LogEntryFilter) ([]datastore.LogEntry, error)
	CountLogEntriesFn                         func(ctx context.Context, filter datastore.LogEntryFilter) (int, error)
	GetLogEntryFn                             func(ctx context.Context, id string) (datastore.LogEntry, error)
	ListRoleDependenciesByOrgFn               func(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error)
	CountDependenciesByRoleFn                 func(ctx context.Context, organisationID string) ([]datastore.RoleDependencyCount, error)
	CountRolesPerCookbookFn                   func(ctx context.Context, organisationID string) ([]datastore.CookbookRoleCount, error)
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
