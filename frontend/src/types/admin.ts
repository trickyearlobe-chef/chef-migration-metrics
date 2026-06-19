// SPDX-License-Identifier: Apache-2.0

import type { PaginatedResponse } from "./common";

export interface AdminUser {
  username: string;
  display_name: string;
  email?: string;
  role: string;
  provider: string;
  locked: boolean;
  created_at: string;
  last_login_at?: string | null;
}

export type AdminUserListResponse = PaginatedResponse<AdminUser>;

export interface CreateUserRequest {
  username: string;
  display_name?: string;
  email?: string;
  password: string;
  role?: string;
}

export interface UpdateUserRequest {
  display_name?: string;
  email?: string;
  role?: string;
  locked?: boolean;
}

export interface ResetPasswordRequest {
  password: string;
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/status — operational health snapshot
// ---------------------------------------------------------------------------

export interface AdminStatusDatastore {
  status: string; // "connected" | "error"
  pending_migrations: number;
}

export interface AdminStatusCredentialStorage {
  encryption_key_configured: boolean;
  total_credentials: number;
  credential_types: Record<string, number>;
  orphaned_credentials: number;
}

export interface AdminStatusCollection {
  next_run_at: string | null;
  last_run_at: string | null;
  last_run_status: string;
}

export interface AdminStatusOrganisation {
  name: string;
  credential_source: string; // "file" | "database"
  last_collected_at: string | null;
  status: string; // collection run status, "never_collected", or "unknown"
  node_count: number;
}

export interface AdminStatus {
  status: string; // "healthy" | "degraded"
  version: string;
  datastore: AdminStatusDatastore;
  credential_storage: AdminStatusCredentialStorage;
  collection: AdminStatusCollection;
  organisations: AdminStatusOrganisation[];
}
