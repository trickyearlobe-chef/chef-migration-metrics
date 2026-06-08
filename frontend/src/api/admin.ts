// SPDX-License-Identifier: Apache-2.0

import type {
  AdminUser,
  AdminUserListResponse,
  CreateUserRequest,
  UpdateUserRequest,
  ResetPasswordRequest,
} from "../types";
import { apiFetch, buildUrl } from "./client";
import type { PaginationQuery } from "./client";

export function fetchAdminUsers(
  filters?: PaginationQuery & { search?: string; sort?: string; order?: string },
): Promise<AdminUserListResponse> {
  return apiFetch<AdminUserListResponse>(
    buildUrl("/admin/users", filters as Record<string, string | number | boolean | undefined>),
  );
}

export function createUser(req: CreateUserRequest): Promise<AdminUser> {
  return apiFetch<AdminUser>(buildUrl("/admin/users"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function updateUser(
  username: string,
  req: UpdateUserRequest,
): Promise<AdminUser> {
  return apiFetch<AdminUser>(
    buildUrl(`/admin/users/${encodeURIComponent(username)}`),
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export function resetUserPassword(
  username: string,
  req: ResetPasswordRequest,
): Promise<void> {
  return apiFetch<void>(
    buildUrl(`/admin/users/${encodeURIComponent(username)}/password`),
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export function deleteUser(username: string): Promise<void> {
  return apiFetch<void>(
    buildUrl(`/admin/users/${encodeURIComponent(username)}`),
    { method: "DELETE" },
  );
}

// restartServer requests a graceful restart of the running process so that
// restart-required configuration changes take effect. The server returns 202
// and then exits for the supervisor to restart; the caller should poll
// waitForServerHealthy to detect when it is back online.
// See configuration-live-reload.md § Apply & Restart.
export function restartServer(): Promise<{ status: string; message: string }> {
  return apiFetch<{ status: string; message: string }>(
    buildUrl("/admin/restart"),
    { method: "POST" },
  );
}
