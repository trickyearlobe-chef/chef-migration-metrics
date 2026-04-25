// SPDX-License-Identifier: Apache-2.0

import type {
  LogListResponse,
  LogEntry,
  CollectionRunListResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";

export interface LogFilterQuery {
  scope?: string;
  severity?: string;
  min_severity?: string;
  organisation?: string;
  cookbook_name?: string;
  collection_run_id?: string;
  since?: string;
  until?: string;
  search?: string;
  sort?: string;
  order?: string;
  page?: number;
  per_page?: number;
}

export function fetchLogs(filters?: LogFilterQuery): Promise<LogListResponse> {
  return apiFetch<LogListResponse>(
    buildUrl(
      "/logs",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchLogDetail(id: string): Promise<LogEntry> {
  return apiFetch<LogEntry>(buildUrl(`/logs/${id}`));
}

export interface CollectionRunFilterQuery {
  organisation?: string;
  status?: string;
  page?: number;
  per_page?: number;
}

export function fetchCollectionRuns(
  filters?: CollectionRunFilterQuery,
): Promise<CollectionRunListResponse> {
  return apiFetch<CollectionRunListResponse>(
    buildUrl(
      "/logs/collection-runs",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}
