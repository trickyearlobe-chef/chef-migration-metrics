// SPDX-License-Identifier: Apache-2.0

import type {
  NodeListResponse,
  NodeDetailResponse,
  NodeDiskDetailResponse,
  NodesByVersionResponse,
  NodesByCookbookResponse,
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
