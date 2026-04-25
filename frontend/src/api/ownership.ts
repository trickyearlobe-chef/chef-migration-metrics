// SPDX-License-Identifier: Apache-2.0

import type {
  OwnerListResponse,
  OwnerDetail,
  Owner,
  AssignmentListResponse,
  AuditLogResponse,
  OwnershipLookupResponse,
  ReassignResponse,
  ImportResponse,
  CookbookCommittersResponse,
  CommitterAssignResponse,
} from "../types";
import { apiFetch, buildUrl, ApiError } from "./client";
import type { PaginationQuery } from "./client";

export interface OwnerFilterQuery extends PaginationQuery {
  owner_type?: string;
  search?: string;
  sort?: string;
  order?: string;
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
  search?: string;
  sort?: string;
  order?: string;
  since?: string;
}

export function fetchOwners(
  filters?: OwnerFilterQuery,
): Promise<OwnerListResponse> {
  return apiFetch<OwnerListResponse>(
    buildUrl(
      "/owners",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchOwnerDetail(
  name: string,
  params?: { target_chef_version?: string },
): Promise<OwnerDetail> {
  return apiFetch<OwnerDetail>(
    buildUrl(`/owners/${encodeURIComponent(name)}`, params),
  );
}

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
  if (res.ok) return res.json() as Promise<Owner>;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const p = JSON.parse(errBody);
      message = p.message || p.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

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
  if (res.ok) return res.json() as Promise<Owner>;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const p = JSON.parse(errBody);
      message = p.message || p.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export async function deleteOwner(name: string): Promise<void> {
  const url = buildUrl(`/owners/${encodeURIComponent(name)}`);
  const res = await fetch(url, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (res.ok) return;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const p = JSON.parse(errBody);
      message = p.message || p.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export function fetchAssignments(
  ownerName: string,
  filters?: AssignmentFilterQuery,
): Promise<AssignmentListResponse> {
  return apiFetch<AssignmentListResponse>(
    buildUrl(
      `/owners/${encodeURIComponent(ownerName)}/assignments`,
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

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
  if (res.ok) return res.json();
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const p = JSON.parse(errBody);
      message = p.message || p.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

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
  if (res.ok) return;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const p = JSON.parse(errBody);
      message = p.message || p.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

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
  if (res.ok) return res.json() as Promise<ReassignResponse>;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const p = JSON.parse(errBody);
      message = p.message || p.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export function fetchOwnershipLookup(params: {
  entity_type: string;
  entity_key: string;
  organisation?: string;
}): Promise<OwnershipLookupResponse> {
  return apiFetch<OwnershipLookupResponse>(
    buildUrl("/ownership/lookup", params),
  );
}

export function fetchAuditLog(
  filters?: AuditLogFilterQuery,
): Promise<AuditLogResponse> {
  return apiFetch<AuditLogResponse>(
    buildUrl(
      "/ownership/audit-log",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

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
  if (res.ok) return res.json() as Promise<ImportResponse>;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const p = JSON.parse(errBody);
      message = p.message || p.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export function fetchCookbookCommitters(
  cookbookName: string,
  filters?: CommitterFilterQuery,
): Promise<CookbookCommittersResponse> {
  return apiFetch<CookbookCommittersResponse>(
    buildUrl(
      `/cookbooks/${encodeURIComponent(cookbookName)}/committers`,
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

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
  if (res.ok) return res.json() as Promise<CommitterAssignResponse>;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const p = JSON.parse(errBody);
      message = p.message || p.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}
