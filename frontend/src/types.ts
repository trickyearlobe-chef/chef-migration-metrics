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

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

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
  collected_at: string;
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
  ohai_time: string;
  is_stale: boolean;
  collected_at: string;
  created_at: string;
}

export interface NodeReadiness {
  id: string;
  node_snapshot_id: string;
  target_chef_version: string;
  ready: boolean;
  blocking_cookbooks: string[] | null;
  blocking_reasons: string[] | null;
  created_at: string;
}

export interface NodeDetailResponse {
  node: NodeSnapshot;
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
  source: string;
  has_test_suite: boolean;
  is_active: boolean;
  is_stale_cookbook: boolean;
  download_status: string;
}

export type CookbookListResponse = PaginatedResponse<CookbookListItem>;

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

export interface CookbookVersionDetail {
  cookbook: CookbookListItem;
  complexity?: CookbookComplexity[];
  cookstyle?: CookstyleResult[];
}

export interface CookbookDetailResponse {
  name: string;
  data: CookbookVersionDetail[];
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
