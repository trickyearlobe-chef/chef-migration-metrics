// ---------------------------------------------------------------------------
// TypeScript types matching the Go backend API response shapes.
//
// These are derived from the handler response structs in:
//   internal/webapi/handle_*.go
//   internal/webapi/response.go
// and the JSON contract in:
//   .claude/specifications/web-api/Specification.md
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Generic pagination
// ---------------------------------------------------------------------------

export interface Pagination {
  page: number;
  per_page: number;
  total_items: number;
  total_pages: number;
}

export interface PaginatedResponse<T> {
  data: T[];
  pagination: Pagination;
}

// ---------------------------------------------------------------------------
// Error
// ---------------------------------------------------------------------------

export interface ApiError {
  error: string;
  message: string;
}

// ---------------------------------------------------------------------------
// Health & version
// ---------------------------------------------------------------------------

export interface HealthResponse {
  status: "healthy" | "unhealthy";
  version: string;
  websocket_enabled: boolean;
  websocket_clients: number;
  error?: string;
}

export interface VersionResponse {
  version: string;
}

// ---------------------------------------------------------------------------
// Organisations
// ---------------------------------------------------------------------------

export interface Organisation {
  name: string;
  chef_server_url: string;
  org_name: string;
  client_name: string;
  credential_source: string;
  source: string;
  node_count: number;
  last_collected_at?: string;
  last_collection_status?: string;
}

export interface OrganisationsResponse {
  data: Organisation[];
}

// ---------------------------------------------------------------------------
// Dashboard — Version Distribution
// ---------------------------------------------------------------------------

export interface VersionCount {
  version: string;
  count: number;
  percent: number;
}

export interface VersionDistributionResponse {
  total_nodes: number;
  distribution: VersionCount[];
}

// ---------------------------------------------------------------------------
// Dashboard — Platform Distribution
// ---------------------------------------------------------------------------

export interface PlatformCount {
  platform: string;
  count: number;
  percent: number;
}

export interface PlatformDistributionResponse {
  total_nodes: number;
  distribution: PlatformCount[];
}

// ---------------------------------------------------------------------------
// Dashboard — Version Distribution Trend
// ---------------------------------------------------------------------------

export interface VersionDistributionTrendPoint {
  organisation_name: string;
  collection_run_id: string;
  completed_at: string;
  total_nodes: number;
  distribution: Record<string, number>;
}

export interface VersionDistributionTrendResponse {
  data: VersionDistributionTrendPoint[];
}

// ---------------------------------------------------------------------------
// Dashboard — Readiness
// ---------------------------------------------------------------------------

export interface ReadinessSummary {
  target_chef_version: string;
  total_nodes: number;
  ready_nodes: number;
  blocked_nodes: number;
  ready_percent: number;
}

export interface ReadinessResponse {
  data: ReadinessSummary[];
}

// ---------------------------------------------------------------------------
// Dashboard — Readiness Trend
// ---------------------------------------------------------------------------

export interface ReadinessTrendPoint {
  organisation_name: string;
  target_chef_version: string;
  total_nodes: number;
  ready_nodes: number;
  blocked_nodes: number;
  ready_percent: number;
}

export interface ReadinessTrendResponse {
  data: ReadinessTrendPoint[];
}

// ---------------------------------------------------------------------------
// Dashboard — Complexity Trend
// ---------------------------------------------------------------------------

export interface ComplexityTrendPoint {
  organisation_name: string;
  target_chef_version: string;
  total_cookbooks: number;
  total_score: number;
  average_score: number;
  low_count: number;
  medium_count: number;
  high_count: number;
  critical_count: number;
}

export interface ComplexityTrendResponse {
  data: ComplexityTrendPoint[];
}

// ---------------------------------------------------------------------------
// Dashboard — Stale Node Trend
// ---------------------------------------------------------------------------

export interface StaleTrendPoint {
  organisation_name: string;
  collection_run_id: string;
  completed_at: string;
  total_nodes: number;
  stale_nodes: number;
  fresh_nodes: number;
}

