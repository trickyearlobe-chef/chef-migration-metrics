// SPDX-License-Identifier: Apache-2.0

export type ExportType =
  | "ready_nodes"
  | "blocked_nodes"
  | "cookbook_remediation";
export type ExportFormat = "csv" | "json" | "chef_search_query";
export type ExportJobStatus =
  | "pending"
  | "processing"
  | "completed"
  | "failed"
  | "expired";

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
