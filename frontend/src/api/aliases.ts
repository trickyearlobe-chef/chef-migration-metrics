// SPDX-License-Identifier: Apache-2.0

import type {
  AliasImportResponse,
  AliasSuggestion,
  OwnerAlias,
} from "../types";
import { apiFetch, buildUrl } from "./client";

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
