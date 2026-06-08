// SPDX-License-Identifier: Apache-2.0

import type {
  TestKitchenConfig,
  PlatformMappingStatusResponse,
  HypervisorTestConnectionResponse,
  NodeKitchenRun,
  NodeKitchenRunRequest,
  NodeKitchenTriggerResponse,
  KitchenBatch,
  KitchenBatchDetail,
  KitchenBatchRequest,
  GitRepoExcludeRequest,
  GitRepoListItem,
  BatchProgress,
  KitchenBatchInstance,
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
import { apiFetch, buildUrl } from "./client";
import { apiMutateConfig } from "./config";
import type { PutConfigResponse } from "./config";

export function fetchTestKitchenConfig(): Promise<TestKitchenConfig> {
  return apiFetch<TestKitchenConfig>(
    buildUrl("/admin/config/test-kitchen"),
  );
}

export function saveTestKitchenConfig(
  config: TestKitchenConfig,
): Promise<PutConfigResponse<TestKitchenConfig>> {
  return apiMutateConfig<TestKitchenConfig>(
    buildUrl("/admin/config/test-kitchen"),
    config,
  );
}

export function deleteTestKitchenConfig(): Promise<void> {
  return apiFetch<void>(buildUrl("/admin/config/test-kitchen"), {
    method: "DELETE",
  });
}

export function fetchPlatformMappingStatus(): Promise<PlatformMappingStatusResponse> {
  return apiFetch<PlatformMappingStatusResponse>(
    buildUrl("/admin/platform-mapping/status"),
  );
}

export function triggerNodeKitchenRun(
  req: NodeKitchenRunRequest,
): Promise<NodeKitchenTriggerResponse> {
  return apiFetch<NodeKitchenTriggerResponse>(buildUrl("/kitchen/node-run"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
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

export function deleteNodeKitchenRun(id: string): Promise<void> {
  return apiFetch<void>(buildUrl(`/kitchen/node-runs/${id}`), {
    method: "DELETE",
  });
}

export function createKitchenBatch(
  req: KitchenBatchRequest,
): Promise<KitchenBatch> {
  return apiFetch<KitchenBatch>(buildUrl("/kitchen/batches"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function listKitchenBatches(): Promise<KitchenBatch[]> {
  return apiFetch<KitchenBatch[]>("/kitchen/batches");
}

export async function getKitchenBatch(id: string): Promise<KitchenBatchDetail> {
  return apiFetch<KitchenBatchDetail>(`/kitchen/batches/${id}`);
}

export function updateKitchenBatch(
  id: string,
  req: KitchenBatchRequest,
): Promise<KitchenBatch> {
  return apiFetch<KitchenBatch>(buildUrl(`/kitchen/batches/${id}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function runKitchenBatch(id: string): Promise<KitchenBatchDetail> {
  return apiFetch<KitchenBatchDetail>(buildUrl(`/kitchen/batches/${id}/run`), {
    method: "POST",
  });
}

export function cancelKitchenBatch(id: string): Promise<KitchenBatch> {
  return apiFetch<KitchenBatch>(buildUrl(`/kitchen/batches/${id}/cancel`), {
    method: "POST",
  });
}

export function deleteKitchenBatch(id: string): Promise<void> {
  return apiFetch<void>(buildUrl(`/kitchen/batches/${id}`), {
    method: "DELETE",
  });
}

export function excludeGitRepo(
  name: string,
  req: GitRepoExcludeRequest,
): Promise<void> {
  return apiFetch<void>(
    buildUrl(`/git-repos/${encodeURIComponent(name)}/exclude`),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export function clearGitRepoExclusion(name: string): Promise<void> {
  return apiFetch<void>(
    buildUrl(`/git-repos/${encodeURIComponent(name)}/exclude`),
    { method: "DELETE" },
  );
}

export async function listExcludedGitRepos(): Promise<GitRepoListItem[]> {
  return apiFetch<GitRepoListItem[]>("/git-repos/excluded");
}

export async function fetchBatchProgress(batchId: string): Promise<BatchProgress> {
  return apiFetch<BatchProgress>(`/kitchen/batches/${batchId}/progress`);
}

export async function fetchBatchInstances(
  batchId: string,
): Promise<KitchenBatchInstance[]> {
  return apiFetch<KitchenBatchInstance[]>(
    `/kitchen/batches/${batchId}/instances`,
  );
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

export function triggerGitKitchenRun(
  req: GitKitchenRunRequest,
): Promise<GitKitchenRunResponse> {
  return apiFetch<GitKitchenRunResponse>(buildUrl("/kitchen/git/run"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function triggerGitKitchenRunAll(
  req: GitKitchenRunAllRequest,
): Promise<GitKitchenRunAllResponse> {
  return apiFetch<GitKitchenRunAllResponse>(buildUrl("/kitchen/git/run-all"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function runOrphanSweep(dryRun: boolean): Promise<SweepResult> {
  return apiFetch<SweepResult>(`/kitchen/orphan-sweep?dry_run=${dryRun}`, {
    method: "POST",
  });
}

export async function testHypervisorConnection(): Promise<HypervisorTestConnectionResponse> {
  return apiFetch<HypervisorTestConnectionResponse>(
    buildUrl("/admin/hypervisor/test-connection"),
    { method: "POST" },
  );
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

export function createKitchenExclusion(
  req: CreateKitchenExclusionRequest,
): Promise<KitchenInstanceExclusion> {
  return apiFetch<KitchenInstanceExclusion>(buildUrl("/kitchen/git/exclusions"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function deleteKitchenExclusion(id: string): Promise<void> {
  return apiFetch<void>(buildUrl(`/kitchen/git/exclusions/${id}`), {
    method: "DELETE",
  });
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
