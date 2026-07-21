// SPDX-License-Identifier: Apache-2.0

import type { Pagination, ConvergeRun, NodeRunsResponse } from "../types";
import { apiFetch, buildUrl } from "./client";
import type { RunEventFilterQuery } from "./client";

// A Run events list row. In the Nodes tab it is a node's latest matching run
// (EXISTS semantics); in the Runs tab it is a single run. Both carry the node
// identity (organisation, node_name) on top of the ConvergeRun run detail.
export interface RunEventItem extends ConvergeRun {
  organisation: string;
  node_name: string;
  source_fqdn?: string;
  chef_server_fqdn?: string;
}

export interface RunEventListResponse {
  data: RunEventItem[];
  pagination: Pagination;
}

// fetchRunEventNodes — the Nodes tab (distinct-node rollup, default surface).
export function fetchRunEventNodes(
  filters?: RunEventFilterQuery,
): Promise<RunEventListResponse> {
  return apiFetch<RunEventListResponse>(
    buildUrl(
      "/run-events/nodes",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

// fetchRunEventRuns — the Runs tab (flat run firehose).
export function fetchRunEventRuns(
  filters?: RunEventFilterQuery,
): Promise<RunEventListResponse> {
  return apiFetch<RunEventListResponse>(
    buildUrl(
      "/run-events/runs",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

// fetchRunEventNodeRuns — every run for one node (the detail page). Keys on the
// DELIVERED org name; unlike fetchNodeRuns it does NOT resolve via the
// organisations table, so ingest-only DMZ nodes resolve.
export function fetchRunEventNodeRuns(
  organisation: string,
  nodeName: string,
): Promise<NodeRunsResponse> {
  return apiFetch<NodeRunsResponse>(
    buildUrl(
      `/run-events/nodes/${encodeURIComponent(organisation)}/${encodeURIComponent(nodeName)}`,
    ),
  );
}
