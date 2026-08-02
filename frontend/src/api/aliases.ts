// SPDX-License-Identifier: Apache-2.0

import type {
  AliasImportResponse,
  AliasSuggestion,
  MergeOwnersResult,
  OwnerAlias,
  OwnerDuplicateRescanResponse,
  OwnerDuplicatesResponse,
} from "../types";
import { apiFetch, buildUrl } from "./client";

/**
 * The alias types the database accepts. The CHECK constraint on
 * owner_aliases.alias_type permits exactly these, so anything else is
 * rejected at write time.
 */
export const ALIAS_TYPES = [
  "email",
  "git_email",
  "git_name",
  "username",
  "custom",
] as const;

export function fetchOwnerAliases(
  ownerName: string,
): Promise<{ aliases: OwnerAlias[] }> {
  return apiFetch<{ aliases: OwnerAlias[] }>(
    buildUrl("/ownership/aliases", { owner: ownerName }),
  );
}

export function createOwnerAlias(body: {
  owner_name: string;
  alias_type: string;
  alias_value: string;
  source?: string;
}): Promise<OwnerAlias> {
  return apiFetch<OwnerAlias>(buildUrl("/ownership/aliases"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function deleteOwnerAlias(id: string): Promise<void> {
  return apiFetch<void>(
    buildUrl(`/ownership/aliases/${encodeURIComponent(id)}`),
    { method: "DELETE" },
  );
}

export async function importOwnerAliases(
  file: File,
  format: "csv" | "json",
): Promise<AliasImportResponse> {
  const formData = new FormData();
  formData.append("format", format);
  formData.append("file", file);
  return apiFetch<AliasImportResponse>(buildUrl("/ownership/aliases/import"), {
    method: "POST",
    body: formData,
  });
}

export function suggestOwnerAliases(
  query: string,
  limit = 8,
): Promise<{ suggestions: AliasSuggestion[] }> {
  return apiFetch<{ suggestions: AliasSuggestion[] }>(
    buildUrl("/ownership/aliases/suggest", { q: query, limit }),
  );
}

export function fetchOwnerDuplicates(params?: {
  page?: number;
  per_page?: number;
  min_similarity?: number;
}): Promise<OwnerDuplicatesResponse> {
  return apiFetch<OwnerDuplicatesResponse>(
    buildUrl("/ownership/duplicates", params),
  );
}

/**
 * Starts a fresh scan of the catalogue. Returns as soon as the scan has been
 * started, not when it finishes — it walks every owner and every alias.
 */
export function rescanOwnerDuplicates(): Promise<OwnerDuplicateRescanResponse> {
  return apiFetch<OwnerDuplicateRescanResponse>(
    buildUrl("/ownership/duplicates/rescan"),
    { method: "POST" },
  );
}

/**
 * Folds one owner into another. The work moves, every identity the source was
 * known by moves with it, and the source owner is removed — which is what
 * stops the next ingest from undoing the correction.
 */
export function mergeOwners(body: {
  from_owner: string;
  into_owner: string;
}): Promise<MergeOwnersResult> {
  return apiFetch<MergeOwnersResult>(buildUrl("/ownership/merge"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}
