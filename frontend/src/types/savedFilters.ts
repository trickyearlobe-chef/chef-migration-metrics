// SPDX-License-Identifier: Apache-2.0

/** The list views a saved filter can belong to. Not portable across views. */
export type SavedFilterView = "nodes" | "roles" | "cookbooks" | "git-repos";

/**
 * The stored selection: the view's own query params, multi-value params
 * carrying lists. The vocabulary is owned by the view (and enforced by the
 * backend allowlist in internal/webapi/saved_filter_params.go) — a saved filter
 * records a selection in it, never redefines it.
 */
export type SavedFilterParams = Record<string, string[]>;

/** Mirrors datastore.SavedFilter. */
export interface SavedFilter {
  id: string;
  name: string;
  view: SavedFilterView;
  filters: SavedFilterParams;
  owner_username: string;
  shared: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateSavedFilterRequest {
  name: string;
  view: SavedFilterView;
  filters: SavedFilterParams;
  shared: boolean;
}

/** Absent fields are left unchanged by the backend. */
export interface UpdateSavedFilterRequest {
  name?: string;
  filters?: SavedFilterParams;
  shared?: boolean;
}
