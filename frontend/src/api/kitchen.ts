// SPDX-License-Identifier: Apache-2.0

import type {
  TestKitchenConfig,
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
  GitKitchenPlanResult,
  GitKitchenResult,
  GitKitchenRunRequest,
  GitKitchenRunResponse,
  GitKitchenRunAllRequest,
  GitKitchenRunAllResponse,
  SweepResult,
  KitchenInstanceExclusion,
  CreateKitchenExclusionRequest,
  KitchenQueueItem,
  KitchenQueueListResponse,
  KitchenQueueEnqueueResponse,
} from "../types";
import { apiFetch, buildUrl, ApiError } from "./client";
import { decodePutConfigResponse } from "./config";
import type { PutConfigResponse } from "./config";

export function fetchTestKitchenConfig(): Promise<TestKitchenConfig> {
  return apiFetch<TestKitchenConfig>(
    buildUrl("/admin/config/test-kitchen"),
  );
}

export async function saveTestKitchenConfig(
  config: TestKitchenConfig,
): Promise<PutConfigResponse<TestKitchenConfig>> {
  const res = await fetch(buildUrl("/admin/config/test-kitchen"), {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(config),
  });
  return decodePutConfigResponse<TestKitchenConfig>(res);
}

export async function deleteTestKitchenConfig(): Promise<void> {
  const res = await fetch(buildUrl("/admin/config/test-kitchen"), {
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

export async function fetchGitKitchenInstances(
  repoName: string,
): Promise<GitKitchenPlanResult> {
  return apiFetch<GitKitchenPlanResult>(
    `/kitchen/git/instances?repo=${encodeURIComponent(repoName)}`,
  );
}

export async function fetchGitKitchenResults(
  repoName?: string,
): Promise<GitKitchenResult[]> {
  const params = repoName
    ? `?repo=${encodeURIComponent(repoName)}`
    : "";
  return apiFetch<GitKitchenResult[]>(`/kitchen/git/results${params}`);
}

export async function triggerGitKitchenRun(
  req: GitKitchenRunRequest,
): Promise<GitKitchenRunResponse> {
  const res = await fetch(buildUrl("/kitchen/git/run"), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`Failed to trigger git kitchen run: ${res.status}`);
  return res.json();
}

export async function triggerGitKitchenRunAll(
  req: GitKitchenRunAllRequest,
): Promise<GitKitchenRunAllResponse> {
  const res = await fetch(buildUrl("/kitchen/git/run-all"), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`Failed to trigger run-all: ${res.status}`);
  return res.json();
}

export async function runOrphanSweep(dryRun: boolean): Promise<SweepResult> {
  return apiFetch<SweepResult>(`/kitchen/orphan-sweep?dry_run=${dryRun}`, {
    method: "POST",
  });
}

// --- Kitchen Instance Exclusions ---

export async function fetchKitchenExclusions(
  repoName?: string,
): Promise<KitchenInstanceExclusion[]> {
  const params = repoName
    ? `?repo=${encodeURIComponent(repoName)}`
    : "";
  return apiFetch<KitchenInstanceExclusion[]>(
    `/kitchen/git/exclusions${params}`,
  );
}

export async function createKitchenExclusion(
  req: CreateKitchenExclusionRequest,
): Promise<KitchenInstanceExclusion> {
  const res = await fetch(buildUrl("/kitchen/git/exclusions"), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    const msg = await res.text().catch(() => "");
    throw new Error(msg || `Failed to create exclusion: ${res.status}`);
  }
  return res.json();
}

export async function deleteKitchenExclusion(id: string): Promise<void> {
  const res = await fetch(buildUrl(`/kitchen/git/exclusions/${id}`), {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`Failed to delete exclusion: ${res.status}`);
}

// --- Kitchen Run Queue ---

export function fetchKitchenQueue(params?: {
  repo?: string;
  type?: "git" | "node";
  status?: string;
}): Promise<KitchenQueueListResponse> {
  const searchParams = new URLSearchParams();
  if (params?.repo) searchParams.set("repo", params.repo);
  if (params?.type) searchParams.set("type", params.type);
  if (params?.status) searchParams.set("status", params.status);
  const qs = searchParams.toString();
  return apiFetch<KitchenQueueListResponse>(
    buildUrl(`/kitchen/queue${qs ? `?${qs}` : ""}`),
  );
}

export function fetchKitchenQueueItem(id: string): Promise<KitchenQueueItem> {
  return apiFetch<KitchenQueueItem>(buildUrl(`/kitchen/queue/${id}`));
}

export function cancelKitchenQueueItem(id: string): Promise<{ message: string; id: string }> {
  return apiFetch<{ message: string; id: string }>(buildUrl(`/kitchen/queue/${id}/cancel`), {
    method: "POST",
  });
}

export function retryKitchenQueueItem(id: string): Promise<KitchenQueueEnqueueResponse> {
  return apiFetch<KitchenQueueEnqueueResponse>(buildUrl(`/kitchen/queue/${id}/retry`), {
    method: "POST",
  });
}

export function fetchKitchenQueueStats(): Promise<KitchenQueueListResponse["stats"]> {
  return apiFetch<KitchenQueueListResponse["stats"]>(buildUrl("/kitchen/queue/stats"));
}
