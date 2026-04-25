// SPDX-License-Identifier: Apache-2.0

import type {
  Credential,
  CredentialListResponse,
  CreateCredentialRequest,
  UpdateCredentialRequest,
  TestCredentialResponse,
} from "../types";
import { apiFetch, buildUrl, ApiError } from "./client";
import type { PaginationQuery } from "./client";

export function fetchCredentials(
  filters?: PaginationQuery & { search?: string; sort?: string; order?: string },
): Promise<CredentialListResponse> {
  return apiFetch<CredentialListResponse>(
    buildUrl("/admin/credentials", filters as Record<string, string | number | boolean | undefined>),
  );
}

export async function createCredential(
  req: CreateCredentialRequest,
): Promise<Credential> {
  const url = buildUrl("/admin/credentials");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(req),
  });
  if (res.ok) return res.json() as Promise<Credential>;
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

export async function updateCredential(
  name: string,
  req: UpdateCredentialRequest,
): Promise<Credential> {
  const url = buildUrl(`/admin/credentials/${encodeURIComponent(name)}`);
  const res = await fetch(url, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(req),
  });
  if (res.ok) return res.json() as Promise<Credential>;
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

export async function deleteCredential(name: string): Promise<void> {
  const url = buildUrl(
    `/admin/credentials/${encodeURIComponent(name)}?confirm=true`,
  );
  const res = await fetch(url, {
    method: "DELETE",
    headers: {
      Accept: "application/json",
    },
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

export async function testCredential(name: string): Promise<TestCredentialResponse> {
  const url = buildUrl(`/admin/credentials/${encodeURIComponent(name)}/test`);
  const res = await fetch(url, {
    method: "POST",
    headers: {
      Accept: "application/json",
    },
  });
  if (res.ok) return res.json() as Promise<TestCredentialResponse>;
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
