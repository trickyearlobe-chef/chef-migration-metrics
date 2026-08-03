// SPDX-License-Identifier: Apache-2.0

import { maintenanceEvents } from "../context/MaintenanceContext";

export const BASE = "/api/v1";

export function buildUrl(
  path: string,
  params?: Record<string, string | number | boolean | undefined>,
): string {
  const url = `${BASE}${path}`;
  if (!params) return url;
  const searchParams = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") {
      searchParams.set(key, String(value));
    }
  }
  const qs = searchParams.toString();
  return qs ? `${url}?${qs}` : url;
}

export class ApiError extends Error {
  status: number;
  body: string;

  constructor(status: number, message: string, body: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

export async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url.startsWith("/api/") ? url : `${BASE}${url}`, {
    ...init,
    headers: {
      Accept: "application/json",
      ...init?.headers,
    },
  });

  if (res.ok) {
    if (res.status === 204 || res.status === 205) return undefined as T;
    const text = await res.text();
    const trimmed = text.trim();
    return (trimmed ? JSON.parse(trimmed) : undefined) as T;
  }

  // Try to extract a structured error message from the JSON body.
  // Many endpoints return { "error": "...", "message": "..." }.
  // Fall back to the HTTP status text when the body is unparseable.
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const body = await res.text();
    try {
      const parsed = JSON.parse(body);
      if (typeof parsed === "object" && parsed !== null) {
        // Detect maintenance mode response
        if (res.status === 503 && parsed.error === "maintenance") {
          maintenanceEvents.dispatchEvent(
            new CustomEvent("maintenance", {
              detail: { active: true, message: parsed.message || "System maintenance in progress." },
            }),
          );
        }
        message =
          parsed.message ||
          parsed.error ||
          parsed.detail ||
          JSON.stringify(parsed);
        if (parsed.code) code = parsed.code;
      }
    } catch {
      if (body) message = body;
    }
    throw new ApiError(code, message, body);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export interface PaginationQuery {
  page?: number;
  per_page?: number;
}

export interface NodeFilterQuery extends PaginationQuery {
  organisation?: string;
  /** Comma-separated owner names. Rejected with a 400 alongside `unowned`. */
  owner?: string;
  /** "true" for the nodes nobody owns. Rejected with a 400 alongside `owner`. */
  unowned?: string;
  node_name?: string;
  environment?: string;
  platform?: string;
  chef_version?: string;
  policy_name?: string;
  policy_group?: string;
  role?: string;
  /** Comma-joined node tags; OR/array-overlap semantics (see node-tags spec). */
  tags?: string;
  stale?: string;
  readiness_filter?: string;
  cookstyle_status?: string;
  kitchen_status?: string;
  target_chef_version?: string;
  migration_state?: string;
  target_converge_status?: string;
  target_version?: string;
  ready_to_activate?: string;
  sort?: string;
  order?: string;
}

export interface CookbookFilterQuery extends PaginationQuery {
  organisation?: string;
  /** Comma-separated owner names. Rejected with a 400 alongside `unowned`. */
  owner?: string;
  /** "true" for the cookbooks nobody owns. Rejected with a 400 alongside `owner`. */
  unowned?: string;
  active?: string;
  name?: string;
  compatibility?: string;
  cookstyle_status?: string;
  tk_status?: string;
  download_status?: string;
  target_chef_version?: string;
  complexity_label?: string;
  is_active?: string;
  sort?: string;
  order?: string;
}

export interface GitRepoFilterQuery extends PaginationQuery {
  name?: string;
  compatibility?: string;
  cookstyle_status?: string;
  tk_status?: string;
  clone_status?: string;
  has_test_suite?: string;
  target_chef_version?: string;
  /** "broken", "not_broken", "any" (somebody has an opinion) or "none". */
  human_verdict?: string;
  /** Comma-separated owner names. Rejected with a 400 alongside `unowned`. */
  owner?: string;
  /** "true" for the repos nobody owns. Rejected with a 400 alongside `owner`. */
  unowned?: string;
  sort?: string;
  order?: string;
}

// RunEventFilterQuery is the view-level filter for the Run events tabs, sourced
// from converge_runs itself (NOT the global organisations filter). `until` is
// the as-of anchor: pinned at view load so live-appended rows don't skew paging.
export interface RunEventFilterQuery extends PaginationQuery {
  organisation?: string;
  status?: string;
  node?: string;
  chef_version?: string;
  cookbook?: string;
  failure_message?: string;
  /** RFC3339 lower bound on end_time. */
  since?: string;
  /** RFC3339 upper bound on end_time — the as-of anchor. */
  until?: string;
  sort?: string;
  order?: string;
}
