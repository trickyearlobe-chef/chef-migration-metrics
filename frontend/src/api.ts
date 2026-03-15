// ---------------------------------------------------------------------------
// Typed API client for Chef Migration Metrics backend.
//
// All calls go to /api/v1/* which, during development, is proxied to the Go
// backend by Vite (see vite.config.ts). In production the SPA is served from
// the same origin so no proxy is needed.
// ---------------------------------------------------------------------------

import type {
  ExportRequest,
  ExportJobResponse,
  HealthResponse,
  VersionResponse,
  OrganisationsResponse,
  VersionDistributionResponse,
  PlatformDistributionResponse,
  VersionDistributionTrendResponse,
  ReadinessResponse,
  ReadinessTrendResponse,
  ComplexityTrendResponse,
  StaleTrendResponse,
  CookbookCompatibilityResponse,
  GitRepoCompatibilityResponse,
  CookbookRemediationResponse,
  NodeListResponse,
  NodeDetailResponse,
  NodesByVersionResponse,
  NodesByCookbookResponse,
  CookbookListResponse,
  CookbookDetailResponse,
  RemediationPriorityResponse,
  RemediationSummaryResponse,
  FilterStringResponse,
  FilterPlatformsResponse,
  DependencyGraphResponse,
  DependencyGraphTableResponse,
  LogListResponse,
  LogEntry,
  CollectionRunListResponse,
  Pagination,
  LoginRequest,
  LoginResponse,
  MeResponse,
  AdminUser,
  AdminUserListResponse,
  CreateUserRequest,
  UpdateUserRequest,
  ResetPasswordRequest,
  OwnerListResponse,
  OwnerDetail,
  AssignmentListResponse,
  AuditLogResponse,
  OwnershipLookupResponse,
  ReassignResponse,
  ImportResponse,
  CookbookCommittersResponse,
  CommitterAssignResponse,
  Owner,
  ResetGitCookbookResponse,
  GitRepoListResponse,
  GitRepoDetailResponse,
  GitRepoRemediationResponse,
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
    // If the session has expired or is invalid, redirect to login
    // immediately rather than showing cryptic errors in the UI.
    if (res.status === 401 && !url.includes("/auth/login") && !url.includes("/auth/me")) {
      window.location.href = "/login";
      // Return a never-resolving promise so callers don't continue.
      return new Promise<T>(() => { });
    }

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
  node_name?: string;
  environment?: string;
  platform?: string;
  chef_version?: string;
  policy_name?: string;
  policy_group?: string;
  role?: string;
  stale?: string; // "true" | "false" | ""
  sort?: string;
  order?: "asc" | "desc";
}

