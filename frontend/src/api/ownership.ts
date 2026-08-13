// SPDX-License-Identifier: Apache-2.0

import type {
  OwnerListResponse,
  OwnerDetail,
  Owner,
  AssignmentListResponse,
  AuditLogResponse,
  OwnershipLookupResponse,
  ReassignResponse,
  CookbookCommittersResponse,
  CommitterAssignResponse,
  IntakeFieldMap,
  IntakeMapping,
  IntakeReport,
  IntakeSourceProfile,
  Pagination,
} from "../types";
import { apiFetch, buildUrl } from "./client";
import type { PaginationQuery } from "./client";

export interface OwnerFilterQuery extends PaginationQuery {
  owner_type?: string;
  search?: string;
  sort?: string;
  order?: string;
  target_chef_version?: string;
}

export interface AssignmentFilterQuery extends PaginationQuery {
  entity_type?: string;
  organisation?: string;
  assignment_source?: string;
}

export interface AuditLogFilterQuery extends PaginationQuery {
  action?: string;
  actor?: string;
  owner_name?: string;
  entity_type?: string;
  entity_key?: string;
  since?: string;
  until?: string;
}

export interface CommitterFilterQuery extends PaginationQuery {
  search?: string;
  sort?: string;
  order?: string;
  since?: string;
}

