// SPDX-License-Identifier: Apache-2.0

import type { LoginRequest, LoginResponse, MeResponse } from "../types";
import { apiFetch, buildUrl, ApiError } from "./client";

export async function login(req: LoginRequest): Promise<LoginResponse> {
  const url = buildUrl("/auth/login");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(req),
  });

  if (res.ok) return res.json() as Promise<LoginResponse>;

  let code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const parsed = JSON.parse(errBody);
      message = parsed.message || parsed.error || message;
    } catch { /* ignore */ }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export async function logout(): Promise<void> {
  const url = buildUrl("/auth/logout");
  const res = await fetch(url, {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new ApiError(res.status, "Logout failed", "");
}

export function fetchMe(): Promise<MeResponse> {
  return apiFetch<MeResponse>(buildUrl("/auth/me"));
}