export interface StaleTrendResponse {
  data: StaleTrendPoint[];
}

// ---------------------------------------------------------------------------
// Dashboard — Cookbook Compatibility
// ---------------------------------------------------------------------------

export interface CookbookCompatibilitySummary {
  target_chef_version: string;
  total_cookbooks: number;
  compatible_cookbooks: number;
  incompatible_cookbooks: number;
  untested_cookbooks: number;
  compatible_percent: number;
}

export interface CookbookCompatibilityResponse {
  data: CookbookCompatibilitySummary[];
}

export interface GitRepoCompatibilitySummary {
  target_chef_version: string;
  total_repos: number;
  compatible_repos: number;
  incompatible_repos: number;
  untested_repos: number;
  compatible_percent: number;
}

export interface GitRepoCompatibilityResponse {
  data: GitRepoCompatibilitySummary[];
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

export interface NodeReadinessSummary {
  target_chef_version: string;
  is_ready: boolean;
  all_cookbooks_compatible: boolean;
  sufficient_disk_space: boolean | null;
  blocking_cookbook_count: number;
  stale_data: boolean;
}

export interface NodeListItem {
  id: string;
  organisation_id: string;
  organisation_name: string;
  node_name: string;
  chef_environment?: string;
  chef_version?: string;
  platform?: string;
  platform_version?: string;
  platform_family?: string;
  policy_name?: string;
  policy_group?: string;
  is_stale: boolean;
  ohai_time?: number;
  collected_at: string;
  readiness?: NodeReadinessSummary[];
}

export type NodeListResponse = PaginatedResponse<NodeListItem>;

// ---------------------------------------------------------------------------
// Node detail
// ---------------------------------------------------------------------------

export interface NodeSnapshot {
  id: string;
  collection_run_id: string;
  organisation_id: string;
  node_name: string;
  chef_environment: string;
  chef_version: string;
  platform: string;
  platform_version: string;
  platform_family: string;
  filesystem: Record<string, unknown> | null;
  cookbooks: Record<string, unknown> | null;
  run_list: string[] | null;
  roles: string[] | null;
  policy_name: string;
  policy_group: string;
  ohai_time: number;
  is_stale: boolean;
  collected_at: string;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Per-source cookbook compatibility verdict (from multi-source readiness evaluation)
// ---------------------------------------------------------------------------

export interface CookbookSourceVerdict {
  source: string;           // "server_cookstyle", "git_cookstyle", "git_test_kitchen"
  status: string;           // "compatible", "incompatible", "untested"
  version?: string;         // server version or "HEAD" for git
  commit_sha?: string;      // git HEAD SHA (git sources only)
  complexity_score?: number;
  complexity_label?: string;
}

export interface BlockingCookbook {
  name: string;
  version: string;
  reason: string;           // "incompatible" or "untested"
  source: string;           // primary source (backward compat)
  complexity_score: number;
  complexity_label: string;
  verdicts?: CookbookSourceVerdict[];
}

export interface NodeReadiness {
  id: string;
  node_snapshot_id: string;
  organisation_id: string;
  node_name: string;
  target_chef_version: string;
  is_ready: boolean;
  all_cookbooks_compatible: boolean;
  sufficient_disk_space: boolean | null;
  blocking_cookbooks: BlockingCookbook[] | null;
  available_disk_mb: number | null;
  required_disk_mb: number | null;
  stale_data: boolean;
  evaluated_at: string;
  created_at: string;
  updated_at: string;
}

export interface NodeDetailResponse {
  node: NodeSnapshot;
  organisation_name: string;
  readiness: NodeReadiness[] | null;
}

// ---------------------------------------------------------------------------
// Nodes by version / by cookbook
// ---------------------------------------------------------------------------

export interface NodesByVersionResponse {
  chef_version: string;
  total: number;
  data: NodeSnapshot[];
}

export interface NodeWithOrg {
  organisation_name: string;
  node: NodeSnapshot;
}

export interface NodesByCookbookResponse {
  cookbook_name: string;
  total: number;
  data: NodeWithOrg[];
}

// ---------------------------------------------------------------------------
// Cookbooks
// ---------------------------------------------------------------------------

export interface CookbookListItem {
  id: string;
  organisation_id?: string;
  name: string;
  version?: string;
  version_count?: number;
  source?: string;
  has_test_suite: boolean;
  is_active: boolean;
  is_stale_cookbook: boolean;
  is_frozen?: boolean;
  download_status: string;
  download_error?: string;
  compatibility?: CompatibilityStatus;
  target_chef_version?: string;
  maintainer?: string;
  description?: string;
  long_description?: string;
  license?: string;
  platforms?: Record<string, string>;
  dependencies?: Record<string, string>;
  first_seen_at?: string;
  last_fetched_at?: string;
  created_at?: string;
  updated_at?: string;
}

export type CookbookListResponse = PaginatedResponse<CookbookListItem>;

export interface GitRepoListItem {
  id: string;
  name: string;
  git_repo_url: string;
  head_commit_sha?: string;
  default_branch?: string;
  has_test_suite: boolean;
  last_fetched_at?: string;
  compatibility?: CompatibilityStatus;
  target_chef_version?: string;
}

export type GitRepoListResponse = PaginatedResponse<GitRepoListItem>;

// ---------------------------------------------------------------------------
// Cookbook detail
// ---------------------------------------------------------------------------

export interface CookbookComplexity {
  id: string;
  cookbook_id: string;
  target_chef_version: string;
  complexity_score: number;
  complexity_label: string;
  auto_correctable_count: number;
  manual_fix_count: number;
  error_count: number;
  deprecation_count: number;
  correctness_count: number;
  modernize_count: number;
  created_at: string;
}

export interface CookstyleResult {
  id: string;
  cookbook_id: string;
  target_chef_version: string;
  passed: boolean;
  offence_count: number;
  deprecation_count: number;
  scanned_at: string;
  created_at: string;
}

export interface TestKitchenResult {
  id: string;
  cookbook_id: string;
  target_chef_version: string;
  commit_sha: string;
  converge_passed: boolean;
  tests_passed: boolean;
  compatible: boolean;
  timed_out: boolean;
  driver_used: string;
  platform_tested: string;
  duration_seconds: number;
  started_at: string;
  completed_at: string;
  created_at: string;
}

export interface CookbookVersionDetail {
  cookbook: CookbookListItem;
  cookstyle?: CookstyleResult[];
  test_kitchen?: TestKitchenResult[];
}

export interface ServerCookbookVersionDetail {
  cookbook: CookbookListItem;
  cookstyle?: CookstyleResult[];
}

export interface GitRepoDetail {
  git_repo: GitRepoListItem;
  cookstyle?: CookstyleResult[];
  test_kitchen?: TestKitchenResult[];
  complexity?: CookbookComplexity[];
}

export interface CookbookDetailResponse {
  name: string;
  server_cookbooks: ServerCookbookVersionDetail[];
  git_repos: GitRepoDetail[];
}

export interface GitRepoDetailResponse {
  name: string;
  git_repos: GitRepoDetail[];
}

// ---------------------------------------------------------------------------
// Remediation
// ---------------------------------------------------------------------------

export interface RemediationPriorityItem {
  cookbook_name: string;
  cookbook_version?: string;
  cookbook_id: string;
  organisation_id?: string;
  complexity_score: number;
  complexity_label: ComplexityLabel | string;
  affected_node_count: number;
  affected_role_count: number;
  priority_score: number;
  auto_correctable_count: number;
  manual_fix_count: number;
  deprecation_count: number;
  error_count: number;
  target_chef_version: string;
  version_count: number;
}

export interface RemediationPriorityResponse {
  target_chef_version: string;
  total_cookbooks: number;
  total_auto_correctable: number;
  total_manual_fix: number;
  total_deprecations: number;
  total_errors: number;
  data: RemediationPriorityItem[];
  pagination: Pagination;
}

export interface RemediationSummaryResponse {
  target_chef_version: string;
  total_cookbooks_evaluated: number;
  total_needing_remediation: number;
  quick_wins: number;
  manual_fixes: number;
  blocked_nodes_by_complexity: number;
  blocked_nodes_by_readiness: number;
  total_auto_correctable: number;
  total_manual_fix: number;
}

// ---------------------------------------------------------------------------
// Filters
// ---------------------------------------------------------------------------

export interface FilterStringResponse {
  data: string[];
}

export interface PlatformFilter {
  platform: string;
  versions: string[];
}

export interface FilterPlatformsResponse {
  data: PlatformFilter[];
}

// ---------------------------------------------------------------------------
// Compatibility status helpers (derived from spec)
// ---------------------------------------------------------------------------

export type CompatibilityStatus =
  | "compatible"
  | "incompatible"
  | "cookstyle_only"
  | "untested";

export type ConfidenceLevel = "high" | "medium" | null;

export type ComplexityLabel = "low" | "medium" | "high" | "critical";

// ---------------------------------------------------------------------------
// Cookbook Remediation Detail
// ---------------------------------------------------------------------------

export interface RemediationOffenseLocation {
  file: string;
  start_line: number;
  start_column: number;
  last_line: number;
  last_column: number;
}

export interface RemediationOffense {
  cop_name: string;
  severity: string;
  message: string;
  correctable: boolean;
  location: RemediationOffenseLocation;
}

export interface CopRemediation {
  cop_name: string;
  description: string;
  migration_url: string;
  introduced_in?: string;
  removed_in?: string;
  replacement_pattern?: string;
}

export interface OffenseGroup {
  cop_name: string;
  severity: string;
  count: number;
  correctable_count: number;
  remediation?: CopRemediation | null;
  offenses: RemediationOffense[];
}

export interface AutocorrectPreview {
  available: boolean;
  total_offenses: number;
  correctable_offenses: number;
  remaining_offenses: number;
  files_modified: number;
  diff_output: string;
  generated_at?: string;
}

export interface RemediationStatistics {
  total_offenses: number;
  correctable_offenses: number;
  remaining_offenses: number;
  auto_correctable_count: number;
  manual_fix_count: number;
  deprecation_count: number;
  error_count: number;
  offense_groups: number;
}

// ---------------------------------------------------------------------------
// Dependency Graph
// ---------------------------------------------------------------------------

export interface DependencyGraphNode {
  id: string;
  name: string;
  type: "role" | "cookbook";
}

export interface DependencyGraphEdge {
  source: string;
  target: string;
  dependency_type: "role" | "cookbook";
}

export interface DependencyGraphSummary {
  total_nodes: number;
  total_edges: number;
  role_count: number;
  cookbook_count: number;
}

export interface DependencyGraphResponse {
  organisation: string;
  summary: DependencyGraphSummary;
  nodes: DependencyGraphNode[];
  edges: DependencyGraphEdge[];
}

export interface DependencyEntry {
  name: string;
  type: "role" | "cookbook";
}

export interface DependencyTableRow {
  role_name: string;
  cookbook_count: number;
  role_count: number;
  total_dependencies: number;
  depended_on_by: number;
  dependencies: DependencyEntry[];
}

export interface SharedCookbook {
  cookbook_name: string;
  role_count: number;
}

export interface DependencyGraphTableResponse {
  organisation: string;
  total_roles: number;
  shared_cookbooks: SharedCookbook[];
  data: DependencyTableRow[];
  pagination: Pagination;
}

// ---------------------------------------------------------------------------
// Logs
// ---------------------------------------------------------------------------

export interface LogEntry {
  id: string;
  timestamp: string;
  severity: string;
  scope: string;
  message: string;
  organisation?: string;
  cookbook_name?: string;
  cookbook_version?: string;
  commit_sha?: string;
  chef_client_version?: string;
  process_output?: string;
  collection_run_id?: string;
  notification_channel?: string;
  export_job_id?: string;
  tls_domain?: string;
  created_at: string;
}

export type LogListResponse = PaginatedResponse<LogEntry>;

export interface CollectionRunWithOrg {
  organisation_name: string;
  run: CollectionRun;
}

export interface CollectionRun {
  id: string;
  organisation_id: string;
  status: string;
  started_at: string;
  completed_at?: string;
  total_nodes?: number;
  nodes_collected?: number;
  checkpoint_start?: number;
  error_message?: string;
  created_at: string;
}

export type CollectionRunListResponse = PaginatedResponse<CollectionRunWithOrg>;

// ---------------------------------------------------------------------------
// Cookbook Remediation Detail
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Data Exports
// ---------------------------------------------------------------------------

export type ExportType = "ready_nodes" | "blocked_nodes" | "cookbook_remediation";
export type ExportFormat = "csv" | "json" | "chef_search_query";
export type ExportJobStatus = "pending" | "processing" | "completed" | "failed" | "expired";

export interface ExportFilters {
  organisation?: string;
  node_name?: string;
  environment?: string;
  platform?: string;
  chef_version?: string;
  policy_name?: string;
  policy_group?: string;
  role?: string;
  stale?: string;
  target_chef_version?: string;
  complexity_label?: string;
}

export interface ExportRequest {
  export_type: ExportType;
  format: ExportFormat;
  target_chef_version?: string;
  filters: ExportFilters;
}

export interface ExportJobResponse {
  job_id: string;
  export_type: ExportType;
  format: ExportFormat;
  status: ExportJobStatus;
  row_count?: number;
  file_size_bytes?: number;
  download_url?: string;
  error_message?: string;
  requested_at: string;
  completed_at?: string;
  expires_at?: string;
  message?: string;
}

// ---------------------------------------------------------------------------
// Cookbook Remediation Detail
// ---------------------------------------------------------------------------

export interface CookbookRemediationResponse {
  cookbook_name: string;
  cookbook_version: string;
  target_chef_version: string;
  complexity_score: number;
  complexity_label: ComplexityLabel | string;
  cookstyle_passed: boolean | null;
  scanned_at: string;
  statistics: RemediationStatistics;
  offense_groups: OffenseGroup[];
  autocorrect_preview: AutocorrectPreview;
}

export interface GitRepoRemediationResponse {
  git_repo_name: string;
  version: string;
  target_chef_version: string;
  source: string;
  complexity_score: number;
  complexity_label: ComplexityLabel | string;
  cookstyle_passed: boolean | null;
  scanned_at: string;
  statistics: RemediationStatistics;
  offense_groups: OffenseGroup[];
  autocorrect_preview: AutocorrectPreview;
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

/** POST /api/v1/auth/login request body. */
export interface LoginRequest {
  username: string;
  password: string;
}

/** POST /api/v1/auth/login success response. */
export interface LoginResponse {
  token: string;
  expires_at: string;
  user: LoginUserInfo;
}

export interface LoginUserInfo {
  username: string;
  display_name: string;
  role: string;
}

/** GET /api/v1/auth/me response. */
export interface MeResponse {
  username: string;
  display_name: string;
  email?: string;
  role: string;
  provider: string;
}

// ---------------------------------------------------------------------------
// Admin user management
// ---------------------------------------------------------------------------

export interface AdminUser {
  username: string;
  display_name: string;
  email?: string;
  role: string;
  provider: string;
  locked: boolean;
  created_at: string;
  last_login_at?: string | null;
}

export type AdminUserListResponse = PaginatedResponse<AdminUser>;

export interface CreateUserRequest {
  username: string;
  display_name?: string;
  email?: string;
  password: string;
  role?: string;
}

export interface UpdateUserRequest {
  display_name?: string;
  email?: string;
  role?: string;
  locked?: boolean;
}

export interface ResetPasswordRequest {
  password: string;
}

// ---------------------------------------------------------------------------
// Ownership
// ---------------------------------------------------------------------------

export type OwnerType = "team" | "individual" | "business_unit" | "cost_centre" | "custom";
export type EntityType = "node" | "cookbook" | "git_repo" | "role" | "policy";
export type AssignmentSource = "manual" | "auto_rule" | "import";
export type OwnershipConfidence = "definitive" | "inferred";

export interface AssignmentCounts {
  node: number;
  cookbook: number;
  git_repo: number;
  role: number;
  policy: number;
}

export interface OwnerReadiness {
  target_chef_version: string;
  total_nodes: number;
  ready: number;
  blocked: number;
  stale: number;
}

export interface Owner {
  name: string;
  display_name?: string;
  contact_email?: string;
  contact_channel?: string;
  owner_type: OwnerType;
  metadata?: Record<string, unknown>;
  assignment_counts?: AssignmentCounts;
  readiness?: OwnerReadiness;
  created_at: string;
  updated_at: string;
}

export interface OwnerDetail extends Owner {
  readiness_summary?: OwnerReadinessSummary;
  cookbook_summary?: OwnerCookbookSummary;
  git_repo_summary?: OwnerGitRepoSummary;
}

export interface OwnerReadinessSummary {
  target_chef_version: string;
  total_nodes: number;
  ready_nodes: number;
  blocked_nodes: number;
  blocking_cookbooks: string[];
}

export interface OwnerCookbookSummary {
  total_cookbooks: number;
  compatible: number;
  incompatible: number;
  untested: number;
}

export interface OwnerGitRepoSummary {
  total_repos: number;
  compatible: number;
  incompatible: number;
}

export interface OwnershipAssignment {
  id: string;
  owner_id: string;
  owner_name: string;
  entity_type: EntityType;
  entity_key: string;
  organisation_id?: string;
  organisation_name?: string;
  assignment_source: AssignmentSource;
  auto_rule_name?: string;
  confidence: OwnershipConfidence;
  notes?: string;
  created_at: string;
  updated_at: string;
}

export interface OwnershipLookupResult {
  owner_name: string;
  owner_type: OwnerType;
  assignment_source: AssignmentSource;
  confidence: OwnershipConfidence;
  resolution: string;
}

export interface OwnershipLookupResponse {
  entity_type: string;
  entity_key: string;
  organisation: string;
  owners: OwnershipLookupResult[];
}

export interface OwnershipAuditEntry {
  id: string;
  timestamp: string;
  action: string;
  actor: string;
  owner_name: string;
  entity_type?: string;
  entity_key?: string;
  organisation?: string;
  details?: Record<string, unknown>;
}

export interface ReassignResponse {
  reassigned: number;
  skipped: number;
  from_owner: string;
  to_owner: string;
  source_owner_deleted: boolean;
}

export interface ImportResponse {
  imported: number;
  skipped: number;
  errors: ImportError[];
}

export interface ImportError {
  line: number;
  error: string;
}

export interface GitRepoCommitter {
  id: string;
  git_repo_url: string;
  author_name: string;
  author_email: string;
  commit_count: number;
  first_commit_at: string;
  last_commit_at: string;
  collected_at: string;
  is_owner?: boolean;
}

export interface CookbookCommittersResponse {
  cookbook_name: string;
  git_repo_url: string;
  data: GitRepoCommitter[];
  pagination: Pagination;
}

export interface CommitterAssignResponse {
  owners_created: number;
  assignments_created: number;
  skipped: number;
}

export interface ResetGitCookbookResponse {
  cookbook_name?: string;
  git_repo_name?: string;
  repos_deleted: number;
  committers_deleted: number;
  repo_urls_removed: string[];
  local_clone_removed: boolean;
  message: string;
}

export type OwnerListResponse = PaginatedResponse<Owner>;
export type AssignmentListResponse = PaginatedResponse<OwnershipAssignment>;
export type AuditLogResponse = PaginatedResponse<OwnershipAuditEntry>;
