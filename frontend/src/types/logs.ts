// SPDX-License-Identifier: Apache-2.0

import type { PaginatedResponse } from "./common";

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
