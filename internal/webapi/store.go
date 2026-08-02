// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ingest"
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

	// ListCollectionRunsFiltered returns collection runs across all
	// organisations matching the given filter, joined with org name,
	// ordered by started_at descending.
	ListCollectionRunsFiltered(ctx context.Context, f datastore.CollectionRunFilter) ([]datastore.CollectionRunWithOrg, error)

	// CountCollectionRunsFiltered returns the total number of collection
	// runs matching the given filter (ignoring Limit and Offset).
	CountCollectionRunsFiltered(ctx context.Context, f datastore.CollectionRunFilter) (int, error)

	// -----------------------------------------------------------------
	// Node snapshots
	// -----------------------------------------------------------------

	// ListNodeSnapshotsByOrganisation returns the latest node snapshots for
	// the given organisation.
	ListNodeSnapshotsByOrganisation(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error)

	// ListNodeSnapshotsFiltered returns node snapshots matching the given
	// filter with SQL WHERE clause push-down. Returns the matching rows,
	// the total count of all matching rows (for pagination), and any error.
	ListNodeSnapshotsFiltered(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.NodeSnapshot, int, error)

	// ListNodeSnapshotsForExport returns up to limit snapshots matching the
	// filter, ordered by the unique (organisation_name, node_name) tuple and
	// strictly after the cursor — keyset pagination for streaming exports.
	ListNodeSnapshotsForExport(ctx context.Context, f datastore.NodeSnapshotFilter, after datastore.NodeSnapshotCursor, limit int) ([]datastore.NodeSnapshot, error)

	// CountNodeSnapshotsFiltered returns the count of node snapshots matching
	// the filter without loading rows (used for sync vs async export dispatch).
	CountNodeSnapshotsFiltered(ctx context.Context, f datastore.NodeSnapshotFilter) (int, error)

	// CountNodeVersionDistribution returns a map of chef_version → count
	// for nodes matching the given filter, aggregated in SQL.
	CountNodeVersionDistribution(ctx context.Context, f datastore.NodeSnapshotFilter) (map[string]int, int, error)

	// CountNodePlatformDistribution returns a map of "platform version" → count
	// for nodes matching the given filter, aggregated in SQL.
	CountNodePlatformDistribution(ctx context.Context, f datastore.NodeSnapshotFilter) (map[string]int, int, error)

	// CountNodePlatformDistributionDetailed returns platform distribution with
	// individual platform, version, family, and caption columns for accurate resolution.
	CountNodePlatformDistributionDetailed(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.PlatformDistributionRow, int, error)

	// ListDistinctNodeValues returns sorted distinct non-empty values for the
	// given column expression from nodes matching the filter.
	ListDistinctNodeValues(ctx context.Context, f datastore.NodeSnapshotFilter, columnExpr string, opts datastore.DistinctValueOpts) ([]string, error)

	// ListDistinctNodeRoles returns sorted distinct non-empty role names from
	// the roles JSONB array across all nodes matching the filter.
	ListDistinctNodeRoles(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error)

	// ListDistinctNodeTags returns sorted distinct non-empty tags from the
	// tags TEXT[] array across all nodes matching the filter.
	ListDistinctNodeTags(ctx context.Context, f datastore.NodeSnapshotFilter, opts datastore.DistinctValueOpts) ([]string, error)

	// ListNodeSnapshotsByCollectionRun returns all node snapshots captured
	// during the given collection run.
	ListNodeSnapshotsByCollectionRun(ctx context.Context, collectionRunID string) ([]datastore.NodeSnapshot, error)

	// CountStaleFreshByCollectionRun returns the total, stale, and fresh
	// node counts for the given collection run.
	CountStaleFreshByCollectionRun(ctx context.Context, collectionRunID string) (total, stale, fresh int, err error)

	// CountNodesByDeploymentVersion returns per-version deployment state
	// counts (staged, activated, converge_passing, converge_failing) for
	// nodes matching the given filter. Also returns total node count.
	CountNodesByDeploymentVersion(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.DeploymentVersionRow, int, error)

	// -----------------------------------------------------------------
	// Metric snapshots
	// -----------------------------------------------------------------

	// ListMetricSnapshotsByOrganisation returns pre-aggregated metric
	// snapshots for the given organisation and snapshot type, ordered by
	// snapshot_at descending. If limit > 0, at most limit rows are returned.
	ListMetricSnapshotsByOrganisation(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error)

	// ListMetricSnapshotsByOrganisationAndVersion returns pre-aggregated
	// metric snapshots for the given organisation, snapshot type, and target
	// Chef version, ordered by snapshot_at descending. If limit > 0, at most
	// limit rows are returned.
	ListMetricSnapshotsByOrganisationAndVersion(ctx context.Context, organisationID, snapshotType, targetChefVersion string, limit int) ([]datastore.MetricSnapshot, error)

	// ListDailyMetricSnapshotsByOrganisation returns at most one snapshot per
	// calendar day (the latest) for the given organisation and snapshot type.
	ListDailyMetricSnapshotsByOrganisation(ctx context.Context, organisationID, snapshotType string, limit int) ([]datastore.MetricSnapshot, error)

	// ListDailyMetricSnapshotsByOrganisationAndVersion returns at most one
	// snapshot per calendar day for the given organisation, snapshot type,
	// and target Chef version.
	ListDailyMetricSnapshotsByOrganisationAndVersion(ctx context.Context, organisationID, snapshotType, targetChefVersion string, limit int) ([]datastore.MetricSnapshot, error)

	// GetNodeSnapshotByName returns the most recent snapshot for a node
	// identified by organisation ID and node name. Returns
	// datastore.ErrNotFound if no such snapshot exists.
	GetNodeSnapshotByName(ctx context.Context, organisationID, nodeName string) (datastore.NodeSnapshot, error)

	// -----------------------------------------------------------------
	// Node readiness
	// -----------------------------------------------------------------

	// ListNodeReadinessForSnapshot returns all readiness records for the
	// given node snapshot, ordered by target_chef_version.
	ListNodeReadinessForSnapshot(ctx context.Context, orgName, nodeName string) ([]datastore.NodeReadiness, error)

	// ListNodeReadinessByNodeName returns the latest readiness records for
	// the given node within the specified organisation. Queries by
	// (organisation_id, node_name) rather than node_snapshot_id.
	ListNodeReadinessByNodeName(ctx context.Context, organisationName, nodeName string) ([]datastore.NodeReadiness, error)

	// BulkListNodeReadinessByNodeNames returns the latest readiness records
	// for multiple nodes within the specified organisation in a single query.
	// Results are returned as a map keyed by node_name.
	BulkListNodeReadinessByNodeNames(ctx context.Context, organisationName string, nodeNames []string) (map[string][]datastore.NodeReadiness, error)

	// CountNodeReadiness returns the total, ready, and blocked counts for
	// the given organisation and target Chef version.
	CountNodeReadiness(ctx context.Context, organisationName, targetChefVersion string) (total, ready, blocked int, err error)

	// CountNodeReadinessByStatus returns the total and per-rollup-status counts
	// (ready / needs_review / blocked) for the given organisation and target
	// Chef version.
	CountNodeReadinessByStatus(ctx context.Context, organisationName, targetChefVersion string) (total, ready, needsReview, blocked int, err error)

	// -----------------------------------------------------------------
	// Server cookbooks
	// -----------------------------------------------------------------

	// ListCookbooksFiltered returns server cookbooks matching the given
	// filter with compatibility computed via SQL JOIN. Pagination and
	// sorting are handled in SQL.
	ListCookbooksFiltered(ctx context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error)

	// ListServerCookbooksByOrganisation returns all server cookbooks
	// belonging to the given organisation.
	ListServerCookbooksByOrganisation(ctx context.Context, organisationID string) ([]datastore.ServerCookbook, error)

	// ListServerCookbooksByName returns all server cookbook versions with
	// the given name across all organisations.
	ListServerCookbooksByName(ctx context.Context, name string) ([]datastore.ServerCookbook, error)

	// ResetServerCookbookDownloadStatus resets the download_status to
	// 'pending' for a server cookbook, forcing the streaming pipeline to
	// re-download and re-scan it on the next collection cycle.
	ResetServerCookbookDownloadStatus(ctx context.Context, organisationName, name, version string) (datastore.ServerCookbook, error)

	// ResetAllServerCookbookDownloadStatuses resets download_status to
	// 'pending' for all server cookbooks with status 'ok', forcing the
	// streaming pipeline to re-download and re-scan them all.
	ResetAllServerCookbookDownloadStatuses(ctx context.Context) (int, error)

	// -----------------------------------------------------------------
	// Git repos
	// -----------------------------------------------------------------

	// ResetAllGitRepoStatuses resets all materialised status columns to
	// 'untested'. Call when the active target Chef version changes.
	ResetAllGitRepoStatuses(ctx context.Context) error

	// ListGitRepos returns all git repos, deduplicated by name (most
	// recently fetched row per name), ordered by name.
	ListGitRepos(ctx context.Context) ([]datastore.GitRepo, error)

	// ListGitReposFiltered returns git repos matching the filter with
	// SQL-level pagination. Returns the page of results and total count.
	ListGitReposFiltered(ctx context.Context, f datastore.GitRepoFilter) ([]datastore.GitRepo, int, error)

	// ListGitReposByName returns all git repo rows with the given cookbook
	// name, ordered by last_fetched_at DESC.
	ListGitReposByName(ctx context.Context, name string) ([]datastore.GitRepo, error)

	// DeleteGitReposByName removes all git repo rows for the given cookbook
	// name and deletes associated committer data. Returns
	// datastore.ErrNotFound if no git repo with that name exists.
	DeleteGitReposByName(ctx context.Context, name string) (datastore.DeleteGitRepoResult, error)

	// -----------------------------------------------------------------
	// Server cookbook analysis results
	// -----------------------------------------------------------------

	// ListServerCookbookComplexitiesByCookbook returns all complexity
	// records for the given server cookbook ID, ordered by
	// target_chef_version.
	ListServerCookbookComplexitiesByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookComplexity, error)

	// ListServerCookbookComplexitiesByOrganisation returns all complexity
	// records for server cookbooks belonging to the given organisation,
	// ordered by cookbook name, version, and target Chef version.
	ListServerCookbookComplexitiesByOrganisation(ctx context.Context, organisationID string) ([]datastore.ServerCookbookComplexity, error)

	// ListServerCookbookCookstyleResults returns all cookstyle results for
	// the given server cookbook ID, ordered by target_chef_version.
	ListServerCookbookCookstyleResults(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookCookstyleResult, error)

	// ListServerCookbookCookstyleResultsByOrganisation returns all cookstyle
	// results for server cookbooks belonging to the given organisation.
	ListServerCookbookCookstyleResultsByOrganisation(ctx context.Context, organisationID string) ([]datastore.ServerCookbookCookstyleResult, error)

	// GetServerCookbookCookstyleResult returns the cookstyle result for the
	// given server cookbook ID and target Chef version. Returns (nil, nil)
	// if no result exists.
	GetServerCookbookCookstyleResult(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error)

	// GetServerCookbookAutocorrectPreview returns the autocorrect preview
	// for the given cookstyle result ID. Returns (nil, nil) if no preview
	// exists.
	GetServerCookbookAutocorrectPreview(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookAutocorrectPreview, error)

	// DeleteServerCookbookCookstyleResultsByCookbook removes all cookstyle
	// results for the given server cookbook ID.
	DeleteServerCookbookCookstyleResultsByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error

	// DeleteServerCookbookComplexitiesByCookbook removes all complexity
	// records for the given server cookbook ID.
	DeleteServerCookbookComplexitiesByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error

	// DeleteServerCookbookAutocorrectPreviewsByCookbook removes all
	// autocorrect previews for the given server cookbook ID.
	DeleteServerCookbookAutocorrectPreviewsByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error

	// DeleteAllServerCookbookCookstyleResults removes all server cookbook
	// cookstyle results. Forces a full rescan on the next collection cycle.
	DeleteAllServerCookbookCookstyleResults(ctx context.Context) error

	// DeleteAllServerCookbookComplexities removes all server cookbook
	// complexity records.
	DeleteAllServerCookbookComplexities(ctx context.Context) error

	// DeleteAllServerCookbookAutocorrectPreviews removes all server cookbook
	// autocorrect preview records.
	DeleteAllServerCookbookAutocorrectPreviews(ctx context.Context) error

	// -----------------------------------------------------------------
	// Git repo analysis results
	// -----------------------------------------------------------------

	// ListGitRepoCookstyleResults returns all cookstyle results for the
	// given git repo ID, ordered by target_chef_version.
	ListGitRepoCookstyleResults(ctx context.Context, gitRepoName, gitRepoURL string) ([]datastore.GitRepoCookstyleResult, error)

	// ListAllGitRepoCookstyleResults returns all git repo cookstyle results,
	// ordered by target_chef_version.
	ListAllGitRepoCookstyleResults(ctx context.Context) ([]datastore.GitRepoCookstyleResult, error)

	// GetGitRepoCookstyleResult returns the cookstyle result for the given
	// git repo ID and target Chef version. Returns (nil, nil) if no result
	// exists.
	GetGitRepoCookstyleResult(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error)

	// ListGitRepoComplexitiesByRepo returns all complexity records for the
	// given git repo ID, ordered by target_chef_version.
	ListGitRepoComplexitiesByRepo(ctx context.Context, gitRepoName, gitRepoURL string) ([]datastore.GitRepoComplexity, error)

	// ListAllGitRepoComplexities returns all git repo complexity records,
	// ordered by target_chef_version.
	ListAllGitRepoComplexities(ctx context.Context) ([]datastore.GitRepoComplexity, error)

	// GetGitRepoAutocorrectPreview returns the autocorrect preview for the
	// given cookstyle result ID. Returns (nil, nil) if no preview exists.
	GetGitRepoAutocorrectPreview(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*datastore.GitRepoAutocorrectPreview, error)

	// DeleteGitRepoCookstyleResultsByRepo removes all cookstyle results for
	// the given git repo ID.
	DeleteGitRepoCookstyleResultsByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error

	// DeleteGitRepoComplexitiesByRepo removes all complexity records for
	// the given git repo ID.
	DeleteGitRepoComplexitiesByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error

	// DeleteGitRepoAutocorrectPreviewsByRepo removes all autocorrect
	// previews for the given git repo ID.
	DeleteGitRepoAutocorrectPreviewsByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error

	// DeleteAllGitRepoCookstyleResults removes all git repo cookstyle
	// results.
	DeleteAllGitRepoCookstyleResults(ctx context.Context) error

	// -----------------------------------------------------------------
	// Cookstyle re-score (bulk verdict update without full rescan)
	// -----------------------------------------------------------------

	// ListServerCookstyleResultsForRescore returns lightweight rows for
	// re-scoring server cookbook cookstyle results.
	ListServerCookstyleResultsForRescore(ctx context.Context) ([]datastore.CookstyleRescoreRow, error)

	// ListGitRepoCookstyleResultsForRescore returns lightweight rows for
	// re-scoring git repo cookstyle results.
	ListGitRepoCookstyleResultsForRescore(ctx context.Context) ([]datastore.CookstyleRescoreRow, error)

	// BatchUpdateServerCookstylePassed updates the passed column for the
	// given server cookbook cookstyle result rows.
	BatchUpdateServerCookstylePassed(ctx context.Context, updates []datastore.CookstylePassedUpdate) error

	// BatchUpdateGitRepoCookstylePassed updates the passed column for the
	// given git repo cookstyle result rows.
	BatchUpdateGitRepoCookstylePassed(ctx context.Context, updates []datastore.CookstylePassedUpdate) error

	// RecomputeGitRepoCompatibilityStatus recomputes the compatibility_status
	// for a single git repo from its latest cookstyle result.
	RecomputeGitRepoCompatibilityStatus(ctx context.Context, name, url, targetVersion string) error

	// RecomputeAllGitRepoCookstyleStatus re-materialises every git repo's
	// cookstyle/compatibility status from its latest result for the target Chef
	// version, so the materialised list columns cannot drift from the results.
	RecomputeAllGitRepoCookstyleStatus(ctx context.Context, targetChefVersion string) error

	// ResetAllGitRepoCookstyleVerdicts clears the materialised cookstyle and
	// compatibility verdicts, leaving Test Kitchen columns intact. Called when
	// cookstyle results are deleted so the list view cannot keep showing a
	// verdict whose backing rows are gone.
	ResetAllGitRepoCookstyleVerdicts(ctx context.Context) error

	// RecomputeAllRoleCompatStatus re-materialises every role's compatibility
	// columns in role_summary from cookstyle results for the target Chef version.
	RecomputeAllRoleCompatStatus(ctx context.Context, targetChefVersion string) error

	// RecomputeAllRoleTKStatus re-materialises every role's TK columns in
	// role_summary from its transitive cookbook set's git_repos.tk_status.
	RecomputeAllRoleTKStatus(ctx context.Context) error

	// ResetAllRoleStatuses blanks the active-target role_summary columns
	// (compat + tk) to defaults, preserving structural columns. Call on target
	// Chef version change.
	ResetAllRoleStatuses(ctx context.Context) error

	// DeleteAllGitRepoComplexities removes all git repo complexity records.
	DeleteAllGitRepoComplexities(ctx context.Context) error

	// DeleteAllGitRepoAutocorrectPreviews removes all git repo autocorrect
	// preview records.
	DeleteAllGitRepoAutocorrectPreviews(ctx context.Context) error

	// -----------------------------------------------------------------
	// Cookstyle violations browser
	// -----------------------------------------------------------------

	// ListAllServerCookbookCookstyleResultsByTargetVersion returns all
	// server cookbook cookstyle results for the given target Chef version,
	// across all organisations.
	ListAllServerCookbookCookstyleResultsByTargetVersion(ctx context.Context, targetChefVersion string) ([]datastore.ServerCookbookCookstyleResult, error)

	// ListGitRepoCookstyleResultsByTargetVersion returns all git repo
	// cookstyle results for a single target Chef version.
	ListGitRepoCookstyleResultsByTargetVersion(ctx context.Context, targetChefVersion string) ([]datastore.GitRepoCookstyleResult, error)

	// -----------------------------------------------------------------
	// Cop classifications
	// -----------------------------------------------------------------

	// ListCopClassifications returns all operator overrides (keyed by cop_name;
	// single active target).
	ListCopClassifications(ctx context.Context) ([]datastore.CopClassification, error)

	// UpsertCopClassification creates or updates a cop classification.
	UpsertCopClassification(ctx context.Context, copName, classification, reason, createdBy string) error

	// -----------------------------------------------------------------
	// Event ingest (converge_runs)
	// -----------------------------------------------------------------

	// BulkUpsertConvergeRuns persists a batch of normalised converge runs in one
	// transaction, deduped on (run_id, end_time). Returns the number inserted.
	BulkUpsertConvergeRuns(ctx context.Context, runs []ingest.ConvergeRun) (int, error)

	// ListConvergeRunsForNode returns a node's recent converge runs (most-recent
	// first) by delivered organisation name + node name, bounded by limit.
	ListConvergeRunsForNode(ctx context.Context, organisation, nodeName string, limit int) ([]datastore.ConvergeRunView, error)

	// ListConvergeRunNodesFiltered returns the distinct-node rollup for the Run
	// events Nodes tab (EXISTS semantics; one row per node = its latest matching
	// run) with SQL-level pagination and the total distinct-node count.
	ListConvergeRunNodesFiltered(ctx context.Context, f datastore.ConvergeRunFilter) ([]datastore.ConvergeRunListItem, int, error)

	// ListConvergeRunsFiltered returns the flat run list for the Run events Runs
	// tab (one row per run) with SQL-level pagination and the total count.
	ListConvergeRunsFiltered(ctx context.Context, f datastore.ConvergeRunFilter) ([]datastore.ConvergeRunListItem, int, error)

	// ListConvergeRunOrganisations returns the distinct delivered org names in
	// converge_runs (the org filter's option source — NOT the organisations table).
	ListConvergeRunOrganisations(ctx context.Context) ([]string, error)

	// ListConvergeRunChefVersions returns the distinct chef_version values in
	// converge_runs (the chef_version filter's option source).
	ListConvergeRunChefVersions(ctx context.Context) ([]string, error)

	// DeleteCopClassification removes an operator override.
	DeleteCopClassification(ctx context.Context, copName string) error

	// ListOffenceFingerprintsByTarget returns every stored offence fingerprint
	// row for a target version (all results), ordered by result identity then
	// scanned_at ascending — the bulk feed for the CookStyle rollup recompute
	// trend.
	ListOffenceFingerprintsByTarget(ctx context.Context, targetChefVersion string) ([]datastore.CookstyleOffenceFingerprint, error)

	// -----------------------------------------------------------------
	// Custom cop definitions
	// -----------------------------------------------------------------

	// ListCustomCopDefinitions returns all custom cop definitions.
	ListCustomCopDefinitions(ctx context.Context) ([]datastore.CustomCopDefinition, error)

	// GetCustomCopDefinition returns a custom cop definition by cop_name.
	GetCustomCopDefinition(ctx context.Context, copName string) (*datastore.CustomCopDefinition, error)

	// CreateCustomCopDefinition inserts a new custom cop and returns its ID.
	CreateCustomCopDefinition(ctx context.Context, d datastore.CustomCopDefinition) (string, error)

	// UpdateCustomCopDefinition updates an existing custom cop by cop_name.
	UpdateCustomCopDefinition(ctx context.Context, d *datastore.CustomCopDefinition) error

	// DeleteCustomCopDefinition removes a custom cop by cop_name.
	DeleteCustomCopDefinition(ctx context.Context, copName string) error

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
	GetLogEntry(ctx context.Context, id int64) (datastore.LogEntry, error)

	// -----------------------------------------------------------------
	// Role dependencies (used by dependency graph handlers)
	// -----------------------------------------------------------------

	// ListRolesFiltered returns roles matching the given filter with derived
	// compatibility status, total count, and summary counts.
	ListRolesFiltered(ctx context.Context, f datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error)

	// GetRoleCompatSummary returns aggregate compatibility counts and a
	// role_name→compat_status map for all roles matching the org and name
	// filters. CompatibilityStatus, Limit, and Offset in f are ignored.
	// Used by ListRolesFiltered and by handlers that need the full compat map.
	GetRoleCompatSummary(ctx context.Context, f datastore.RoleFilter) (datastore.RoleFilterSummary, map[string]string, error)

	// GetCookbookTKStatuses returns aggregate TK status for server cookbooks
	// that have matching git repos with Test Kitchen results.
	GetCookbookTKStatuses(ctx context.Context, cookbookNames []string, targetVersion string) (map[string]string, error)

	// GetRoleDetail returns the full detail view for a single role including
	// dependencies, blocking cookbooks, blast radius, and nested role chain.
	GetRoleDetail(ctx context.Context, roleName, targetChefVersion string) (*datastore.RoleDetail, error)

	// ListRoleDependenciesByOrg returns all dependency records for the given
	// organisation, ordered by role_name, dependency_type, dependency_name.
	ListRoleDependenciesByOrg(ctx context.Context, organisationID string) ([]datastore.RoleDependency, error)

	// ListCookbookDependenciesByOrg returns an adjacency map of cookbook name
	// to its direct dependency cookbook names for all active cookbooks in the
	// given organisation. Used to expand cookbook→cookbook transitive deps.
	ListCookbookDependenciesByOrg(ctx context.Context, orgName string) (map[string][]string, error)

	// GetCookbookComplexityMap returns complexity scores for named cookbooks.
	GetCookbookComplexityMap(ctx context.Context, org, targetChefVersion string, names []string) (map[string]int, error)

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
	GetAssignment(ctx context.Context, id int64) (datastore.OwnershipAssignment, error)

	// DeleteAssignment removes an assignment by ID. Returns
	// datastore.ErrNotFound if no such assignment exists.
	DeleteAssignment(ctx context.Context, id int64) error

	// ReassignOwnership moves assignments from one owner to another.
	// Returns the number reassigned and the number skipped (duplicates).
	ReassignOwnership(ctx context.Context, fromOwnerName, toOwnerName string, entityType, organisationName string) (reassigned, skipped int, err error)

	// MergeOwners folds one owner into another: the work moves, the
	// identities the source was known by move with it, and the source
	// owner is removed. Returns datastore.ErrNotFound if either is absent.
	MergeOwners(ctx context.Context, fromOwnerName, intoOwnerName string) (datastore.MergeOwnersResult, error)

	// ListOwnerDuplicateCandidates returns stored pairs of owners that may
	// be the same person, with the total number found.
	ListOwnerDuplicateCandidates(ctx context.Context, f datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error)

	// RecomputeOwnerDuplicateCandidates rescans the catalogue and returns
	// the number of pairs found.
	RecomputeOwnerDuplicateCandidates(ctx context.Context) (int, error)

	// GetOwnerDuplicateScan returns when the catalogue was last scanned.
	// Returns datastore.ErrNotFound if it never has been.
	GetOwnerDuplicateScan(ctx context.Context) (datastore.OwnerDuplicateScan, error)

	// CountOwnersMissingAliases returns the number of owners and how many
	// of them have no alias recorded.
	CountOwnersMissingAliases(ctx context.Context) (total, missing int, err error)

	// LookupOwnership returns the owners of a given entity, including
	// inherited ownership.
	LookupOwnership(ctx context.Context, entityType, entityKey, organisationID string) ([]datastore.OwnershipLookupResult, error)

	// -----------------------------------------------------------------
	// Owner aliases
	// -----------------------------------------------------------------

	// InsertOwnerAlias creates a new alias for an owner.
	InsertOwnerAlias(ctx context.Context, p datastore.InsertOwnerAliasParams) (datastore.OwnerAlias, error)

	// GetOwnerAliasesByOwner returns all aliases for a given owner.
	GetOwnerAliasesByOwner(ctx context.Context, ownerName string) ([]datastore.OwnerAlias, error)

	// ResolveOwnerByAlias finds the owner name for a given alias.
	ResolveOwnerByAlias(ctx context.Context, aliasType, aliasValue string) (string, error)

	// DeleteOwnerAlias removes an alias by ID.
	DeleteOwnerAlias(ctx context.Context, id string) error

	// SuggestOwnerAliases returns fuzzy match suggestions.
	SuggestOwnerAliases(ctx context.Context, input string, limit int) ([]datastore.AliasSuggestion, error)

	// -----------------------------------------------------------------
	// Discovery-driven ownership intake
	// -----------------------------------------------------------------

	// InsertImportMapping creates a saved column mapping. Returns
	// datastore.ErrAlreadyExists when the name is taken.
	InsertImportMapping(ctx context.Context, p datastore.InsertImportMappingParams) (datastore.ImportMapping, error)

	// ListImportMappings returns saved mappings without their field maps,
	// with the total count for pagination.
	ListImportMappings(ctx context.Context, limit, offset int) ([]datastore.ImportMapping, int, error)

	// GetImportMapping returns one saved mapping including its field map.
	// Returns datastore.ErrNotFound when no mapping has that id.
	GetImportMapping(ctx context.Context, id int64) (datastore.ImportMapping, error)

	// UpdateImportMapping replaces a saved mapping's name, delimiter and
	// field map.
	UpdateImportMapping(ctx context.Context, id int64, p datastore.UpdateImportMappingParams) (datastore.ImportMapping, error)

	// DeleteImportMapping removes a saved mapping.
	DeleteImportMapping(ctx context.Context, id int64) error

	// LookupAssignmentOwnersByEntity returns the assignments that already
	// exist on each of the given entity keys. Keys with no assignment are
	// absent from the map rather than present and empty.
	LookupAssignmentOwnersByEntity(ctx context.Context, entityType string, entityKeys []string) (map[string][]datastore.EntityAssignment, error)

	// EntityKeysExist reports which of the given keys name an entity CMM
	// has collected. Informational only: assignments are soft references,
	// so an absent key is reported, never rejected.
	EntityKeysExist(ctx context.Context, entityType string, keys []string) (map[string]bool, error)

	// SuggestOwnersByEmailLocalpart finds owners whose email-shaped aliases
	// share a localpart with the given one. Suggestions only — the same
	// localpart under two domains is as often two people as one.
	SuggestOwnersByEmailLocalpart(ctx context.Context, localpart string, limit int) ([]datastore.AliasSuggestion, error)

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

	// GetOwnerEmailsForGitRepo returns the set of contact_email addresses
	// for owners assigned to the given git repo URL.
	GetOwnerEmailsForGitRepo(ctx context.Context, gitRepoURL string) (map[string]bool, error)

	// -----------------------------------------------------------------
	// Ownership audit log
	// -----------------------------------------------------------------

	// InsertAuditEntry creates a new ownership audit log entry.
	InsertAuditEntry(ctx context.Context, p datastore.InsertAuditEntryParams) error

	// ListAuditLog returns audit log entries matching the given filter,
	// in reverse chronological order.
	ListAuditLog(ctx context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error)

	// InsertCookstyleAuditEntry records a CookStyle criteria-change event
	// (cop reclassification or custom-cop change) for explainability.
	InsertCookstyleAuditEntry(ctx context.Context, p datastore.InsertCookstyleAuditParams) error

	// -----------------------------------------------------------------
	// System health
	// -----------------------------------------------------------------

	// DatabaseSize returns the size of the current database in bytes.
	DatabaseSize(ctx context.Context) (int64, error)

	// DatabaseTableSizes returns per-table disk usage for all user tables
	// in the public schema, ordered by total size descending.
	DatabaseTableSizes(ctx context.Context) ([]datastore.TableSize, error)

	// -----------------------------------------------------------------
	// PostgreSQL performance stats
	// -----------------------------------------------------------------

	// PgStatStatementsAvailable returns true if the pg_stat_statements
	// extension is installed and queryable.
	PgStatStatementsAvailable(ctx context.Context) bool

	// TopQueryStats returns the top N queries by total execution time
	// from pg_stat_statements. Returns nil, nil if extension unavailable.
	TopQueryStats(ctx context.Context, limit int) ([]datastore.TopQueryStat, error)

	// TableStats returns per-table statistics from pg_stat_user_tables.
	TableStats(ctx context.Context) ([]datastore.TableStat, error)

	// IndexStats returns per-index statistics from pg_stat_user_indexes.
	IndexStats(ctx context.Context) ([]datastore.IndexStat, error)

	// ActiveQueries returns currently running queries from pg_stat_activity.
	ActiveQueries(ctx context.Context) ([]datastore.ActiveQuery, error)

	// ResetPgStats calls pg_stat_statements_reset() (if available) and
	// pg_stat_reset() to clear cumulative PostgreSQL statistics.
	ResetPgStats(ctx context.Context) error

	// VacuumFull runs VACUUM FULL to reclaim disk space from dead tuples.
	VacuumFull(ctx context.Context) error

	// ExplainCatalog returns the static list of canned EXPLAIN entries.
	ExplainCatalog() []datastore.ExplainCatalogEntry

	// ResolveCatalogExplain returns the SQL + args for a catalog key, plus a
	// label and a param summary. Returns datastore.ErrExplainUnavailable when
	// the entry needs live sample data that is absent.
	ResolveCatalogExplain(ctx context.Context, key string, p datastore.CatalogParams) (sqlText string, args []interface{}, label, paramSummary string, err error)

	// RunExplain runs EXPLAIN on the given SQL inside a read-only, timeout-bounded
	// transaction and returns the plan run(s). It never mutates data.
	RunExplain(ctx context.Context, sqlText string, args []interface{}, opts datastore.ExplainOptions) (datastore.ExplainResult, error)

	// -----------------------------------------------------------------
	// Cookbook Platform Coverage
	// -----------------------------------------------------------------

	// GetCookbookPlatformCoverage returns the platform coverage analysis
	// for the named cookbook. Returns (nil, nil) if no coverage exists.
	GetCookbookPlatformCoverage(ctx context.Context, cookbookName string) (*datastore.CookbookPlatformCoverage, error)

	// -----------------------------------------------------------------
	// Kitchen Analysis
	// -----------------------------------------------------------------

	// GetKitchenAnalysisSummary returns aggregate statistics about kitchen
	// config analysis across all repos.
	GetKitchenAnalysisSummary(ctx context.Context) (*datastore.KitchenAnalysisSummary, error)

	// ListKitchenAnalysisResults returns all kitchen analysis results.
	ListKitchenAnalysisResults(ctx context.Context) ([]datastore.KitchenAnalysisResult, error)

	// ListKitchenAnalysisResultsFiltered returns kitchen analysis results
	// matching the given filters. Empty driverName means no driver filter.
	// Nil hasLocalOverride means no override filter.
	ListKitchenAnalysisResultsFiltered(ctx context.Context, driverName string, hasLocalOverride *bool) ([]datastore.KitchenAnalysisResult, error)

	// GetKitchenAnalysisResultByName returns the kitchen analysis result for
	// the given cookbook/repo name. Returns (nil, nil) if not found.
	GetKitchenAnalysisResultByName(ctx context.Context, gitRepoName string) (*datastore.KitchenAnalysisResult, error)

	// ListDiscoveredPlatforms returns all discovered kitchen platforms
	// ordered by cookbook_count descending.
	ListDiscoveredPlatforms(ctx context.Context) ([]datastore.KitchenDiscoveredPlatform, error)

	// ListDiscoveredPlatformsFiltered returns discovered platforms matching
	// the given filters.
	ListDiscoveredPlatformsFiltered(ctx context.Context, osFamily string, minCount int) ([]datastore.KitchenDiscoveredPlatform, error)

	// -----------------------------------------------------------------
	// VM Tracking
	// -----------------------------------------------------------------

	// ListTrackedVMs returns all tracked VMs ordered by created_at DESC.
	ListTrackedVMs(ctx context.Context) ([]datastore.TrackedVM, error)

	// ListTrackedVMsFiltered returns tracked VMs with optional status filter.
	ListTrackedVMsFiltered(ctx context.Context, status string) ([]datastore.TrackedVM, error)

	// GetTrackedVM returns a tracked VM by ID. Returns (nil, nil) if not found.
	GetTrackedVM(ctx context.Context, id string) (*datastore.TrackedVM, error)

	// ListOrphanedVMs returns VMs past their TTL that are not yet destroyed.
	ListOrphanedVMs(ctx context.Context) ([]datastore.TrackedVM, error)

	// MarkVMDestroyed marks a VM as destroyed with actual_destroy_at = now().
	MarkVMDestroyed(ctx context.Context, id string) error

	// MarkVMOrphaned marks a VM as orphaned.
	MarkVMOrphaned(ctx context.Context, id string) error

	// CountTrackedVMsByStatus returns VM counts grouped by status.
	CountTrackedVMsByStatus(ctx context.Context) (map[string]int, error)

	// -----------------------------------------------------------------
	// Node Kitchen Runs
	// -----------------------------------------------------------------

	// ListNodeKitchenRuns returns all kitchen runs for the given organisation.
	ListNodeKitchenRuns(ctx context.Context, orgName string) ([]datastore.NodeKitchenRun, error)

	// ListNodeKitchenRunsByNode returns kitchen runs for a specific node.
	ListNodeKitchenRunsByNode(ctx context.Context, orgName, nodeName string) ([]datastore.NodeKitchenRun, error)

	// GetNodeKitchenRun returns a single run by ID, or (nil, nil) if not found.
	GetNodeKitchenRun(ctx context.Context, id string) (*datastore.NodeKitchenRun, error)

	// DeleteNodeKitchenRun removes a run by ID. Returns ErrNotFound if missing.
	DeleteNodeKitchenRun(ctx context.Context, id string) error

	// -----------------------------------------------------------------
	// Kitchen Batches
	// -----------------------------------------------------------------

	// CreateKitchenBatch creates a new batch definition.
	CreateKitchenBatch(ctx context.Context, p datastore.CreateKitchenBatchParams) (datastore.KitchenBatch, error)

	// GetKitchenBatch returns a batch by UUID. Returns ErrNotFound if missing.
	GetKitchenBatch(ctx context.Context, id string) (datastore.KitchenBatch, error)

	// ListKitchenBatches returns all batches ordered by created_at DESC.
	ListKitchenBatches(ctx context.Context) ([]datastore.KitchenBatch, error)

	// UpdateKitchenBatch updates a draft batch. Returns ErrNotFound if
	// the batch does not exist or is not in draft status.
	UpdateKitchenBatch(ctx context.Context, id string, p datastore.UpdateKitchenBatchParams) (datastore.KitchenBatch, error)

	// UpdateKitchenBatchStatus transitions a batch to a new status.
	UpdateKitchenBatchStatus(ctx context.Context, id string, status string, now time.Time) (datastore.KitchenBatch, error)

	// DeleteKitchenBatch removes a batch (only draft/completed/cancelled/failed).
	DeleteKitchenBatch(ctx context.Context, id string) error

	// UpdateKitchenBatchStatusIfCurrent is a CAS-style status transition.
	// Returns ErrNotFound if batch doesn't exist or status doesn't match.
	UpdateKitchenBatchStatusIfCurrent(ctx context.Context, id string, expectedStatus string, newStatus string, now time.Time) (datastore.KitchenBatch, error)

	// CancelStaleBatches transitions running/preparing batches to cancelled.
	CancelStaleBatches(ctx context.Context, now time.Time) (int, error)

	// -----------------------------------------------------------------
	// Kitchen Batch Instances
	// -----------------------------------------------------------------

	// CreateBatchInstance inserts a single batch instance.
	CreateBatchInstance(ctx context.Context, p datastore.CreateBatchInstanceParams) (datastore.KitchenBatchInstance, error)

	// CreateBatchInstances bulk-inserts batch instances in a transaction.
	CreateBatchInstances(ctx context.Context, params []datastore.CreateBatchInstanceParams) ([]datastore.KitchenBatchInstance, error)

	// ListBatchInstances returns all instances for a batch.
	ListBatchInstances(ctx context.Context, batchID string) ([]datastore.KitchenBatchInstance, error)

	// UpdateBatchInstanceStatus transitions an instance to a new status.
	UpdateBatchInstanceStatus(ctx context.Context, id string, status string, errorMessage string, now time.Time) error

	// CountBatchInstancesByStatus returns status counts for a batch.
	CountBatchInstancesByStatus(ctx context.Context, batchID string) (map[string]int, error)

	// CancelPendingBatchInstances cancels all pending instances for a batch.
	CancelPendingBatchInstances(ctx context.Context, batchID string) (int, error)

	// -----------------------------------------------------------------
	// Git Repo Kitchen Exclusions
	// -----------------------------------------------------------------

	// SetGitRepoKitchenExclusion marks a repo as excluded from kitchen testing.
	SetGitRepoKitchenExclusion(ctx context.Context, name string, reason string, excludedBy string) error

	// ClearGitRepoKitchenExclusion removes the kitchen exclusion flag.
	ClearGitRepoKitchenExclusion(ctx context.Context, name string) error

	// ListExcludedGitRepos returns all repos excluded from kitchen testing.
	ListExcludedGitRepos(ctx context.Context) ([]datastore.GitRepo, error)

	// -----------------------------------------------------------------
	// Kitchen Instance Exclusions (per suite+platform)
	// -----------------------------------------------------------------

	// CreateKitchenExclusion inserts a manual instance exclusion.
	CreateKitchenExclusion(ctx context.Context, p datastore.CreateKitchenExclusionParams) (datastore.KitchenInstanceExclusion, error)

	// ListKitchenExclusions returns exclusions, optionally filtered by repo name.
	ListKitchenExclusions(ctx context.Context, repoName string) ([]datastore.KitchenInstanceExclusion, error)

	// DeleteKitchenExclusion removes an exclusion by ID. Returns false if not found.
	DeleteKitchenExclusion(ctx context.Context, id string) (bool, error)

	// -----------------------------------------------------------------
	// Git Kitchen Results (per-instance)
	// -----------------------------------------------------------------

	// UpsertGitKitchenResult inserts or updates a per-instance kitchen result.
	UpsertGitKitchenResult(ctx context.Context, p datastore.UpsertGitKitchenResultParams) (datastore.GitKitchenResult, error)

	// GetGitKitchenResult returns a result by UUID.
	GetGitKitchenResult(ctx context.Context, id string) (datastore.GitKitchenResult, error)

	// ListGitKitchenResults returns all per-instance kitchen results.
	ListGitKitchenResults(ctx context.Context) ([]datastore.GitKitchenResult, error)

	// ListActiveGitKitchenResults returns results excluding user-excluded instances.
	ListActiveGitKitchenResults(ctx context.Context) ([]datastore.GitKitchenResult, error)

	// ListGitKitchenResultsByRepo returns results for a specific repo.
	ListGitKitchenResultsByRepo(ctx context.Context, gitRepoName string) ([]datastore.GitKitchenResult, error)

	// DeleteGitKitchenResultsByRepo removes all per-instance kitchen results for a repo.
	DeleteGitKitchenResultsByRepo(ctx context.Context, gitRepoName string) error

	// -----------------------------------------------------------------
	// Kitchen Run Queue
	// -----------------------------------------------------------------

	// EnqueueKitchenRun adds an item to the queue.
	EnqueueKitchenRun(ctx context.Context, p datastore.EnqueueKitchenRunParams) (*datastore.KitchenQueueItem, error)

	// ClaimNextKitchenRun atomically claims the highest-priority queued item.
	ClaimNextKitchenRun(ctx context.Context) (*datastore.KitchenQueueItem, error)

	// CompleteKitchenRun marks a running item as completed.
	CompleteKitchenRun(ctx context.Context, id string, output string) error

	// FailKitchenRun marks a running item as failed.
	FailKitchenRun(ctx context.Context, id string, errMsg string, output string) error

	// CancelKitchenRun transitions a queued or running item to cancelled.
	CancelKitchenRun(ctx context.Context, id string) error

	// CancelKitchenRunsByBatch cancels all queued items for a batch.
	CancelKitchenRunsByBatch(ctx context.Context, batchID string) (int64, error)

	// RetryKitchenRun re-enqueues a failed/interrupted/cancelled item.
	RetryKitchenRun(ctx context.Context, id string) (*datastore.KitchenQueueItem, error)

	// ListKitchenQueue returns queue items matching the filter.
	ListKitchenQueue(ctx context.Context, f datastore.KitchenQueueFilter) ([]datastore.KitchenQueueItem, error)

	// GetKitchenQueueItem returns a single queue item by ID.
	GetKitchenQueueItem(ctx context.Context, id string) (*datastore.KitchenQueueItem, error)

	// GetKitchenQueueStats returns counts of queued and running items.
	GetKitchenQueueStats(ctx context.Context) (*datastore.KitchenQueueStats, error)

	// MarkInterruptedKitchenRuns marks in-flight items as interrupted on startup.
	MarkInterruptedKitchenRuns(ctx context.Context) (int64, error)

	// -----------------------------------------------------------------
	// Diagnostic bundle
	// -----------------------------------------------------------------

	// ListAppliedMigrations returns all rows from schema_migrations ordered
	// by version ascending.
	ListAppliedMigrations(ctx context.Context) ([]datastore.AppliedMigration, error)

	// InventoryStats returns aggregate inventory counts across all organisations.
	// When includeNames is true, cookbook/role/git-repo names are also returned.
	InventoryStats(ctx context.Context, includeNames bool) (datastore.InventoryStatsResult, error)

	// DependencyDepthStats returns recursive role and cookbook dependency
	// depth statistics per organisation. When includeNames is true, the top
	// 10 deepest roles are also returned.
	DependencyDepthStats(ctx context.Context, includeNames bool) (datastore.DepthStatsResult, error)

	// -----------------------------------------------------------------
	// Saved filters
	// -----------------------------------------------------------------

	// ListSavedFilters returns the saved filters visible to a user — their own
	// plus every shared one — optionally narrowed to a single view.
	ListSavedFilters(ctx context.Context, f datastore.SavedFilterListFilter) ([]datastore.SavedFilter, error)

	// GetSavedFilter returns a saved filter by id. Returns datastore.ErrNotFound
	// if no such filter exists.
	GetSavedFilter(ctx context.Context, id string) (datastore.SavedFilter, error)

	// InsertSavedFilter creates a saved filter. Returns datastore.ErrAlreadyExists
	// if the owner already has one of that name on that view.
	InsertSavedFilter(ctx context.Context, p datastore.InsertSavedFilterParams) (datastore.SavedFilter, error)

	// UpdateSavedFilter applies the non-nil fields of p — a rename, a new
	// selection, and a share toggle are all the same call. Returns
	// datastore.ErrNotFound if the filter is gone, or datastore.ErrAlreadyExists
	// if a rename collides.
	UpdateSavedFilter(ctx context.Context, id string, p datastore.UpdateSavedFilterParams) (datastore.SavedFilter, error)

	// DeleteSavedFilter removes a saved filter by id. Returns
	// datastore.ErrNotFound if no such filter exists.
	DeleteSavedFilter(ctx context.Context, id string) error
}

// Compile-time assertion: *datastore.DB satisfies DataStore.
var _ DataStore = (*datastore.DB)(nil)
