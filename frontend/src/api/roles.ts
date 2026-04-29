// SPDX-License-Identifier: Apache-2.0

import type {
  RoleListResponse,
  RoleDetailResponse,
  RoleGraphResponse,
} from "../types/roles";
import { apiFetch, buildUrl } from "./client";

export interface RoleFilterQuery {
  page?: number;
  per_page?: number;
  name?: string;
  organisation?: string;
  compatibility_status?: string;
  tk_status?: string;
  target_chef_version?: string;
  sort?: string;
  order?: string;
}

export function fetchRoles(
  filters?: RoleFilterQuery,
): Promise<RoleListResponse> {
  return apiFetch<RoleListResponse>(
    buildUrl(
      "/roles",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchRoleDetail(
  name: string,
  targetChefVersion?: string,
): Promise<RoleDetailResponse> {
  return apiFetch<RoleDetailResponse>(
    buildUrl(`/roles/${encodeURIComponent(name)}`, {
      target_chef_version: targetChefVersion,
    }),
  );
}

export function fetchRoleDependencyGraph(
  name: string,
  params?: { organisation?: string; target_chef_version?: string },
): Promise<RoleGraphResponse> {
  return apiFetch<RoleGraphResponse>(
    buildUrl(
      `/roles/${encodeURIComponent(name)}/dependency-graph`,
      params,
    ),
  );
}
