// SPDX-License-Identifier: Apache-2.0

export interface VersionCount {
  version: string;
  count: number;
  percent: number;
}

export interface VersionDistributionResponse {
  total_nodes: number;
  distribution: VersionCount[];
}

export interface PlatformCount {
  platform: string;
  count: number;
  percent: number;
}

export interface PlatformDistributionResponse {
  total_nodes: number;
  distribution: PlatformCount[];
}

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

export interface ReadinessTrendPoint {
  organisation_name: string;
  collection_run_org: string;
  completed_at: string;
  target_chef_version: string;
  total_nodes: number;
  ready_nodes: number;
  blocked_nodes: number;
  ready_percent: number;
}

export interface ReadinessTrendResponse {
  data: ReadinessTrendPoint[];
}

export interface ComplexityTrendPoint {
  organisation_name: string;
  collection_run_org: string;
  completed_at: string;
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

export interface StaleTrendPoint {
  organisation_name: string;
  collection_run_id: string;
  completed_at: string;
  total_nodes: number;
  stale_nodes: number;
  fresh_nodes: number;
  warning_nodes?: number;
  critical_nodes?: number;
}

export interface StaleTrendResponse {
  data: StaleTrendPoint[];
}

export interface CookbookCompatibilitySummary {
  target_chef_version: string;
  total_cookbooks: number;
  compatible_cookbooks: number;
  incompatible_cookbooks: number;
  untested_cookbooks: number;
  untested_inactive_cookbooks: number;
  untested_unscanned_cookbooks: number;
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
  untested_clone_failed_repos: number;
  untested_pending_scan_repos: number;
  compatible_percent: number;
}

export interface GitRepoCompatibilityResponse {
  data: GitRepoCompatibilitySummary[];
}

export interface TestKitchenCompatibilitySummary {
  target_chef_version: string;
  total_repos: number;
  passed_repos: number;
  failed_repos: number;
  timed_out_repos: number;
  untested_repos: number;
  untested_clone_failed_repos: number;
  untested_pending_scan_repos: number;
  passed_percent: number;
}

export interface TestKitchenCompatibilityResponse {
  data: TestKitchenCompatibilitySummary[];
}

export interface SystemHealthAlert {
  level: "warning" | "critical";
  metric: "disk" | "cpu" | "memory";
  message: string;
}

export interface SystemHealthThresholds {
  disk_used_warning_percent: number;
  disk_used_critical_percent: number;
  cpu_load_warning_per_cpu: number;
  cpu_load_critical_per_cpu: number;
  mem_used_warning_percent: number;
  mem_used_critical_percent: number;
}

export interface TableSize {
  table_name: string;
  total_bytes: number;
  table_bytes: number;
  index_bytes: number;
  row_estimate: number;
}

export interface DiskStats {
  path: string;
  total_bytes: number;
  free_bytes: number;
  used_percent: number;
}

export interface SystemHealthResponse {
  timestamp: string;
  uptime: string;
  disks: DiskStats[];
  cpu_count: number;
  load_avg_1: number;
  load_per_cpu: number;
  mem_total_bytes: number;
  mem_avail_bytes: number;
  mem_used_percent: number;
  go_heap_bytes: number;
  go_goroutines: number;
  database_size_bytes: number;
  table_sizes: TableSize[];
  alerts: SystemHealthAlert[];
  collection_paused: boolean;
  thresholds: SystemHealthThresholds;
}

export interface EndpointStat {
  method: string;
  path: string;
  count: number;
  error_count: number;
  p50_ms: number;
  p95_ms: number;
  p99_ms: number;
  max_ms: number;
}

export interface PerformanceResponse {
  window_seconds: number;
  endpoints: EndpointStat[];
}

export interface TopQueryStat {
  query: string;
  calls: number;
  total_time_ms: number;
  mean_time_ms: number;
  min_time_ms: number;
  max_time_ms: number;
  rows: number;
  shared_blks_hit: number;
  shared_blks_read: number;
}

export interface PgTableStat {
  table_name: string;
  seq_scan: number;
  seq_tup_read: number;
  idx_scan: number;
  idx_tup_fetch: number;
  n_live_tup: number;
  n_dead_tup: number;
  last_vacuum: string | null;
  last_analyze: string | null;
}

export interface PgIndexStat {
  table_name: string;
  index_name: string;
  idx_scan: number;
  idx_tup_read: number;
  idx_tup_fetch: number;
  size_bytes: number;
}

export interface PgActiveQuery {
  pid: number;
  state: string;
  query: string;
  duration_ms: number;
  wait_event_type: string | null;
  wait_event: string | null;
}

export interface PerformanceDBResponse {
  pg_stat_statements_available: boolean;
  top_queries: TopQueryStat[];
  table_stats: PgTableStat[];
  index_stats: PgIndexStat[];
  active_queries: PgActiveQuery[];
}
