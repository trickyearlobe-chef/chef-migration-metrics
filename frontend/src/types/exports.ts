// SPDX-License-Identifier: Apache-2.0

export type ExportType =
  | "nodes"
  | "cookbooks"
  | "roles"
  | "git_repos"
  | "run_events";
export type ExportFormat = "csv" | "json" | "chef_search_query";
export type ExportJobStatus =
  | "pending"
  | "processing"
  | "completed"
  | "failed"
  | "expired";

// ExportParams is the list view's own query object (the same shape passed to the
// list fetch, e.g. NodeFilterQuery). The export sends these verbatim as query
// params so it reproduces the current filtered list exactly.
export type ExportParams = Record<
  string,
  string | number | boolean | null | undefined
>;

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
