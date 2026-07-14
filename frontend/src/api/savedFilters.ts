// SPDX-License-Identifier: Apache-2.0

import type {
  SavedFilter,
  SavedFilterView,
  CreateSavedFilterRequest,
  UpdateSavedFilterRequest,
} from "../types";
import { apiFetch, buildUrl } from "./client";

const PATH = "/saved-filters";

/** The caller's own filters for a view, plus every shared one. */
export async function listSavedFilters(
  view: SavedFilterView,
): Promise<SavedFilter[]> {
  const res = await apiFetch<SavedFilter[] | null>(buildUrl(PATH, { view }));
  return res ?? [];
}

export function createSavedFilter(
  req: CreateSavedFilterRequest,
): Promise<SavedFilter> {
  return apiFetch<SavedFilter>(buildUrl(PATH), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

/**
 * Rename, re-select and share/unshare are all this one call — fields absent
 * from the body are left unchanged. Owner only.
 */
export function updateSavedFilter(
  id: string,
  req: UpdateSavedFilterRequest,
): Promise<SavedFilter> {
  return apiFetch<SavedFilter>(
    buildUrl(`${PATH}/${encodeURIComponent(id)}`),
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

/** Owner only. */
export function deleteSavedFilter(id: string): Promise<void> {
  return apiFetch<void>(buildUrl(`${PATH}/${encodeURIComponent(id)}`), {
    method: "DELETE",
  });
}
