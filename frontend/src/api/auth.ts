// SPDX-License-Identifier: Apache-2.0

import type { LoginRequest, LoginResponse, MeResponse } from "../types";
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
