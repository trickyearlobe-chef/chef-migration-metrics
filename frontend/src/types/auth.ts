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
// narrow: there are no tags and no response detail today, so a type promising
// them would make the page render empty sections that look like missing data
// rather than absent data.

// ApiSchema is as much of JSON Schema as the generator emits. A named type is
// emitted once under components and referred to, so anything reading a field's
// type has to be prepared to follow a reference.
export interface ApiSchema {
  $ref?: string;
  type?: string;
  format?: string;
  // Bounds, on the pagination parameters. The maximum is load-bearing: the
  // service clamps rather than refusing, so a caller that cannot see it never
  // finds out it was capped.
  minimum?: number;
  maximum?: number;
  default?: unknown;
  properties?: Record<string, ApiSchema>;
  items?: ApiSchema;
  additionalProperties?: ApiSchema;
  allOf?: ApiSchema[];
  // One of several shapes, which is how an address that answers differently
  // depending on a parameter is described. A caller has to branch.
  oneOf?: ApiSchema[];
}

export interface ApiParameter {
  name: string;
  // "path" today; "query" once the generator emits filters and pagination.
  in: string;
  required?: boolean;
  schema?: ApiSchema;
}

export interface ApiRequestBody {
  required?: boolean;
  // Present when the body is deliberately not described in full — an upload,
  // or telemetry whose shape this service does not decide. The reason is
  // served so a reader is not left assuming it was forgotten.
  description?: string;
  content?: Record<string, { schema?: ApiSchema }>;
}

// ApiResponse is one outcome of a call. Only the successful one carries a
// schema today; a failure is the same error document everywhere, described in
// prose on the page rather than repeated on every operation.
export interface ApiResponse {
  description?: string;
  content?: Record<string, { schema?: ApiSchema }>;
}

export interface ApiOperation {
  operationId?: string;
  parameters?: ApiParameter[];
  requestBody?: ApiRequestBody;
  // Keyed by status code as a string, which is how OpenAPI writes it.
  responses?: Record<string, ApiResponse>;
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
  components?: { schemas?: Record<string, ApiSchema> };
}
