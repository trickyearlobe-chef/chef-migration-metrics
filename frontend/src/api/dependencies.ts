// SPDX-License-Identifier: Apache-2.0

import type {
  DependencyGraphResponse,
  DependencyGraphTableResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";
import type { PaginationQuery } from "./client";

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
