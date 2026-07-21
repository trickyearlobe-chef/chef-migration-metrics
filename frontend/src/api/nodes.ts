// SPDX-License-Identifier: Apache-2.0

import type {
  NodeListResponse,
  NodeDetailResponse,
  NodeDiskDetailResponse,
  NodesByVersionResponse,
  NodesByCookbookResponse,
  NodeDependencyGraphResponse,
  NodeRunsResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";
import type { NodeFilterQuery } from "./client";

export function fetchNodes(
  filters?: NodeFilterQuery,
): Promise<NodeListResponse> {
  return apiFetch<NodeListResponse>(
    buildUrl(
      "/nodes",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchNodeDetail(
  orgName: string,
  nodeName: string,
): Promise<NodeDetailResponse> {
  return apiFetch<NodeDetailResponse>(
    buildUrl(
      `/nodes/${encodeURIComponent(orgName)}/${encodeURIComponent(nodeName)}`,
    ),
  );
}

export function fetchNodeDisks(
  orgName: string,
  nodeName: string,
  showAll?: boolean,
): Promise<NodeDiskDetailResponse> {
  return apiFetch<NodeDiskDetailResponse>(
    buildUrl(
      `/nodes/disks/${encodeURIComponent(orgName)}/${encodeURIComponent(nodeName)}`,
      { show_all: showAll ? "true" : undefined },
    ),
  );
}

export function fetchNodeRuns(
  orgName: string,
  nodeName: string,
): Promise<NodeRunsResponse> {
  return apiFetch<NodeRunsResponse>(
    buildUrl(
      `/nodes/runs/${encodeURIComponent(orgName)}/${encodeURIComponent(nodeName)}`,
    ),
  );
}

export function fetchNodesByVersion(
  version: string,
  organisation?: string,
): Promise<NodesByVersionResponse> {
  return apiFetch<NodesByVersionResponse>(
    buildUrl(`/nodes/by-version/${encodeURIComponent(version)}`, {
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

export function fetchNodeDependencyGraph(
  org: string,
  name: string,
  targetChefVersion?: string,
): Promise<NodeDependencyGraphResponse> {
  return apiFetch<NodeDependencyGraphResponse>(
    buildUrl(
      `/nodes/${encodeURIComponent(org)}/${encodeURIComponent(name)}/dependency-graph`,
      { target_chef_version: targetChefVersion },
    ),
  );
}
