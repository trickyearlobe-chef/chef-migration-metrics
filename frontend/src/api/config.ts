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
  IngestConfig,
} from "../types/config";

// Re-export config types so consumers importing from "../api" still find them.
// ConfigOrganisation is also re-exported as Organisation for backward compat.
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
  CertMetadata,
  AcmeStatus,
  AuthConfig,
  AuthProvider,
  IngestConfig,
} from "../types/config";
import { apiFetch, buildUrl } from "./client";

export interface PutConfigResponse<T> {
  value: T;
  restartRequired: boolean;
  verdictsChanged?: number;
}

// Request body for the CSR generation endpoint. All fields
// except an identifier (common_name or a SAN) are optional.
export interface GenerateCSRRequest {
  common_name: string;
  organization?: string;
  organizational_unit?: string;
  country?: string;
  dns_sans?: string[];
  ip_sans?: string[];
  key_algorithm?: string;
}

// Response from the CSR generation endpoint. The CSR PEM is downloadable; the
// private key is stored as pending server-side and never returned.
export interface GenerateCSRResponse {
  csr_pem: string;
  key_algorithm: string;
}

function apiMutateConfig<T>(
  url: string,
  body: unknown,
): Promise<PutConfigResponse<T>> {
  return apiFetch<{ value: T; restart_required?: boolean; verdicts_changed?: number }>(url, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).then((envelope) => ({
    value: envelope.value,
    restartRequired: envelope.restart_required ?? false,
    verdictsChanged: envelope.verdicts_changed,
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

// Event ingest config (the ingest sink + Run events feature gates). Lives in its
// own section; surfaced under the Collection page for now (see todo-tech-debt:
// config UI refactor).
export function fetchIngestConfig(): Promise<IngestConfig> {
  return apiFetch<IngestConfig>(buildUrl("/admin/config/ingest"));
}

export function saveIngestConfig(
  value: IngestConfig,
): Promise<PutConfigResponse<IngestConfig>> {
  return apiMutateConfig<IngestConfig>(buildUrl("/admin/config/ingest"), value);
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

export interface AnalysisToolsGetResponse {
  value: AnalysisToolsConfig;
}

export function fetchAnalysisTools(): Promise<AnalysisToolsGetResponse> {
  return apiFetch<AnalysisToolsGetResponse>(
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
  // The certificate chain and the ACME status are read-only things the GET
  // attaches; the screen carries them in the same object it edits, so they came
  // back on every save. The service now refuses a body carrying anything it
  // does not read, so stripping them here is what keeps saving possible — and
  // what finally makes the "never sent on save" in the types true.
  const { tls_certificate_info: _chain, acme_status: _acme, ...settings } = value;
  return apiMutateConfig<ServerConfig>(
    buildUrl("/admin/config/server"),
    settings,
  );
}

export function fetchAuthConfig(): Promise<AuthConfig> {
  return apiFetch<AuthConfig>(buildUrl("/admin/config/auth"));
}

export function saveAuthConfig(
  value: AuthConfig,
): Promise<PutConfigResponse<AuthConfig>> {
  return apiMutateConfig<AuthConfig>(buildUrl("/admin/config/auth"), value);
}

// Generate a keypair + CSR for cert_source: db static issuance. The server
// stores the new private key as pending and returns only the CSR PEM; the
// operator submits it to their CA and pastes the signed cert back through the
// server-config save path, which match-and-promotes the pending key.
export function generateCSR(
  req: GenerateCSRRequest,
): Promise<GenerateCSRResponse> {
  return apiFetch<GenerateCSRResponse>(
    buildUrl("/admin/config/server/generate-csr"),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

// apiMutateConfig is exported so domain modules that use the same config
// PUT envelope (e.g. kitchen) can reuse it without duplicating the
// response-transformation logic.
export { apiMutateConfig };
