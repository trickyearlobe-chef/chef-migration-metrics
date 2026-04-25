// SPDX-License-Identifier: Apache-2.0

import type {
  CollectionConfig,
  ConcurrencyConfig,
  AnalysisToolsConfig,
  LoggingConfig,
  ExportsConfig,
  ConfigOrganisation,
  ServerConfig,
  AuthConfig,
  NotificationsConfig,
} from "../types/config";

// Re-export config types so consumers importing from "../api" still find them.
// ConfigOrganisation is also re-exported as Organisation for backward compat
// (the old api.ts defined a local Organisation interface for config).
export type {
  CollectionConfig,
  ConcurrencyConfig,
  AnalysisToolsConfig,
  LoggingConfig,
  ExportsConfig,
  ConfigOrganisation,
  ConfigOrganisation as Organisation,
  ServerConfig,
  AuthConfig,
  AuthProvider,
  NotificationsConfig,
  NotificationChannel,
} from "../types/config";
import { apiFetch, buildUrl, ApiError } from "./client";

export interface PutConfigResponse<T> {
  value: T;
  restartRequired: boolean;
}

export async function decodePutConfigResponse<T>(
  res: Response,
): Promise<PutConfigResponse<T>> {
  if (!res.ok) {
    let message = res.statusText || `HTTP ${res.status}`;
    try {
      const body = await res.text();
      try {
        const parsed = JSON.parse(body);
        message = parsed.message || parsed.error || message;
      } catch {
        /* ignore */
      }
      throw new ApiError(res.status, message, body);
    } catch (e) {
      if (e instanceof ApiError) throw e;
      throw new ApiError(res.status, message, "");
    }
  }
  const envelope = (await res.json()) as {
    value: T;
    restart_required?: boolean;
  };
  return {
    value: envelope.value ?? (envelope as unknown as T),
    restartRequired: envelope.restart_required ?? false,
  };
}

export function fetchGitURLs(): Promise<string[]> {
  return apiFetch<string[]>(buildUrl("/admin/config/git-urls"));
}

export async function saveGitURLs(
  urls: string[],
): Promise<PutConfigResponse<string[]>> {
  const res = await fetch(buildUrl("/admin/config/git-urls"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(urls),
  });
  return decodePutConfigResponse<string[]>(res);
}

export function fetchCollection(): Promise<CollectionConfig> {
  return apiFetch<CollectionConfig>(buildUrl("/admin/config/collection"));
}

export async function saveCollection(
  value: CollectionConfig,
): Promise<PutConfigResponse<CollectionConfig>> {
  const res = await fetch(buildUrl("/admin/config/collection"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(value),
  });
  return decodePutConfigResponse<CollectionConfig>(res);
}

export function fetchTargetVersions(): Promise<string[]> {
  return apiFetch<string[]>(buildUrl("/admin/config/target-versions"));
}

export async function saveTargetVersions(
  versions: string[],
): Promise<PutConfigResponse<string[]>> {
  const res = await fetch(buildUrl("/admin/config/target-versions"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(versions),
  });
  return decodePutConfigResponse<string[]>(res);
}

export function fetchConcurrency(): Promise<ConcurrencyConfig> {
  return apiFetch<ConcurrencyConfig>(buildUrl("/admin/config/concurrency"));
}

export async function saveConcurrency(
  value: ConcurrencyConfig,
): Promise<PutConfigResponse<ConcurrencyConfig>> {
  const res = await fetch(buildUrl("/admin/config/concurrency"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(value),
  });
  return decodePutConfigResponse<ConcurrencyConfig>(res);
}

export function fetchAnalysisTools(): Promise<AnalysisToolsConfig> {
  return apiFetch<AnalysisToolsConfig>(
    buildUrl("/admin/config/analysis-tools"),
  );
}

export async function saveAnalysisTools(
  value: AnalysisToolsConfig,
): Promise<PutConfigResponse<AnalysisToolsConfig>> {
  const res = await fetch(buildUrl("/admin/config/analysis-tools"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(value),
  });
  return decodePutConfigResponse<AnalysisToolsConfig>(res);
}

export function fetchLogging(): Promise<LoggingConfig> {
  return apiFetch<LoggingConfig>(buildUrl("/admin/config/logging"));
}

export async function saveLogging(
  value: LoggingConfig,
): Promise<PutConfigResponse<LoggingConfig>> {
  const res = await fetch(buildUrl("/admin/config/logging"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(value),
  });
  return decodePutConfigResponse<LoggingConfig>(res);
}

export function fetchExportsConfig(): Promise<ExportsConfig> {
  return apiFetch<ExportsConfig>(buildUrl("/admin/config/exports"));
}

export async function saveExportsConfig(
  value: ExportsConfig,
): Promise<PutConfigResponse<ExportsConfig>> {
  const res = await fetch(buildUrl("/admin/config/exports"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(value),
  });
  return decodePutConfigResponse<ExportsConfig>(res);
}

export function fetchConfigOrganisations(): Promise<ConfigOrganisation[]> {
  return apiFetch<ConfigOrganisation[]>(
    buildUrl("/admin/config/organisations"),
  );
}

export async function saveConfigOrganisations(
  orgs: ConfigOrganisation[],
): Promise<PutConfigResponse<ConfigOrganisation[]>> {
  const res = await fetch(buildUrl("/admin/config/organisations"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(orgs),
  });
  return decodePutConfigResponse<ConfigOrganisation[]>(res);
}

export function fetchServerConfig(): Promise<ServerConfig> {
  return apiFetch<ServerConfig>(buildUrl("/admin/config/server"));
}

export async function saveServerConfig(
  value: ServerConfig,
): Promise<PutConfigResponse<ServerConfig>> {
  const res = await fetch(buildUrl("/admin/config/server"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(value),
  });
  return decodePutConfigResponse<ServerConfig>(res);
}

export function fetchAuthConfig(): Promise<AuthConfig> {
  return apiFetch<AuthConfig>(buildUrl("/admin/config/auth"));
}

export async function saveAuthConfig(
  value: AuthConfig,
): Promise<PutConfigResponse<AuthConfig>> {
  const res = await fetch(buildUrl("/admin/config/auth"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(value),
  });
  return decodePutConfigResponse<AuthConfig>(res);
}

export function fetchNotifications(): Promise<NotificationsConfig> {
  return apiFetch<NotificationsConfig>(buildUrl("/admin/config/notifications"));
}

export async function saveNotifications(
  value: NotificationsConfig,
): Promise<PutConfigResponse<NotificationsConfig>> {
  const res = await fetch(buildUrl("/admin/config/notifications"), {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(value),
  });
  return decodePutConfigResponse<NotificationsConfig>(res);
}
