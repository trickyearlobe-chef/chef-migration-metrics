// SPDX-License-Identifier: Apache-2.0

import type {
  Credential,
  CredentialListResponse,
  CreateCredentialRequest,
  UpdateCredentialRequest,
  TestCredentialResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";
import type { PaginationQuery } from "./client";

export function fetchCredentials(
  filters?: PaginationQuery & { search?: string; sort?: string; order?: string },
): Promise<CredentialListResponse> {
  return apiFetch<CredentialListResponse>(
    buildUrl("/admin/credentials", filters as Record<string, string | number | boolean | undefined>),
  );
}

export function createCredential(
  req: CreateCredentialRequest,
): Promise<Credential> {
  return apiFetch<Credential>(buildUrl("/admin/credentials"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function updateCredential(
  name: string,
  req: UpdateCredentialRequest,
): Promise<Credential> {
  return apiFetch<Credential>(
    buildUrl(`/admin/credentials/${encodeURIComponent(name)}`),
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export function deleteCredential(name: string): Promise<void> {
  return apiFetch<void>(
    buildUrl(`/admin/credentials/${encodeURIComponent(name)}?confirm=true`),
    { method: "DELETE" },
  );
}

export function testCredential(name: string): Promise<TestCredentialResponse> {
  return apiFetch<TestCredentialResponse>(
    buildUrl(`/admin/credentials/${encodeURIComponent(name)}/test`),
    { method: "POST" },
  );
}
