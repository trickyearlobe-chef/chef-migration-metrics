// SPDX-License-Identifier: Apache-2.0

import type { FilterStringResponse, FilterPlatformsResponse } from "../types";
import { apiFetch, buildUrl } from "./client";

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

export function fetchFilterTags(
  organisation?: string,
): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(
    buildUrl("/filters/tags", { organisation }),
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
  return apiFetch<FilterStringResponse>(buildUrl("/filters/complexity-labels"));
}

// Run events filter options — sourced from converge_runs, NOT the organisations
// table, so ingest-only DMZ orgs are selectable.
export function fetchFilterRunOrganisations(): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(buildUrl("/filters/run-organisations"));
}

export function fetchFilterRunChefVersions(): Promise<FilterStringResponse> {
  return apiFetch<FilterStringResponse>(buildUrl("/filters/run-chef-versions"));
}
