// SPDX-License-Identifier: Apache-2.0

import type { PaginatedResponse } from "./common";

export interface Credential {
  name: string;
  credential_type: string;
  metadata?: Record<string, unknown>;
  last_rotated_at?: string | null;
  created_by: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

export type CredentialListResponse = PaginatedResponse<Credential>;

export interface CreateCredentialRequest {
  name: string;
  credential_type: string;
  value: string;
}

export interface UpdateCredentialRequest {
  value: string;
}

export interface TestCredentialResponse {
  valid: boolean;
  error?: string | null;
  metadata?: Record<string, unknown> | null;
}
