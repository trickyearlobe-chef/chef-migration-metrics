// SPDX-License-Identifier: Apache-2.0

import type {
  CollectionConfig,
  ConcurrencyConfig,
  AnalysisToolsConfig,
  LoggingConfig,
  ExportsConfig,
  ReadinessConfig,
  ConfigOrganisation,
  ServerConfig,
  AuthConfig,
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
  ReadinessConfig,
  ConfigOrganisation,
  ConfigOrganisation as Organisation,
  ServerConfig,
  AuthConfig,
  AuthProvider,
} from "../types/config";
import { apiFetch, buildUrl } from "./client";

export interface PutConfigResponse<T> {
  value: T;
  restartRequired: boolean;
}

function apiMutateConfig<T>(
  url: string,
  body: unknown,
): Promise<PutConfigResponse<T>> {
  return apiFetch<{ value: T; restart_required?: boolean }>(url, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).then((envelope) => ({
    value: envelope.value,
    restartRequired: envelope.restart_required ?? false,
  }));
}

export function fetchGitURLs(): Promise<string[]> {
  return apiFetch<string[]>(buildUrl("/admin/config/git-urls"));
}

export function saveGitURLs(
  urls: string[],
): Promise<PutConfigResponse<string[]>> {
  return apiMutateConfig<string[]>(buildUrl("/admin/config/git-urls"), urls);
}

export function fetchCollection(): Promise<CollectionConfig> {
  return apiFetch<CollectionConfig>(buildUrl("/admin/config/collection"));
}

export function saveCollection(
  value: CollectionConfig,
): Promise<PutConfigResponse<CollectionConfig>> {
  return apiMutateConfig<CollectionConfig>(
    buildUrl("/admin/config/collection"),
    value,
  );
}

export function fetchTargetVersions(): Promise<string[]> {
  return apiFetch<string[]>(buildUrl("/admin/config/target-versions"));
}

export function saveTargetVersions(
  versions: string[],
): Promise<PutConfigResponse<string[]>> {
  return apiMutateConfig<string[]>(
    buildUrl("/admin/config/target-versions"),
    versions,
  );
}

export function fetchConcurrency(): Promise<ConcurrencyConfig> {
  return apiFetch<ConcurrencyConfig>(buildUrl("/admin/config/concurrency"));
}

export function saveConcurrency(
  value: ConcurrencyConfig,
): Promise<PutConfigResponse<ConcurrencyConfig>> {
  return apiMutateConfig<ConcurrencyConfig>(
    buildUrl("/admin/config/concurrency"),
    value,
  );
}

export function fetchAnalysisTools(): Promise<AnalysisToolsConfig> {
  return apiFetch<AnalysisToolsConfig>(
    buildUrl("/admin/config/analysis-tools"),
  );
}

export function saveAnalysisTools(
  value: AnalysisToolsConfig,
): Promise<PutConfigResponse<AnalysisToolsConfig>> {
  return apiMutateConfig<AnalysisToolsConfig>(
    buildUrl("/admin/config/analysis-tools"),
    value,
  );
}

export function fetchLogging(): Promise<LoggingConfig> {
  return apiFetch<LoggingConfig>(buildUrl("/admin/config/logging"));
}

export function saveLogging(
  value: LoggingConfig,
): Promise<PutConfigResponse<LoggingConfig>> {
  return apiMutateConfig<LoggingConfig>(buildUrl("/admin/config/logging"), value);
}

export function fetchExportsConfig(): Promise<ExportsConfig> {
  return apiFetch<ExportsConfig>(buildUrl("/admin/config/exports"));
}

export function saveExportsConfig(
  value: ExportsConfig,
): Promise<PutConfigResponse<ExportsConfig>> {
  return apiMutateConfig<ExportsConfig>(buildUrl("/admin/config/exports"), value);
}

export function fetchReadinessConfig(): Promise<ReadinessConfig> {
  return apiFetch<ReadinessConfig>(buildUrl("/admin/config/readiness"));
}

export function saveReadinessConfig(
  value: ReadinessConfig,
): Promise<PutConfigResponse<ReadinessConfig>> {
  return apiMutateConfig<ReadinessConfig>(buildUrl("/admin/config/readiness"), value);
}

export function fetchConfigOrganisations(): Promise<ConfigOrganisation[]> {
  return apiFetch<ConfigOrganisation[]>(
    buildUrl("/admin/config/organisations"),
  );
}

export function saveConfigOrganisations(
  orgs: ConfigOrganisation[],
): Promise<PutConfigResponse<ConfigOrganisation[]>> {
  return apiMutateConfig<ConfigOrganisation[]>(
    buildUrl("/admin/config/organisations"),
    orgs,
  );
}

export function fetchServerConfig(): Promise<ServerConfig> {
  return apiFetch<ServerConfig>(buildUrl("/admin/config/server"));
}

export function saveServerConfig(
  value: ServerConfig,
): Promise<PutConfigResponse<ServerConfig>> {
  return apiMutateConfig<ServerConfig>(buildUrl("/admin/config/server"), value);
}

export function fetchAuthConfig(): Promise<AuthConfig> {
  return apiFetch<AuthConfig>(buildUrl("/admin/config/auth"));
}

export function saveAuthConfig(
  value: AuthConfig,
): Promise<PutConfigResponse<AuthConfig>> {
  return apiMutateConfig<AuthConfig>(buildUrl("/admin/config/auth"), value);
}

// apiMutateConfig is exported so domain modules that use the same config
// PUT envelope (e.g. kitchen) can reuse it without duplicating the
// response-transformation logic.
export { apiMutateConfig };
