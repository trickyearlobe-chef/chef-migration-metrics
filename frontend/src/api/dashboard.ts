// SPDX-License-Identifier: Apache-2.0

import type {
  VersionDistributionResponse,
  PlatformDistributionResponse,
  ReadinessResponse,
  VersionDistributionTrendResponse,
  ReadinessTrendResponse,
  ComplexityTrendResponse,
  StaleTrendResponse,
  DeploymentTrendResponse,
  DeploymentStatusResponse,
  CookbookCompatibilityResponse,
  GitRepoCompatibilityResponse,
  TestKitchenCompatibilityResponse,
  SystemHealthResponse,
  PerformanceResponse,
  PerformanceDBResponse,
} from "../types";
import { apiFetch, buildUrl, ApiError } from "./client";

export function fetchVersionDistribution(
  organisation?: string,
): Promise<VersionDistributionResponse> {
  return apiFetch<VersionDistributionResponse>(
    buildUrl("/dashboard/version-distribution", { organisation }),
  );
}

export function fetchPlatformDistribution(
  organisation?: string,
): Promise<PlatformDistributionResponse> {
  return apiFetch<PlatformDistributionResponse>(
    buildUrl("/dashboard/platform-distribution", { organisation }),
  );
}

export function fetchReadiness(organisation?: string): Promise<ReadinessResponse> {
  return apiFetch<ReadinessResponse>(
    buildUrl("/dashboard/readiness", { organisation }),
  );
}

export function fetchVersionDistributionTrend(
  organisation?: string,
): Promise<VersionDistributionTrendResponse> {
  return apiFetch<VersionDistributionTrendResponse>(
    buildUrl("/dashboard/version-distribution/trend", { organisation }),
  );
}

export function fetchReadinessTrend(
  organisation?: string,
  stale?: string,
): Promise<ReadinessTrendResponse> {
  return apiFetch<ReadinessTrendResponse>(
    buildUrl("/dashboard/readiness/trend", { organisation, stale }),
  );
}

export function fetchComplexityTrend(
  organisation?: string,
): Promise<ComplexityTrendResponse> {
  return apiFetch<ComplexityTrendResponse>(
    buildUrl("/dashboard/complexity/trend", { organisation }),
  );
}

export function fetchStaleTrend(organisation?: string): Promise<StaleTrendResponse> {
  return apiFetch<StaleTrendResponse>(
    buildUrl("/dashboard/stale/trend", { organisation }),
  );
}

export function fetchDeploymentTrend(
  organisation?: string,
): Promise<DeploymentTrendResponse> {
  return apiFetch<DeploymentTrendResponse>(
    buildUrl("/dashboard/deployment/trend", { organisation }),
  );
}

export function fetchDeploymentStatus(
  organisation?: string,
): Promise<DeploymentStatusResponse> {
  return apiFetch<DeploymentStatusResponse>(
    buildUrl("/dashboard/deployment/status", { organisation }),
  );
}

export function fetchCookbookCompatibility(
  organisation?: string,
): Promise<CookbookCompatibilityResponse> {
  return apiFetch<CookbookCompatibilityResponse>(
    buildUrl("/dashboard/cookbook-compatibility", { organisation }),
  );
}

export function fetchGitRepoCompatibility(
  organisation?: string,
): Promise<GitRepoCompatibilityResponse> {
  return apiFetch<GitRepoCompatibilityResponse>(
    buildUrl("/dashboard/git-repo-compatibility", { organisation }),
  );
}

export function fetchTestKitchenCompatibility(
  organisation?: string,
): Promise<TestKitchenCompatibilityResponse> {
  return apiFetch<TestKitchenCompatibilityResponse>(
    buildUrl("/dashboard/test-kitchen-compatibility", { organisation }),
  );
}

export function fetchSystemHealth(): Promise<SystemHealthResponse> {
  return apiFetch<SystemHealthResponse>(buildUrl("/admin/system-health"));
}

export function fetchPerformanceStats(): Promise<PerformanceResponse> {
  return apiFetch<PerformanceResponse>(buildUrl("/admin/performance"));
}

export async function resetPerformanceStats(): Promise<void> {
  const res = await fetch(buildUrl("/admin/performance/reset"), {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    const code = res.status;
    let message = res.statusText || `HTTP ${res.status}`;
    try {
      const body = await res.text();
      const parsed = JSON.parse(body);
      message = parsed.message || parsed.error || message;
    } catch { /* ignore */ }
    throw new ApiError(code, message, "");
  }
}

export function fetchPerformanceDB(): Promise<PerformanceDBResponse> {
  return apiFetch<PerformanceDBResponse>(buildUrl("/admin/performance/db"));
}

export async function resetPerformanceDB(): Promise<void> {
  const res = await fetch(buildUrl("/admin/performance/db/reset"), {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    const code = res.status;
    let message = res.statusText || `HTTP ${res.status}`;
    try {
      const body = await res.text();
      const parsed = JSON.parse(body);
      message = parsed.message || parsed.error || message;
    } catch { /* ignore */ }
    throw new ApiError(code, message, "");
  }
}

export async function vacuumFull(): Promise<void> {
  const res = await fetch(buildUrl("/admin/performance/vacuum"), {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    const code = res.status;
    let message = res.statusText || `HTTP ${res.status}`;
    try {
      const body = await res.text();
      const parsed = JSON.parse(body);
      message = parsed.message || parsed.error || message;
    } catch { /* ignore */ }
    throw new ApiError(code, message, "");
  }
}