export function fetchOwners(
  filters?: OwnerFilterQuery,
): Promise<OwnerListResponse> {
  return apiFetch<OwnerListResponse>(
    buildUrl(
      "/owners",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchOwnerDetail(
  name: string,
  params?: { target_chef_version?: string },
): Promise<OwnerDetail> {
  return apiFetch<OwnerDetail>(
    buildUrl(`/owners/${encodeURIComponent(name)}`, params),
  );
}

export function createOwner(body: {
  name: string;
  owner_type: string;
  display_name?: string;
  contact_email?: string;
  contact_channel?: string;
  metadata?: Record<string, unknown>;
}): Promise<Owner> {
  return apiFetch<Owner>(buildUrl("/owners"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function updateOwner(
  name: string,
  body: {
    display_name?: string;
    contact_email?: string;
    contact_channel?: string;
    owner_type?: string;
    metadata?: Record<string, unknown>;
  },
): Promise<Owner> {
  return apiFetch<Owner>(buildUrl(`/owners/${encodeURIComponent(name)}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function deleteOwner(name: string): Promise<void> {
  return apiFetch<void>(buildUrl(`/owners/${encodeURIComponent(name)}`), {
    method: "DELETE",
  });
}

export function fetchAssignments(
  ownerName: string,
  filters?: AssignmentFilterQuery,
): Promise<AssignmentListResponse> {
  return apiFetch<AssignmentListResponse>(
    buildUrl(
      `/owners/${encodeURIComponent(ownerName)}/assignments`,
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function createAssignments(
  ownerName: string,
  body: {
    assignments: {
      entity_type: string;
      entity_key: string;
      organisation?: string;
      notes?: string;
    }[];
  },
): Promise<{ created: number; assignments: unknown[] }> {
  return apiFetch<{ created: number; assignments: unknown[] }>(
    buildUrl(`/owners/${encodeURIComponent(ownerName)}/assignments`),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}

export function deleteAssignment(
  ownerName: string,
  assignmentId: string,
): Promise<void> {
  return apiFetch<void>(
    buildUrl(
      `/owners/${encodeURIComponent(ownerName)}/assignments/${encodeURIComponent(assignmentId)}`,
    ),
    { method: "DELETE" },
  );
}

export function reassignOwnership(body: {
  from_owner: string;
  to_owner: string;
  entity_type?: string;
  organisation?: string;
  delete_source_owner?: boolean;
}): Promise<ReassignResponse> {
  return apiFetch<ReassignResponse>(buildUrl("/ownership/reassign"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function fetchOwnershipLookup(params: {
  entity_type: string;
  entity_key: string;
  organisation?: string;
}): Promise<OwnershipLookupResponse> {
  return apiFetch<OwnershipLookupResponse>(
    buildUrl("/ownership/lookup", params),
  );
}

export function fetchAuditLog(
  filters?: AuditLogFilterQuery,
): Promise<AuditLogResponse> {
  return apiFetch<AuditLogResponse>(
    buildUrl(
      "/ownership/audit-log",
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function fetchCookbookCommitters(
  cookbookName: string,
  filters?: CommitterFilterQuery,
): Promise<CookbookCommittersResponse> {
  return apiFetch<CookbookCommittersResponse>(
    buildUrl(
      `/cookbooks/${encodeURIComponent(cookbookName)}/committers`,
      filters as Record<string, string | number | boolean | undefined>,
    ),
  );
}

export function assignCookbookCommitters(
  cookbookName: string,
  body: {
    committers: {
      author_email: string;
      owner_name: string;
      display_name?: string;
    }[];
  },
): Promise<CommitterAssignResponse> {
  return apiFetch<CommitterAssignResponse>(
    buildUrl(`/cookbooks/${encodeURIComponent(cookbookName)}/committers/assign`),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}

// ---------------------------------------------------------------------------
// Discovery-driven ownership intake
//
// Profile, preview and commit each open their own source, so each call carries
// its own file and delimiter. Nothing is remembered between them on the server.
// ---------------------------------------------------------------------------

/** Where the rows come from. A file that was uploaded, or a query against a
 * database reached through a connection that was set up beforehand.
 *
 * Only the NAME of the connection is sent. The connection itself, and the
 * credential holding its password, are read on the server — so no password
 * travels through the browser, and which database reads it is not asked twice:
 * the connection already says. */
export interface IntakeDatabaseSource {
  /** Name of a connection set up under "Where the rows come from". */
  connection: string;
  query: string;
}

export interface IntakeRunOptions {
  /** Required unless `database` is given. */
  file?: File;
  database?: IntakeDatabaseSource;
  delimiter?: string;
  fieldMap?: IntakeFieldMap;
  mappingId?: number;
  createOwners?: boolean;
  /** Import only rows whose filterColumn equals filterValue, compared
   * case-insensitively. Lets a consolidated export holding several kinds of
   * row be imported one kind at a time. */
  filterColumn?: string;
  filterValue?: string;
}

function intakeFormData(opts: IntakeRunOptions): FormData {
  const formData = new FormData();
  if (opts.database) {
    appendDatabaseSource(formData, opts.database);
  } else if (opts.file) {
    formData.append("file", opts.file);
  }
  if (opts.delimiter) formData.append("delimiter", opts.delimiter);
  if (opts.fieldMap) formData.append("field_map", JSON.stringify(opts.fieldMap));
  if (opts.mappingId !== undefined) formData.append("mapping_id", String(opts.mappingId));
  // Only send the flag when switching creation off — the default is on.
  if (opts.createOwners === false) formData.append("create_owners", "false");
  if (opts.filterColumn) {
    formData.append("filter_column", opts.filterColumn);
    formData.append("filter_value", opts.filterValue ?? "");
  }
  return formData;
}

/** The database half of the multipart body the server expects. */
function appendDatabaseSource(formData: FormData, db: IntakeDatabaseSource) {
  formData.append("source_type", "database");
  formData.append("db_connection", db.connection);
  formData.append("db_query", db.query);
}

export function profileImportSource(
  file: File,
  delimiter?: string,
): Promise<IntakeSourceProfile> {
  const formData = new FormData();
  formData.append("file", file);
  if (delimiter) formData.append("delimiter", delimiter);
  return apiFetch<IntakeSourceProfile>(buildUrl("/ownership/import/profile"), {
    method: "POST",
    body: formData,
  });
}

/** One table or view a connection can see. */
export interface IntakeDatabaseTable {
  schema: string;
  name: string;
  kind: "table" | "view";
  /** The name quoted for its database, ready to drop into a query. */
  qualified_name: string;
}

/** List what a connection can see, so a table can be chosen rather than typed.
 * Whoever sets the import up often cannot inspect the database themselves. */
export function listImportDatabaseTables(
  connection: string,
): Promise<{ data: IntakeDatabaseTable[] }> {
  const formData = new FormData();
  formData.append("db_connection", connection);
  return apiFetch<{ data: IntakeDatabaseTable[] }>(
    buildUrl("/ownership/import/tables"),
    { method: "POST", body: formData },
  );
}

/** Profile what a query returns, so the mapping screen can offer its columns. */
export function profileImportDatabase(
  db: IntakeDatabaseSource,
): Promise<IntakeSourceProfile> {
  const formData = new FormData();
  appendDatabaseSource(formData, db);
  return apiFetch<IntakeSourceProfile>(buildUrl("/ownership/import/profile"), {
    method: "POST",
    body: formData,
  });
}

export function previewOwnershipImport(opts: IntakeRunOptions): Promise<IntakeReport> {
  return apiFetch<IntakeReport>(buildUrl("/ownership/import/preview"), {
    method: "POST",
    body: intakeFormData(opts),
  });
}

export function commitOwnershipImport(opts: IntakeRunOptions): Promise<IntakeReport> {
  return apiFetch<IntakeReport>(buildUrl("/ownership/import/commit"), {
    method: "POST",
    body: intakeFormData(opts),
  });
}

export function fetchImportMappings(): Promise<{
  data: IntakeMapping[];
  pagination: Pagination;
}> {
  return apiFetch(buildUrl("/ownership/import/mappings"));
}

export function fetchImportMapping(id: number): Promise<IntakeMapping> {
  return apiFetch<IntakeMapping>(buildUrl(`/ownership/import/mappings/${id}`));
}

export function createImportMapping(body: {
  name: string;
  delimiter: string;
  field_map: IntakeFieldMap;
  // A database import saves its source too, so it can be re-run — and, with a
  // schedule, re-run with nobody present. A file import has none of this:
  // somebody has to bring the file.
  source_kind?: string;
  db_connection?: string;
  db_query?: string;
  filter_column?: string;
  filter_value?: string;
  create_owners?: boolean;
  schedule?: string;
  schedule_enabled?: boolean;
}): Promise<IntakeMapping> {
  return apiFetch<IntakeMapping>(buildUrl("/ownership/import/mappings"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source_kind: "csv", ...body }),
  });
}

// Run a saved database import now. Synchronous: it answers with what the run
// did, because it exists for judging a source rather than for automation.
export function runImportNow(
  id: number,
): Promise<{ summary: ImportRunSummary; detail: string }> {
  return apiFetch(buildUrl(`/ownership/import/mappings/${id}/run`), {
    method: "POST",
  });
}

export interface ImportRunSummary {
  row_count: number;
  filtered_out: number;
  counts: Record<string, number>;
}

export interface ClearedOwnership {
  assignments: number;
  owners: number;
}

// What a clear-down would remove. Read before asking somebody to confirm it,
// so the confirmation can name a number.
export function previewClearImportedOwnership(): Promise<ClearedOwnership> {
  return apiFetch<ClearedOwnership>(buildUrl("/ownership/import/clear"));
}

export function clearImportedOwnership(): Promise<ClearedOwnership> {
  return apiFetch<ClearedOwnership>(buildUrl("/ownership/import/clear"), {
    method: "POST",
  });
}

export function deleteImportMapping(id: number): Promise<void> {
  return apiFetch<void>(buildUrl(`/ownership/import/mappings/${id}`), {
    method: "DELETE",
  });
}

// ---------------------------------------------------------------------------
// Import rejections — the rows an import could not use
// ---------------------------------------------------------------------------

/**
 * One row an import could not use. Stored since imports existed, but readable
 * only by taking an export until now — so an import that dropped a quarter of
 * its rows said nothing to the person who could get the source fixed.
 */
export interface ImportRejection {
  import_label: string;
  run_at: string;
  source_row: number;
  reason: string;
  owner_raw?: string;
  entity_type?: string;
  entity_key?: string;
}

export function fetchImportRejections(params?: {
  page?: number;
  per_page?: number;
}): Promise<{ data: ImportRejection[]; pagination: Pagination }> {
  return apiFetch<{ data: ImportRejection[]; pagination: Pagination }>(
    buildUrl("/ownership/import/rejections", {
      page: params?.page,
      per_page: params?.per_page,
    }),
  );
}

// ---------------------------------------------------------------------------
// Setting up the connection an import reads through
//
// See journeys/ownership-connection.md. The address, the database and the
// account are configuration the administrator reads and edits; only the
// password is a secret, held as a credential and named here. The marker below
// is where it goes — it is shown on screen because a marker nobody can read
// about is just a new thing to get wrong.
// ---------------------------------------------------------------------------

/** Where the password goes in a connection. Written out here rather than
 * fetched: the screen has to show it before any call is made, including the
 * very first one. A Go test reads this line and compares it with the marker the
 * server really substitutes, so the two cannot drift — a screen telling
 * somebody to write a marker the server does not recognise would refuse every
 * connection typed from it. */
export const PASSWORD_MARKER = "PASSWORD_GOES_HERE";

/** A connection an import can read through. The password is not here: it lives
 * in the credential named by password_credential. */
export interface OwnershipConnection {
  name: string;
  driver: string;
  connection: string;
  password_credential: string;
  updated_at?: string;
  updated_by?: string;
}

export function listOwnershipConnections(): Promise<{
  data: OwnershipConnection[];
}> {
  return apiFetch<{ data: OwnershipConnection[] }>(
    buildUrl("/ownership/import/connections"),
  );
}

export function saveOwnershipConnection(body: {
  name: string;
  driver?: string;
  connection: string;
  password_credential: string;
}): Promise<OwnershipConnection> {
  return apiFetch<OwnershipConnection>(buildUrl("/ownership/import/connections"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function deleteOwnershipConnection(
  name: string,
): Promise<{ deleted: string }> {
  return apiFetch<{ deleted: string }>(
    buildUrl(`/ownership/import/connections/${encodeURIComponent(name)}`),
    { method: "DELETE" },
  );
}

/** What a connection composes to, with the password masked. This is the answer
 * to "what was actually sent", which is the question that has cost days. */
export interface ComposedConnection {
  driver: string;
  connection: string;
  form: string;
}

export function showOwnershipConnection(body: {
  name?: string;
  driver?: string;
  connection?: string;
}): Promise<ComposedConnection> {
  return apiFetch<ComposedConnection>(
    buildUrl("/ownership/import/show-connection"),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}

/** What a connection test found. `outcome` is which of the five it was, so the
 * screen can name a person to go and talk to rather than printing "failed". */
export interface ConnectionTestResult {
  outcome:
    | "connected"
    | "malformed"
    | "unreachable"
    | "refused"
    | "no-database"
    | "untrusted-domain"
    | "unknown";
  connection: string;
  form: string;
  detail?: string;
}

export function testOwnershipConnection(body: {
  name?: string;
  driver?: string;
  connection?: string;
  password_credential?: string;
}): Promise<ConnectionTestResult> {
  return apiFetch<ConnectionTestResult>(
    buildUrl("/ownership/import/test-connection"),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}
