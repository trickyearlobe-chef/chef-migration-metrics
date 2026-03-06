// ---------------------------------------------------------------------------
// Typed API client for Chef Migration Metrics backend.
//
// All calls go to /api/v1/* which, during development, is proxied to the Go
// backend by Vite (see vite.config.ts). In production the SPA is served from
// the same origin so no proxy is needed.
// ---------------------------------------------------------------------------

import type {
  HealthResponse,
  VersionResponse,
  OrganisationsResponse,
  VersionDistributionResponse,
  ReadinessResponse,
  CookbookCompatibilityResponse,
  NodeListResponse,
  NodeDetailResponse,
  NodesByVersionResponse,
  NodesByCookbookResponse,
  CookbookListResponse,
  CookbookDetailResponse,
  FilterStringResponse,
  FilterPlatformsResponse,
  Pagination,
} from "./types";

// ---------------------------------------------------------------------------
// Base helpers
// ---------------------------------------------------------------------------

const BASE = "/api/v1";

/** Build a URL with optional query parameters. Empty/undefined values are omitted. */
function buildUrl(
  path: string,
  params?: Record<string, string | number | boolean | undefined | null>,
): string {
  const url = new URL(`${BASE}${path}`, window.location.origin);
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null && value !== "") {
        url.searchParams.set(key, String(value));
      }
    }
  }
  return url.pathname + url.search;
}

/** Custom error class carrying the HTTP status and API error body. */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * Core fetch wrapper. Throws `ApiError` on non-2xx responses.
 * Automatically sets Accept header and parses JSON.
 */
async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...init,
    headers: {
      Accept: "application/json",
      ...init?.headers,
    },
  });

  // Health endpoint returns 503 for unhealthy — we still want the body.
  if (!res.ok && res.status !== 503) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const body = await res.json();
      code = body.error ?? code;
      message = body.message ?? message;
    } catch {
      // response body wasn't JSON — use the status text
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<T>;
}

// ---------------------------------------------------------------------------
// Pagination query params helper
// ---------------------------------------------------------------------------

export interface PaginationQuery {
  page?: number;
  per_page?: number;
}

export interface NodeFilterQuery extends PaginationQuery {
  organisation?: string;
  environment?: string;
  platform?: string;
  chef_version?: string;
  policy_name?: string;
  policy_group?: string;
  stale?: string; // "true" | "false" | ""
  sort?: string;
  order?: "asc" | "desc";
}

export interface CookbookFilterQuery extends PaginationQuery {
  organisation?: string;
  source?: string;
  active?: string; // "true" | "false" | ""
  name?: string;
  sort?: string;
  order?: "asc" | "desc";
}

// ---------------------------------------------------------------------------
// Health & version
// ---------------------------------------------------------------------------

export function fetchHealth(): Promise<HealthResponse> {
  return apiFetch<HealthResponse>(buildUrl("/health"));
}

export function fetchVersion(): Promise<VersionResponse> {
  return apiFetch<VersionResponse>(buildUrl("/version"));
}

// ---------------------------------------------------------------------------
// Organisations
// ---------------------------------------------------------------------------

export function fetchOrganisations(): Promise<OrganisationsResponse> {
  return apiFetch<OrganisationsResponse>(buildUrl("/organisations"));
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

export function fetchVersionDistribution(
  organisation?: string,
): Promise<VersionDistributionResponse> {
  return apiFetch<VersionDistributionResponse>(
    buildUrl("/dashboard/version-distribution", { organisation }),
  );
}

export function fetchReadiness(
  organisation?: string,
): Promise<ReadinessResponse> {
  return apiFetch<ReadinessResponse>(
    buildUrl("/dashboard/readiness", { organisation }),
  );
}

export function fetchCookbookCompatibility(
  organisation?: string,
): Promise<CookbookCompatibilityResponse> {
  return apiFetch<CookbookCompatibilityResponse>(
    buildUrl("/dashboard/cookbook-compatibility", { organisation }),
  );
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

export function fetchNodes(
  filters?: NodeFilterQuery,
): Promise<NodeListResponse> {
  return apiFetch<NodeListResponse>(
    buildUrl("/nodes", filters as Record<string, string | number | undefined>),
  );
}

export function fetchNodeDetail(
  organisation: string,
  name: string,
): Promise<NodeDetailResponse> {
  return apiFetch<NodeDetailResponse>(
    buildUrl(`/nodes/${encodeURIComponent(organisation)}/${encodeURIComponent(name)}`),
  );
}

export function fetchNodesByVersion(
  chefVersion: string,
  organisation?: string,
): Promise<NodesByVersionResponse> {
  return apiFetch<NodesByVersionResponse>(
    buildUrl(`/nodes/by-version/${encodeURIComponent(chefVersion)}`, {
      organisation,
    }),
  );
}

export function fetchNodesByCookbook(
  cookbookName: string,
  organisation?: string,
): Promise<NodesByCookbookResponse> {
  return apiFetch<NodesByCookbookResponse>(
    buildUrl(`/nodes/by-cookbook/${encodeURIComponent(cookbookName)}`, {
      organisation,
    }),
  );
}

// ---------------------------------------------------------------------------
// Cookbooks
// ---------------------------------------------------------------------------

export function fetchCookbooks(
  filters?: CookbookFilterQuery,
): Promise<CookbookListResponse> {
  return apiFetch<CookbookListResponse>(
    buildUrl(
      "/cookbooks",
      filters as Record<string, string | number | undefined>,
    ),
  );
}

export function fetchCookbookDetail(
  name: string,
): Promise<CookbookDetailResponse> {
  return apiFetch<CookbookDetailResponse>(
    buildUrl(`/cookbooks/${encodeURIComponent(name)}`),
  );
}

// ---------------------------------------------------------------------------
// Filters
// ---------------------------------------------------------------------------

export function fetchFilterEnvironments(
  organisation?: string,
): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(
    buildUrl("/filters/environments", { organisation }),
  );
}

export function fetchFilterRoles(
  organisation?: string,
): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(
    buildUrl("/filters/roles", { organisation }),
  );
}

export function fetchFilterPolicyNames(
  organisation?: string,
): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(
    buildUrl("/filters/policy-names", { organisation }),
  );
}

export function fetchFilterPolicyGroups(
  organisation?: string,
): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(
    buildUrl("/filters/policy-groups", { organisation }),
  );
}

export function fetchFilterPlatforms(
  organisation?: string,
): Promise<FilterPlatformsResponse> {
  return apiFetch<FilterPlatformsResponse>(
    buildUrl("/filters/platforms", { organisation }),
  );
}

export function fetchFilterTargetChefVersions(): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(
    buildUrl("/filters/target-chef-versions"),
  );
}

export function fetchFilterComplexityLabels(): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(
    buildUrl("/filters/complexity-labels"),
  );
}

// ---------------------------------------------------------------------------
// Utility: poll helper for health badge
// ---------------------------------------------------------------------------

/**
 * Starts polling the health endpoint at the given interval (ms).
 * Returns a cleanup function to stop polling.
 */
export function pollHealth(
  callback: (health: HealthResponse | null) => void,
  intervalMs = 30_000,
): () => void {
  let active = true;

  const tick = async () => {
    if (!active) return;
    try {
      const h = await fetchHealth();
      if (active) callback(h);
    } catch {
      if (active) callback(null);
    }
  };

  // Fetch immediately, then on interval.
  tick();
  const id = setInterval(tick, intervalMs);

  return () => {
    active = false;
    clearInterval(id);
  };
}

// ---------------------------------------------------------------------------
// Re-export Pagination type for convenience
// ---------------------------------------------------------------------------

export type { Pagination };
