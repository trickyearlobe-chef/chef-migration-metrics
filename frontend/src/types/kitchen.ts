// SPDX-License-Identifier: Apache-2.0

export interface PlatformMapTransport {
  username: string;
  password_credential: string;
  ssh_key_credential: string;
}

export interface ImageEntry {
  name: string;
  id: string;
  driver_settings?: Record<string, unknown>;
  transport?: PlatformMapTransport | null;
  chef_download_urls?: Record<string, string>;
}

export interface PlatformMapEntry {
  kitchen_name: string;
  image: string;
  is_pattern?: boolean;
  skip?: boolean;
  transport?: PlatformMapTransport | null;
}

export interface TestKitchenConfig {
  enabled: boolean | null;
  driver: string;
  timeout_minutes: number;
  driver_settings: Record<string, unknown>;
  driver_secrets: Record<string, string>;
  image_field_name: string;
  chef_license_key_credential: string;
  images: ImageEntry[];
  platform_map: PlatformMapEntry[];
}

export interface TestKitchenConfigResponse {
  config: TestKitchenConfig;
  source: "database" | "file";
  updated_at?: string;
  updated_by?: string;
}

export interface TestKitchenConfigSaveResponse {
  config: TestKitchenConfig;
  source: string;
  updated_at?: string;
  updated_by?: string;
  warnings?: string[];
}

export interface DiscoveredPlatformStatus {
  platform_name: string;
  normalised_name: string;
  os_family: string;
  cookbook_count: number;
  transport_type: string;
  mapping_status: "mapped" | "skipped" | "unmapped";
  matched_entry_index: number;
  matched_image: string;
}

export interface HypervisorTemplate {
  id: string;
  name: string;
  guest_os?: string;
  notes?: string;
  last_modified?: string;
}

export interface PlatformMappingStatusResponse {
  discovered_platforms: DiscoveredPlatformStatus[];
  templates: HypervisorTemplate[];
  unmapped_count: number;
  skipped_count: number;
  mapped_count: number;
}

export interface NodeKitchenRun {
  id: string;
  node_name: string;
  organisation_name: string;
  target_chef_version: string;
  cookbook_source: "server" | "git" | "hybrid";
  platform_name: string;
  template_used?: string;
  run_list: string[];
  cookbook_versions: Record<string, string>;
  converge_passed: boolean | null;
  verify_passed: boolean | null;
  converge_output?: string;
  verify_output?: string;
  destroy_output?: string;
  duration_seconds?: number;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  vm_tracking_id?: string;
  created_at: string;
}

export interface NodeKitchenRunRequest {
  node_name: string;
  organisation_name: string;
  target_chef_version: string;
  cookbook_source: "server" | "git" | "hybrid";
}

export interface NodeKitchenTriggerResponse {
  status: string;
  message: string;
}

export interface BatchFilters {
  cookbook_names?: string[];
  platforms?: string[];
  exclude_cookbooks?: string[];
  has_test_suite?: boolean;
  previous_status?: string;
  target_chef_versions?: string[];
  include_excluded?: boolean;
}

export interface KitchenBatch {
  id: string;
  name: string;
  filters: BatchFilters;
  max_count: number | null;
  max_concurrent_vms: number | null;
  dry_run: boolean;
  status: "draft" | "previewing" | "running" | "completed" | "cancelled";
  created_by?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface ResolvedCookbook {
  name: string;
  git_repo_url: string;
  platforms?: string[];
  suites?: string[];
  estimated_vms: number;
}

export interface BatchEstimate {
  total_cookbooks: number;
  total_estimated_vms: number;
  per_platform?: Record<string, number>;
  cookbooks: ResolvedCookbook[];
}

export interface KitchenBatchDetail extends KitchenBatch {
  estimate?: BatchEstimate;
}

export interface KitchenBatchRequest {
  name: string;
  filters: BatchFilters;
  max_count?: number | null;
  max_concurrent_vms?: number | null;
  dry_run?: boolean;
}

export interface GitRepoExcludeRequest {
  reason: string;
  excluded_by: string;
}

export interface BatchProgress {
  passed: number;
  failed: number;
  pending: number;
  timed_out: number;
  errored: number;
  total: number;
}

export interface KitchenAnalysisCookbook {
  git_repo_name: string;
  git_repo_url: string;
  platforms: KitchenPlatformInfo[];
  suites: KitchenSuiteInfo[];
  driver_name?: string;
  provisioner_name?: string;
  has_local_override: boolean;
  error_message?: string;
}

export interface KitchenPlatformInfo {
  name: string;
  driver?: Record<string, unknown>;
}

export interface KitchenSuiteInfo {
  name: string;
  run_list?: string[];
}

export type GitKitchenInstanceStatus = "mapped" | "unmapped" | "skipped" | "excluded";

export interface GitKitchenPlannedInstance {
  instance_name: string;
  suite_name: string;
  platform_name: string;
  status: GitKitchenInstanceStatus;
  status_reason: string;
  image_name?: string;
}

export interface GitKitchenPlanResult {
  git_repo_name: string;
  git_repo_url: string;
  commit_sha: string;
  instances: GitKitchenPlannedInstance[];
  total: number;
  mapped: number;
  unmapped: number;
  skipped: number;
  excluded: number;
}

export interface GitKitchenResult {
  id: string;
  git_repo_name: string;
  git_repo_url: string;
  target_chef_version: string;
  commit_sha: string;
  platform_name: string;
  suite_name: string;
  instance_name: string;
  driver_used?: string;
  passed: boolean | null;
  timed_out: boolean;
  output?: string;
  duration_seconds?: number;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface GitKitchenRunRequest {
  git_repo_name: string;
  instance_name: string;
  target_chef_version: string;
}

export interface GitKitchenRunResponse {
  message: string;
}
