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
  install_method?: "download" | "baked_in";
  chef_client_path?: string;
  /**
   * Opt in to the best-effort IP-release pre_destroy hook (default off).
   * Spike — enable only where confirmed to release the DHCP lease without
   * abending the run. The VM start-rate limiter, not this, is the guarantee.
   */
  release_ip_on_destroy?: boolean;
}

export interface PlatformMapEntry {
  kitchen_name: string;
  image: string;
  is_pattern?: boolean;
  skip?: boolean;
  transport?: PlatformMapTransport | null;
}

/**
 * Opt-in repo-provided setup scripts inlined into a remote: pre_converge
 * lifecycle hook before converge (e.g. user creation). Patterns are glob
 * patterns matched against repo file paths, scoped per OS family. These hooks
 * MUST fail the run on a non-zero exit — the cookbook depends on them.
 */
export interface SetupScriptsConfig {
  linux?: string[];
  windows?: string[];
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
  /** VM start-rate limiter window in minutes (= DHCP lease time). 0 disables. */
  start_rate_window_minutes: number;
  /** Max VM starts per window (= usable DHCP pool size). 0 disables. */
  start_rate_max_per_window: number;
  /** Opt-in setup scripts inlined into pre_converge, per OS family. */
  setup_scripts?: SetupScriptsConfig | null;
}

export interface DiscoveredPlatformStatus {
  platform_name: string;
  display_name?: string | null;
  normalised_name: string;
  os_family: string;
  cookbook_count: number;
  node_count: number;
  source: "kitchen" | "nodes" | "both";
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

export interface HypervisorTestConnectionResponse {
  status: "ok" | "error" | "not_configured";
  message?: string;
  hypervisor_type?: string;
  template_count?: number;
  templates?: HypervisorTemplate[];
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
  dry_run: boolean;
  status: "draft" | "previewing" | "preparing" | "running" | "completed" | "cancelled" | "failed";
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
  planning_status?: string;
  planning_note?: string;
  total_instances?: number;
  unmapped?: number;
  skipped?: number;
  excluded?: number;
  user_excluded?: number;
}

export interface BatchEstimate {
  total_cookbooks: number;
  total_estimated_vms: number;
  skipped_cookbooks?: number;
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
  dry_run?: boolean;
}

export interface GitRepoExcludeRequest {
  reason: string;
  excluded_by: string;
}

export interface KitchenBatchInstance {
  id: string;
  batch_id: string;
  git_repo_name: string;
  git_repo_url: string;
  instance_name: string;
  platform_name: string;
  suite_name: string;
  target_chef_version: string;
  status: string;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface BatchProgress {
  passed: number;
  failed: number;
  pending: number;
  timed_out: number;
  network_timeout: number;
  errored: number;
  total: number;
}

export interface SweepResult {
  scanned: number;
  destroyed: number;
  skipped_too_young: number;
  skipped_unparsed: number;
  errors: number;
  dry_run: boolean;
  details: SweepDetail[];
}

export interface SweepDetail {
  vm_name: string;
  hypervisor_id: string;
  age_seconds: number;
  action: string;
  error?: string;
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

export type GitKitchenInstanceStatus = "mapped" | "unmapped" | "skipped" | "excluded" | "user_excluded";

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
  user_excluded: number;
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

export interface GitKitchenRunAllRequest {
  git_repo_name: string;
  target_chef_version: string;
}

export interface GitKitchenRunResponse {
  message: string;
  queue_id?: string;
  status?: string;
}

export interface GitKitchenRunAllResponse {
  message: string;
  instance_count?: number;
  queued_count?: number;
  skipped_count?: number;
  queue_ids?: string[];
}

export interface KitchenInstanceExclusion {
  id: string;
  git_repo_name: string;
  git_repo_url: string;
  suite_name: string;
  platform_name: string;
  reason: string;
  excluded_by: string;
  created_at: string;
}

export interface CreateKitchenExclusionRequest {
  git_repo_name: string;
  git_repo_url: string;
  suite_name: string;
  platform_name: string;
  reason: string;
}

export interface KitchenQueueItem {
  id: string;
  run_type: "git" | "node";
  git_repo_name?: string;
  git_repo_url?: string;
  suite_name?: string;
  platform_name?: string;
  instance_name?: string;
  target_chef_version: string;
  head_commit_sha?: string;
  node_name?: string;
  organisation_name?: string;
  cookbook_source?: string;
  batch_id?: string;
  priority: number;
  status: "queued" | "running" | "completed" | "failed" | "cancelled" | "interrupted";
  enqueued_at: string;
  started_at?: string;
  completed_at?: string;
  error_message?: string;
  output?: string;
  retry_of?: string;
}

export interface KitchenQueueStats {
  queued: number;
  running: number;
  workers_active: number;
}

export interface KitchenQueueListResponse {
  items: KitchenQueueItem[];
  stats: KitchenQueueStats;
}

export interface KitchenQueueEnqueueResponse {
  message: string;
  queue_id: string;
  status: string;
}

export interface KitchenQueueRunAllResponse {
  message: string;
  queued_count: number;
  skipped_count: number;
  queue_ids: string[];
}