export interface CookbookFilterQuery extends PaginationQuery {
  organisation?: string;
  active?: string; // "true" | "false" | ""
  name?: string;
  compatibility?: string; // "compatible" | "incompatible" | "untested" | ""
  target_chef_version?: string;
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

export function fetchPlatformDistribution(
  organisation?: string,
): Promise<PlatformDistributionResponse> {
  return apiFetch<PlatformDistributionResponse>(
    buildUrl("/dashboard/platform-distribution", { organisation }),
  );
}

export function fetchReadiness(
  organisation?: string,
): Promise<ReadinessResponse> {
  return apiFetch<ReadinessResponse>(
    buildUrl("/dashboard/readiness", { organisation }),
  );
}

export function fetchVersionDistributionTrend(
  organisation?: string,
): Promise<VersionDistributionTrendResponse> {
  return apiFetch<VersionDistributionTrendResponse>(
    buildUrl("/dashboard/version-distribution/trend", { organisation }),
  );
}

export function fetchReadinessTrend(
  organisation?: string,
): Promise<ReadinessTrendResponse> {
  return apiFetch<ReadinessTrendResponse>(
    buildUrl("/dashboard/readiness/trend", { organisation }),
  );
}

export function fetchComplexityTrend(
  organisation?: string,
): Promise<ComplexityTrendResponse> {
  return apiFetch<ComplexityTrendResponse>(
    buildUrl("/dashboard/complexity/trend", { organisation }),
  );
}

export function fetchStaleTrend(
  organisation?: string,
): Promise<StaleTrendResponse> {
  return apiFetch<StaleTrendResponse>(
    buildUrl("/dashboard/stale/trend", { organisation }),
  );
}

export function fetchCookbookCompatibility(
  organisation?: string,
): Promise<CookbookCompatibilityResponse> {
  return apiFetch<CookbookCompatibilityResponse>(
    buildUrl("/dashboard/cookbook-compatibility", { organisation }),
  );
}

export function fetchGitRepoCompatibility(
  organisation?: string,
): Promise<GitRepoCompatibilityResponse> {
  return apiFetch<GitRepoCompatibilityResponse>(
    buildUrl("/dashboard/git-repo-compatibility", { organisation }),
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

export function requestCookbookRescan(
  name: string,
): Promise<{ cookbook_name: string; versions_invalidated: number; message: string }> {
  return apiFetch(
    `/api/v1/cookbooks/${encodeURIComponent(name)}/rescan`,
    { method: "POST" },
  );
}

export function rescanAllCookstyle(): Promise<{ message: string }> {
  return apiFetch<{ message: string }>(
    "/api/v1/admin/rescan-all-cookstyle",
    { method: "POST" },
  );
}

export function resetGitCookbook(
  name: string,
): Promise<ResetGitCookbookResponse> {
  return apiFetch<ResetGitCookbookResponse>(
    `/api/v1/cookbooks/${encodeURIComponent(name)}/reset-git`,
    { method: "POST" },
  );
}

export function fetchCookbookRemediation(
  name: string,
  version: string,
  params?: { target_chef_version?: string },
): Promise<CookbookRemediationResponse> {
  return apiFetch<CookbookRemediationResponse>(
    buildUrl(
      `/cookbooks/${encodeURIComponent(name)}/${encodeURIComponent(version)}/remediation`,
      params,
    ),
  );
}

// ---------------------------------------------------------------------------
// Git Repos
// ---------------------------------------------------------------------------

export function fetchGitRepos(
  filters?: { name?: string; compatibility?: string; target_chef_version?: string; page?: number; per_page?: number },
): Promise<GitRepoListResponse> {
  return apiFetch<GitRepoListResponse>(
    buildUrl("/git-repos", filters as Record<string, string | number | undefined>),
  );
}

export function fetchGitRepoDetail(
  name: string,
): Promise<GitRepoDetailResponse> {
  return apiFetch<GitRepoDetailResponse>(
    buildUrl(`/git-repos/${encodeURIComponent(name)}`),
  );
}

export function requestGitRepoRescan(
  name: string,
): Promise<{ git_repo_name: string; repos_invalidated: number; message: string }> {
  return apiFetch(
    `/api/v1/git-repos/${encodeURIComponent(name)}/rescan`,
    { method: "POST" },
  );
}

export function resetGitRepo(
  name: string,
): Promise<ResetGitCookbookResponse> {
  return apiFetch<ResetGitCookbookResponse>(
    `/api/v1/git-repos/${encodeURIComponent(name)}/reset`,
    { method: "POST" },
  );
}

export function fetchGitRepoRemediation(
  name: string,
  version: string,
  params?: { target_chef_version?: string },
): Promise<GitRepoRemediationResponse> {
  return apiFetch<GitRepoRemediationResponse>(
    buildUrl(
      `/git-repos/${encodeURIComponent(name)}/${encodeURIComponent(version)}/remediation`,
      params,
    ),
  );
}

export function fetchGitRepoCommitters(
  repoName: string,
  filters?: CommitterFilterQuery,
): Promise<CookbookCommittersResponse> {
  return apiFetch<CookbookCommittersResponse>(
    buildUrl(
      `/git-repos/${encodeURIComponent(repoName)}/committers`,
      filters as Record<string, string | number | undefined>,
    ),
  );
}

// ---------------------------------------------------------------------------
// Dependency Graph
// ---------------------------------------------------------------------------

export function fetchDependencyGraph(
  organisation: string,
): Promise<DependencyGraphResponse> {
  return apiFetch<DependencyGraphResponse>(
    buildUrl("/dependency-graph", { organisation }),
  );
}

export interface DependencyGraphTableQuery extends PaginationQuery {
  organisation: string;
  sort?: string;
  order?: "asc" | "desc";
}

export function fetchDependencyGraphTable(
  filters: DependencyGraphTableQuery,
): Promise<DependencyGraphTableResponse> {
  return apiFetch<DependencyGraphTableResponse>(
    buildUrl(
      "/dependency-graph/table",
      filters as unknown as Record<string, string | number | undefined>,
    ),
  );
}

// ---------------------------------------------------------------------------
// Remediation
// ---------------------------------------------------------------------------

export interface RemediationQuery extends PaginationQuery {
  organisation?: string;
  target_chef_version?: string;
  complexity_label?: string;
  sort?: string;
  order?: "asc" | "desc";
}

export function fetchRemediationPriority(
  filters?: RemediationQuery,
): Promise<RemediationPriorityResponse> {
  return apiFetch<RemediationPriorityResponse>(
    buildUrl(
      "/remediation/priority",
      filters as Record<string, string | number | undefined>,
    ),
  );
}

export function fetchRemediationSummary(
  params?: { organisation?: string; target_chef_version?: string },
): Promise<RemediationSummaryResponse> {
  return apiFetch<RemediationSummaryResponse>(
    buildUrl("/remediation/summary", params),
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
// Logs
// ---------------------------------------------------------------------------

export interface LogFilterQuery extends PaginationQuery {
  scope?: string;
  severity?: string;
  min_severity?: string;
  organisation?: string;
  cookbook_name?: string;
  collection_run_id?: string;
  since?: string;
  until?: string;
}

export function fetchLogs(filters?: LogFilterQuery): Promise<LogListResponse> {
  return apiFetch<LogListResponse>(
    buildUrl("/logs", filters as Record<string, string | number | undefined>),
  );
}

export function fetchLogDetail(id: string): Promise<LogEntry> {
  return apiFetch<LogEntry>(
    buildUrl(`/logs/${encodeURIComponent(id)}`),
  );
}

export interface CollectionRunFilterQuery extends PaginationQuery {
  organisation?: string;
  status?: string;
}

export function fetchCollectionRuns(
  filters?: CollectionRunFilterQuery,
): Promise<CollectionRunListResponse> {
  return apiFetch<CollectionRunListResponse>(
    buildUrl(
      "/logs/collection-runs",
      filters as Record<string, string | number | undefined>,
    ),
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
// Exports
// ---------------------------------------------------------------------------

/**
 * Create a new data export. For small result sets the server responds with
 * 200 and streams the file directly (the browser will trigger a download).
 * For large result sets it responds with 202 and a job ID for polling.
 *
 * When the response is 200 (synchronous), the returned promise resolves to
 * `null` — the file download is handled by the browser via a hidden link.
 * When the response is 202 (asynchronous), the promise resolves to the
 * ExportJobResponse containing the `job_id` for status polling.
 */
export async function createExport(
  body: ExportRequest,
): Promise<ExportJobResponse | null> {
  const url = buildUrl("/exports");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (res.status === 200) {
    // Synchronous export — server streamed the file directly.
    // Trigger a browser download from the response blob.
    const disposition = res.headers.get("Content-Disposition") ?? "";
    const filenameMatch = disposition.match(/filename="?([^"]+)"?/);
    const filename = filenameMatch?.[1] ?? `export.${body.format === "json" ? "json" : body.format === "chef_search_query" ? "txt" : "csv"}`;
    const blob = await res.blob();
    const blobUrl = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = blobUrl;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(blobUrl);
    return null;
  }

  if (res.status === 202) {
    return res.json() as Promise<ExportJobResponse>;
  }

  // Error responses.
  let code = "unknown";
  let message = `HTTP ${res.status}`;
  try {
    const errBody = await res.json();
    code = errBody.error ?? code;
    message = errBody.message ?? message;
  } catch {
    message = res.statusText || message;
  }
  throw new ApiError(res.status, code, message);
}

/**
 * Poll an async export job's status.
 */
export function fetchExportStatus(jobId: string): Promise<ExportJobResponse> {
  return apiFetch<ExportJobResponse>(
    buildUrl(`/exports/${encodeURIComponent(jobId)}`),
  );
}

/**
 * Returns the URL to download a completed export file.
 * The caller should open this in a new tab or create a hidden anchor click.
 */
export function downloadExportUrl(jobId: string): string {
  return `${BASE}/exports/${encodeURIComponent(jobId)}/download`;
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

/**
 * POST /api/v1/auth/login — authenticate with username and password.
 * On success the server sets an HTTP-only session cookie and returns
 * a LoginResponse with token, expiry, and user info.
 */
export async function login(body: LoginRequest): Promise<LoginResponse> {
  const url = buildUrl("/auth/login");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<LoginResponse>;
}

/**
 * POST /api/v1/auth/logout — invalidate the current session.
 * Returns void (204 No Content on success).
 */
export async function logout(): Promise<void> {
  const url = buildUrl("/auth/logout");
  const res = await fetch(url, {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  // 204 No Content is the expected success response.
  if (!res.ok && res.status !== 204) {
    throw new ApiError(res.status, "logout_failed", "Logout failed.");
  }
}

/**
 * GET /api/v1/auth/me — fetch the current user's profile from the session.
 */
export function fetchMe(): Promise<MeResponse> {
  return apiFetch<MeResponse>(buildUrl("/auth/me"));
}

// ---------------------------------------------------------------------------
// Admin user management
// ---------------------------------------------------------------------------

/** GET /api/v1/admin/users — list all users (admin only). */
export function fetchAdminUsers(
  params?: { page?: number; per_page?: number },
): Promise<AdminUserListResponse> {
  return apiFetch<AdminUserListResponse>(
    buildUrl("/admin/users", params as Record<string, string | number | undefined>),
  );
}

/** POST /api/v1/admin/users — create a new local user (admin only). */
export async function createUser(body: CreateUserRequest): Promise<AdminUser> {
  const url = buildUrl("/admin/users");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<AdminUser>;
}

/** PUT /api/v1/admin/users/:username — update an existing user (admin only). */
export async function updateUser(
  username: string,
  body: UpdateUserRequest,
): Promise<AdminUser> {
  const url = buildUrl(`/admin/users/${encodeURIComponent(username)}`);
  const res = await fetch(url, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<AdminUser>;
}

/** PUT /api/v1/admin/users/:username/password — reset a user's password (admin only). */
export async function resetUserPassword(
  username: string,
  body: ResetPasswordRequest,
): Promise<void> {
  const url = buildUrl(
    `/admin/users/${encodeURIComponent(username)}/password`,
  );
  const res = await fetch(url, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }
}

/** DELETE /api/v1/admin/users/:username — delete a user (admin only). */
export async function deleteUser(username: string): Promise<void> {
  const url = buildUrl(`/admin/users/${encodeURIComponent(username)}`);
  const res = await fetch(url, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });

  if (!res.ok && res.status !== 204) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }
}

// ---------------------------------------------------------------------------
// Ownership
// ---------------------------------------------------------------------------

export interface OwnerFilterQuery extends PaginationQuery {
  owner_type?: string;
  search?: string;
  sort?: string;
  order?: "asc" | "desc";
  target_chef_version?: string;
}

export interface AssignmentFilterQuery extends PaginationQuery {
  entity_type?: string;
  organisation?: string;
  assignment_source?: string;
}

export interface AuditLogFilterQuery extends PaginationQuery {
  action?: string;
  actor?: string;
  owner_name?: string;
  entity_type?: string;
  entity_key?: string;
  since?: string;
  until?: string;
}

export interface CommitterFilterQuery extends PaginationQuery {
  sort?: string;
  order?: "asc" | "desc";
  since?: string;
}

/** GET /api/v1/owners — list owners with optional filters. */
export function fetchOwners(
  filters?: OwnerFilterQuery,
): Promise<OwnerListResponse> {
  return apiFetch<OwnerListResponse>(
    buildUrl("/owners", filters as Record<string, string | number | undefined>),
  );
}

/** GET /api/v1/owners/:name — get owner detail with summaries. */
export function fetchOwnerDetail(
  name: string,
  params?: { target_chef_version?: string },
): Promise<OwnerDetail> {
  return apiFetch<OwnerDetail>(
    buildUrl(`/owners/${encodeURIComponent(name)}`, params),
  );
}

/** POST /api/v1/owners — create a new owner. */
export async function createOwner(body: {
  name: string;
  owner_type: string;
  display_name?: string;
  contact_email?: string;
  contact_channel?: string;
  metadata?: Record<string, unknown>;
}): Promise<Owner> {
  const url = buildUrl("/owners");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<Owner>;
}

/** PUT /api/v1/owners/:name — update an existing owner. */
export async function updateOwner(
  name: string,
  body: {
    display_name?: string;
    contact_email?: string;
    contact_channel?: string;
    owner_type?: string;
    metadata?: Record<string, unknown>;
  },
): Promise<Owner> {
  const url = buildUrl(`/owners/${encodeURIComponent(name)}`);
  const res = await fetch(url, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<Owner>;
}

/** DELETE /api/v1/owners/:name — delete an owner. */
export async function deleteOwner(name: string): Promise<void> {
  const url = buildUrl(`/owners/${encodeURIComponent(name)}`);
  const res = await fetch(url, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });

  if (!res.ok && res.status !== 204) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }
}

/** GET /api/v1/owners/:name/assignments — list assignments for an owner. */
export function fetchAssignments(
  ownerName: string,
  filters?: AssignmentFilterQuery,
): Promise<AssignmentListResponse> {
  return apiFetch<AssignmentListResponse>(
    buildUrl(
      `/owners/${encodeURIComponent(ownerName)}/assignments`,
      filters as Record<string, string | number | undefined>,
    ),
  );
}

/** POST /api/v1/owners/:name/assignments — create assignments. */
export async function createAssignments(
  ownerName: string,
  body: {
    assignments: {
      entity_type: string;
      entity_key: string;
      organisation?: string;
      notes?: string;
    }[];
  },
): Promise<{ created: number; assignments: unknown[] }> {
  const url = buildUrl(`/owners/${encodeURIComponent(ownerName)}/assignments`);
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json();
}

/** DELETE /api/v1/owners/:name/assignments/:id — delete an assignment. */
export async function deleteAssignment(
  ownerName: string,
  assignmentId: string,
): Promise<void> {
  const url = buildUrl(
    `/owners/${encodeURIComponent(ownerName)}/assignments/${encodeURIComponent(assignmentId)}`,
  );
  const res = await fetch(url, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });

  if (!res.ok && res.status !== 204) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }
}

/** POST /api/v1/ownership/reassign — bulk reassign ownership. */
export async function reassignOwnership(body: {
  from_owner: string;
  to_owner: string;
  entity_type?: string;
  organisation?: string;
  delete_source_owner?: boolean;
}): Promise<ReassignResponse> {
  const url = buildUrl("/ownership/reassign");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<ReassignResponse>;
}

/** GET /api/v1/ownership/lookup — lookup who owns an entity. */
export function fetchOwnershipLookup(params: {
  entity_type: string;
  entity_key: string;
  organisation?: string;
}): Promise<OwnershipLookupResponse> {
  return apiFetch<OwnershipLookupResponse>(
    buildUrl("/ownership/lookup", params),
  );
}

/** GET /api/v1/ownership/audit-log — paginated audit log. */
export function fetchAuditLog(
  filters?: AuditLogFilterQuery,
): Promise<AuditLogResponse> {
  return apiFetch<AuditLogResponse>(
    buildUrl(
      "/ownership/audit-log",
      filters as Record<string, string | number | undefined>,
    ),
  );
}

/** POST /api/v1/ownership/import — bulk import via file upload. */
export async function importOwnership(
  file: File,
  format: "csv" | "json",
): Promise<ImportResponse> {
  const url = buildUrl("/ownership/import");
  const formData = new FormData();
  formData.append("format", format);
  formData.append("file", file);

  const res = await fetch(url, {
    method: "POST",
    headers: { Accept: "application/json" },
    body: formData,
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<ImportResponse>;
}

/** GET /api/v1/cookbooks/:name/committers — list committers for a cookbook. */
export function fetchCookbookCommitters(
  cookbookName: string,
  filters?: CommitterFilterQuery,
): Promise<CookbookCommittersResponse> {
  return apiFetch<CookbookCommittersResponse>(
    buildUrl(
      `/cookbooks/${encodeURIComponent(cookbookName)}/committers`,
      filters as Record<string, string | number | undefined>,
    ),
  );
}

/** POST /api/v1/cookbooks/:name/committers/assign — assign committers as owners. */
export async function assignCookbookCommitters(
  cookbookName: string,
  body: {
    committers: {
      author_email: string;
      owner_name: string;
      display_name?: string;
    }[];
  },
): Promise<CommitterAssignResponse> {
  const url = buildUrl(
    `/cookbooks/${encodeURIComponent(cookbookName)}/committers/assign`,
  );
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `HTTP ${res.status}`;
    try {
      const errBody = await res.json();
      code = errBody.error ?? code;
      message = errBody.message ?? message;
    } catch {
      message = res.statusText || message;
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<CommitterAssignResponse>;
}

// ---------------------------------------------------------------------------
// Re-export Pagination type for convenience
// ---------------------------------------------------------------------------

export type { Pagination };
