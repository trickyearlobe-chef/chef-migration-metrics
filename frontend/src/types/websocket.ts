// SPDX-License-Identifier: Apache-2.0

export interface WSEvent<T = unknown> {
  event: string;
  timestamp: string;
  data: T;
}

export interface WSLogEntryData {
  severity: string;
  scope: string;
  message: string;
  timestamp: string;
  organisation?: string;
  cookbook_name?: string;
  cookbook_version?: string;
  commit_sha?: string;
  chef_client_version?: string;
  collection_run_id?: string;
  notification_channel?: string;
  export_job_id?: string;
  tls_domain?: string;
}
