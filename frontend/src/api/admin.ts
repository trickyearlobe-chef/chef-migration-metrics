// SPDX-License-Identifier: Apache-2.0

import type {
  AdminUser,
  AdminUserListResponse,
  CreateUserRequest,
  UpdateUserRequest,
  ResetPasswordRequest,
} from "../types";
import { apiFetch, buildUrl, ApiError } from "./client";
import type { PaginationQuery } from "./client";

export function fetchAdminUsers(
  filters?: PaginationQuery & { search?: string; sort?: string; order?: string },
): Promise<AdminUserListResponse> {
  return apiFetch<AdminUserListResponse>(
    buildUrl("/admin/users", filters as Record<string, string | number | boolean | undefined>),
  );
}

export async function createUser(req: CreateUserRequest): Promise<AdminUser> {
  const url = buildUrl("/admin/users");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(req),
  });
  if (res.ok) return res.json() as Promise<AdminUser>;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const parsed = JSON.parse(errBody);
      message = parsed.message || parsed.error || message;
    } catch { /* ignore */ }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export async function updateUser(
  username: string,
  req: UpdateUserRequest,
): Promise<AdminUser> {
  const url = buildUrl(
    `/admin/users/${encodeURIComponent(username)}`,
  );
  const res = await fetch(url, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(req),
  });
  if (res.ok) return res.json() as Promise<AdminUser>;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const parsed = JSON.parse(errBody);
      message = parsed.message || parsed.error || message;
    } catch { /* ignore */ }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export async function resetUserPassword(
  username: string,
  req: ResetPasswordRequest,
): Promise<void> {
  const url = buildUrl(
    `/admin/users/${encodeURIComponent(username)}/reset-password`,
  );
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(req),
  });
  if (res.ok) return;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const parsed = JSON.parse(errBody);
      message = parsed.message || parsed.error || message;
    } catch { /* ignore */ }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export async function deleteUser(username: string): Promise<void> {
  const url = buildUrl(`/admin/users/${encodeURIComponent(username)}`);
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
      const parsed = JSON.parse(errBody);
      message = parsed.message || parsed.error || message;
    } catch { /* ignore */ }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}
