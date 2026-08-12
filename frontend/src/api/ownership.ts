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
 * database. The connection string is never sent — the server reads it from a
 * stored credential, so a password never travels through the browser. */
export interface IntakeDatabaseSource {
  driver: string;
  /** Name of a stored credential holding the connection string. */
  credential: string;
  query: string;
  /** Overrides the connection's own sslmode, PostgreSQL only. Empty leaves the
   * stored connection exactly as it is. Needed because the Postgres driver
   * requires TLS when the connection says nothing, so a connection that works
   * elsewhere fails here — and the alternative was retyping the whole
   * credential, password included. */
  tlsMode?: string;
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
  formData.append("db_driver", db.driver);
  formData.append("db_credential", db.credential);
  formData.append("db_query", db.query);
  if (db.tlsMode) {
    formData.append("db_tls_mode", db.tlsMode);
  }
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
  driver: string,
  credential: string,
  tlsMode?: string,
): Promise<{ data: IntakeDatabaseTable[] }> {
  const formData = new FormData();
  formData.append("db_driver", driver);
  formData.append("db_credential", credential);
  if (tlsMode) {
    formData.append("db_tls_mode", tlsMode);
  }
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
  db_driver?: string;
  db_credential?: string;
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
