// SPDX-License-Identifier: Apache-2.0

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  expires_at: string;
  user: LoginUserInfo;
}

export interface LoginUserInfo {
  username: string;
  display_name: string;
  role: string;
}

export interface MeResponse {
  username: string;
  display_name: string;
  email?: string;
  role: string;
  provider: string;
}

export interface AuthInfoResponse {
  local_enabled: boolean;
  saml_enabled: boolean;
}

// An API credential in somebody's own record. Note what is not here: there is
// no field for the secret, because the listing does not carry one and never
// will.
export interface ApiToken {
  id: string;
  name: string;
  can_write: boolean;
  created_at: string;
  last_used_at?: string;
}

// What creating one returns. The only response that ever carries the secret.
export interface CreatedApiToken {
  token: ApiToken;
  secret: string;
  notice: string;
}

// The served OpenAPI document, as this service actually emits it. Deliberately
// narrow: there are no tags, no components and no parameter detail today, so a
// type promising them would make the page render empty sections that look like
// missing data rather than absent data.
export interface ApiParameter {
  name: string;
  // "path" today; "query" once the generator emits filters and pagination.
  in: string;
  required?: boolean;
  schema?: { type?: string };
}

export interface ApiOperation {
  operationId?: string;
  parameters?: ApiParameter[];
  summary?: string;
  description?: string;
  // The access this operation needs, folded in by the generator from both the
  // route table and the checks handlers make themselves.
  "x-required-role"?: string;
}

export interface OpenApiDocument {
  openapi?: string;
  info?: { title?: string; version?: string; description?: string };
  paths?: Record<string, Record<string, ApiOperation | undefined>>;
}
