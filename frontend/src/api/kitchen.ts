// SPDX-License-Identifier: Apache-2.0

import type {
  TestKitchenConfig,
  TestKitchenConfigResponse,
  TestKitchenConfigSaveResponse,
  PlatformMappingStatusResponse,
  NodeKitchenRun,
  NodeKitchenRunRequest,
  NodeKitchenTriggerResponse,
  KitchenBatch,
  KitchenBatchDetail,
  KitchenBatchRequest,
  GitRepoExcludeRequest,
  GitRepoListItem,
  BatchProgress,
  KitchenAnalysisCookbook,
} from "../types";
import { apiFetch, buildUrl, ApiError } from "./client";

export function fetchTestKitchenConfig(): Promise<TestKitchenConfigResponse> {
  return apiFetch<TestKitchenConfigResponse>(
    buildUrl("/admin/test-kitchen/config"),
  );
}

export async function saveTestKitchenConfig(
  config: TestKitchenConfig,
): Promise<TestKitchenConfigSaveResponse> {
  const url = buildUrl("/admin/test-kitchen/config");
  const res = await fetch(url, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(config),
  });
  if (res.ok) return res.json() as Promise<TestKitchenConfigSaveResponse>;
  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try { const p = JSON.parse(errBody); message = p.message || p.error || message; } catch { /* ignore */ }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export async function deleteTestKitchenConfig(): Promise<void> {
  const url = buildUrl(
    "/admin/test-kitchen/config?confirm=true",
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
    try { const p = JSON.parse(errBody); message = p.message || p.error || message; } catch { /* ignore */ }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export function fetchPlatformMappingStatus(): Promise<PlatformMappingStatusResponse> {
  return apiFetch<PlatformMappingStatusResponse>(
    buildUrl("/admin/platform-mapping/status"),
  );
}

export async function triggerNodeKitchenRun(
  req: NodeKitchenRunRequest,
): Promise<NodeKitchenTriggerResponse> {
  const res = await fetch(buildUrl("/kitchen/node-run"), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`Failed to trigger node kitchen run: ${res.status}`);
  return res.json();
}

export async function fetchNodeKitchenRuns(
  org: string,
  node?: string,
): Promise<NodeKitchenRun[]> {
  const params = new URLSearchParams({ org });
  if (node) params.set("node", node);
  return apiFetch<NodeKitchenRun[]>(`/kitchen/node-runs?${params}`);
}

export async function fetchNodeKitchenRun(id: string): Promise<NodeKitchenRun> {
  return apiFetch<NodeKitchenRun>(`/kitchen/node-runs/${id}`);
}

export async function deleteNodeKitchenRun(id: string): Promise<void> {
  const res = await fetch(buildUrl(`/kitchen/node-runs/${id}`), {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`Failed to delete node kitchen run: ${res.status}`);
}

export async function createKitchenBatch(
  req: KitchenBatchRequest,
): Promise<KitchenBatch> {
  const res = await fetch(buildUrl("/kitchen/batches"), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`Failed to create kitchen batch: ${res.status}`);
  return res.json() as Promise<KitchenBatch>;
}

export async function listKitchenBatches(): Promise<KitchenBatch[]> {
  return apiFetch<KitchenBatch[]>("/kitchen/batches");
}

export async function getKitchenBatch(id: string): Promise<KitchenBatchDetail> {
  return apiFetch<KitchenBatchDetail>(`/kitchen/batches/${id}`);
}

export async function updateKitchenBatch(
  id: string,
  req: KitchenBatchRequest,
): Promise<KitchenBatch> {
  const res = await fetch(buildUrl(`/kitchen/batches/${id}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`Failed to update kitchen batch: ${res.status}`);
  return res.json() as Promise<KitchenBatch>;
}

export async function runKitchenBatch(id: string): Promise<KitchenBatchDetail> {
  const res = await fetch(buildUrl(`/kitchen/batches/${id}/run`), {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`Failed to run kitchen batch: ${res.status}`);
  return res.json() as Promise<KitchenBatchDetail>;
}

export async function cancelKitchenBatch(id: string): Promise<KitchenBatch> {
  const res = await fetch(buildUrl(`/kitchen/batches/${id}/cancel`), {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`Failed to cancel kitchen batch: ${res.status}`);
  return res.json() as Promise<KitchenBatch>;
}

export async function deleteKitchenBatch(id: string): Promise<void> {
  const res = await fetch(buildUrl(`/kitchen/batches/${id}`), {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`Failed to delete kitchen batch: ${res.status}`);
}

export async function excludeGitRepo(
  name: string,
  req: GitRepoExcludeRequest,
): Promise<void> {
  const res = await fetch(
    buildUrl(`/git-repos/${encodeURIComponent(name)}/exclude`),
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify(req),
    },
  );
  if (!res.ok) throw new Error(`Failed to exclude git repo: ${res.status}`);
}

export async function clearGitRepoExclusion(name: string): Promise<void> {
  const res = await fetch(
    buildUrl(`/git-repos/${encodeURIComponent(name)}/exclude`),
    {
      method: "DELETE",
      headers: { Accept: "application/json" },
    },
  );
  if (!res.ok) throw new Error(`Failed to clear git repo exclusion: ${res.status}`);
}

export async function listExcludedGitRepos(): Promise<GitRepoListItem[]> {
  return apiFetch<GitRepoListItem[]>("/git-repos/excluded");
}

export async function fetchBatchProgress(batchId: string): Promise<BatchProgress> {
  return apiFetch<BatchProgress>(`/kitchen/batches/${batchId}/progress`);
}

export async function fetchKitchenAnalysisCookbook(
  name: string,
): Promise<KitchenAnalysisCookbook | null> {
  try {
    return await apiFetch<KitchenAnalysisCookbook>(
      `/kitchen/analysis/cookbooks/${encodeURIComponent(name)}`,
    );
  } catch {
    return null;
  }
}
