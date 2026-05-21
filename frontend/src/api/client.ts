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
  node_name?: string;
  environment?: string;
  platform?: string;
  chef_version?: string;
  policy_name?: string;
  policy_group?: string;
  role?: string;
  stale?: string;
  readiness_filter?: string;
  cookstyle_status?: string;
  kitchen_status?: string;
  target_chef_version?: string;
  sort?: string;
  order?: string;
  search?: string;
}

export interface CookbookFilterQuery extends PaginationQuery {
  organisation?: string;
  active?: string;
  name?: string;
  compatibility?: string;
  tk_status?: string;
  download_status?: string;
  target_chef_version?: string;
  complexity_label?: string;
  is_active?: string;
  search?: string;
  sort?: string;
  order?: string;
}

export interface GitRepoFilterQuery extends PaginationQuery {
  name?: string;
  compatibility?: string;
  tk_status?: string;
  clone_status?: string;
  has_test_suite?: string;
  target_chef_version?: string;
  search?: string;
  sort?: string;
  order?: string;
}
