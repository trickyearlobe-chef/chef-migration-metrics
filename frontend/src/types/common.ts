// SPDX-License-Identifier: Apache-2.0

export interface Pagination {
  page: number;
  per_page: number;
  total_items: number;
  total_pages: number;
}

export interface PaginatedResponse<T> {
  data: T[];
  pagination: Pagination;
}

export interface ApiError {
  error: string;
  message: string;
}

export interface HealthResponse {
  status: "healthy" | "unhealthy";
  version: string;
  websocket_enabled: boolean;
  websocket_clients: number;
  error?: string;
}

export interface VersionResponse {
  version: string;
}

// TLSStatus mirrors GET /api/v1/server/tls-status. When the TLS listener cannot
// be built at startup the server falls open to a degraded listener — a
// self-signed HTTPS cert, or plain HTTP as a last resort (tls.md § 6.3) — and
// reports degraded=true so the UI can warn that the certificate is untrusted.
export interface TLSStatus {
  degraded: boolean;
  // kind is the degraded-listener kind: "self-signed" (untrusted HTTPS) or
  // "plain" (last-resort cleartext). Absent when healthy.
  kind?: "self-signed" | "plain";
  reason?: string;
}

export interface Organisation {
  name: string;
  chef_server_url: string;
  org_name: string;
  client_name: string;
  credential_source: string;
  source: string;
  node_count: number;
  last_collected_at?: string;
  last_collection_status?: string;
}

export interface OrganisationsResponse {
  data: Organisation[];
}

export interface FilterStringResponse {
  data: string[];
}

export interface FilterPlatformEntry {
  value: string;
  display_name: string | null;
  group_key?: string;
  group_display_name?: string;
}

export interface FilterPlatformsResponse {
  data: FilterPlatformEntry[];
}

export type CompatibilityStatus =
  | "compatible"
  | "incompatible"
  | "cookstyle_only"
  | "untested";

/**
 * CookStyleStatus is the classification-derived CookStyle rollup verdict (the
 * single source of truth), returned as `cookstyle_status` on list, detail, and
 * remediation responses. It is kept separate from the Test Kitchen signal.
 */
export type CookStyleStatus =
  | "ready"
  | "needs_review"
  | "blocked"
  | "untested";

export type ConfidenceLevel = "high" | "medium" | null;

export type ComplexityLabel = "low" | "medium" | "high" | "critical";
