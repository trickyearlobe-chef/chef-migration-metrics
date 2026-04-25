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
