// SPDX-License-Identifier: Apache-2.0

import type {
  LoginRequest,
  LoginResponse,
  MeResponse,
  AuthInfoResponse,
  ApiToken,
  CreatedApiToken,
  OpenApiDocument,
} from "../types";
import { apiFetch, buildUrl } from "./client";

export function login(req: LoginRequest): Promise<LoginResponse> {
  return apiFetch<LoginResponse>(buildUrl("/auth/login"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function logout(): Promise<void> {
  return apiFetch<void>(buildUrl("/auth/logout"), { method: "POST" });
}

export function fetchMe(): Promise<MeResponse> {
  return apiFetch<MeResponse>(buildUrl("/auth/me"));
}

export function fetchAuthInfo(): Promise<AuthInfoResponse> {
  return apiFetch<AuthInfoResponse>(buildUrl("/auth/info"));
}

// API credentials, from the signed-in person's own record. There is no
// endpoint for anybody else's, so none of these take a username.

export function fetchMyApiTokens(): Promise<{ tokens: ApiToken[] }> {
  return apiFetch<{ tokens: ApiToken[] }>(buildUrl("/auth/me/tokens"));
}

// The response carries the secret. It is the only time it will, so whatever
// calls this has to show it rather than store it and move on.
export function createMyApiToken(
  name: string,
  canWrite: boolean,
): Promise<CreatedApiToken> {
  return apiFetch<CreatedApiToken>(buildUrl("/auth/me/tokens"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, can_write: canWrite }),
  });
}

export function destroyMyApiToken(id: string): Promise<void> {
  return apiFetch<void>(buildUrl(`/auth/me/tokens/${encodeURIComponent(id)}`), {
    method: "DELETE",
  });
}

// The service's own description of every address it serves. Open to any
// authenticated person on purpose: an assistant holding a viewer's credential
// has to read this to work at all.
export function fetchApiDocument(): Promise<OpenApiDocument> {
  return apiFetch<OpenApiDocument>(buildUrl("/openapi.json"));
}
