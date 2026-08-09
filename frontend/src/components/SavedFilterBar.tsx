// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// ---------------------------------------------------------------------------
// Saved filters control for a list view (journeys/named-cohorts.md).
//
// Names a filter selection so an operator stops rebuilding a 20-role cohort by
// hand every session. Applying is not an API call: the stored params are handed
// back to the page, which sets its own filter state — so a saved filter can
// never carry, or drift from, something the view does not natively support.
//
// This component owns the CRUD and the list. It does NOT own what a param means:
// the page decides how to apply a selection, and (for Nodes) whether any of the
// entities it names have since vanished.
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useRef, useState } from "react";
import type { SavedFilter, SavedFilterParams, SavedFilterView } from "../types";
import {
  listSavedFilters,
  createSavedFilter,
  updateSavedFilter,
  deleteSavedFilter,
} from "../api/savedFilters";
import { useAuth } from "../context/AuthContext";

interface SavedFilterBarProps {
  /** Which list view these filters belong to. Not portable across views. */
  view: SavedFilterView;
  /** The view's current selection — what "Save" would store. */
  currentParams: SavedFilterParams;
  /** Hand the stored selection back to the page to apply. */
  onApply: (params: SavedFilterParams) => void;
}

type Pending =
  | { kind: "rename"; id: string; name: string }
  | { kind: "delete"; id: string }
  | null;

export function SavedFilterBar({
  view,
  currentParams,
  onApply,
}: SavedFilterBarProps) {
  const { user } = useAuth();
  const [isOpen, setIsOpen] = useState(false);
  const [filters, setFilters] = useState<SavedFilter[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [pending, setPending] = useState<Pending>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const refresh = useCallback(async () => {
    try {
      setFilters(await listSavedFilters(view));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not load saved filters");
    }
  }, [view]);

  // Only fetch when the operator opens the panel — a saved filter is something
  // you reach for, not something every page load pays for.
  useEffect(() => {
    if (isOpen) void refresh();
  }, [isOpen, refresh]);

  useEffect(() => {
    function onClickOutside(e: MouseEvent) {
      if (!containerRef.current?.contains(e.target as Node)) setIsOpen(false);
    }
    document.addEventListener("mousedown", onClickOutside);
    return () => document.removeEventListener("mousedown", onClickOutside);
  }, []);

  /** Run a mutation, surfacing the backend's own message on failure. */
  const mutate = useCallback(
    async (fn: () => Promise<unknown>) => {
      try {
        await fn();
        setError(null);
        setPending(null);
        await refresh();
      } catch (e) {
        setError(e instanceof Error ? e.message : "Request failed");
      }
    },
    [refresh],
  );

  const save = () => {
    const name = newName.trim();
    if (!name) return;
    void mutate(async () => {
      await createSavedFilter({ name, view, filters: currentParams, shared: false });
      setNewName("");
    });
  };

  const owned = (f: SavedFilter) => f.owner_username === user?.username;

  return (
    <div ref={containerRef} className="relative mb-0.5">
      <button
        type="button"
        onClick={() => setIsOpen((v) => !v)}
        aria-expanded={isOpen}
        title="Apply a saved filter, or save the current selection"
        className="flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-900"
      >
        Saved filters
        <span aria-hidden="true" className="text-[0.6rem] leading-none">
          ▾
        </span>
      </button>

      {isOpen && (
        <div className="absolute left-0 z-20 mt-1 w-96 rounded-md border border-gray-300 bg-white p-3 text-left shadow-lg">
          {error && (
            <p className="mb-2 rounded bg-red-50 px-2 py-1 text-xs text-red-700">
              {error}
            </p>
          )}

          {filters.length === 0 ? (
            <p className="mb-3 text-xs text-gray-500">
              No saved filters yet. Build a selection, then name it below.
            </p>
          ) : (
            <ul className="mb-3 max-h-72 divide-y divide-gray-100 overflow-auto">
              {filters.map((f) => (
                <li key={f.id} className="py-2">
                  {pending?.kind === "rename" && pending.id === f.id ? (
                    <div className="flex items-center gap-1.5">
                      <input
                        type="text"
                        value={pending.name}
                        onChange={(e) =>
                          setPending({ ...pending, name: e.target.value })
                        }
                        className="w-full rounded-md border border-gray-300 px-2 py-1 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                      />
                      <button
                        type="button"
                        onClick={() =>
                          void mutate(() =>
                            updateSavedFilter(f.id, { name: pending.name.trim() }),
                          )
                        }
                        disabled={!pending.name.trim()}
                        className="whitespace-nowrap text-xs font-medium text-blue-600 hover:underline disabled:text-gray-400"
                      >
                        Confirm rename
                      </button>
                      <button
                        type="button"
                        onClick={() => setPending(null)}
                        className="text-xs text-gray-500 hover:underline"
                      >
                        Cancel
                      </button>
                    </div>
                  ) : pending?.kind === "delete" && pending.id === f.id ? (
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm text-gray-700">
                        Delete &ldquo;{f.name}&rdquo;?
                      </span>
                      <span className="flex gap-2">
                        <button
                          type="button"
                          onClick={() =>
                            void mutate(() => deleteSavedFilter(f.id))
                          }
                          className="text-xs font-medium text-red-600 hover:underline"
                        >
                          Confirm delete
                        </button>
                        <button
                          type="button"
                          onClick={() => setPending(null)}
                          className="text-xs text-gray-500 hover:underline"
                        >
                          Cancel
                        </button>
                      </span>
                    </div>
                  ) : (
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <button
                          type="button"
                          aria-label={`Apply ${f.name}`}
                          onClick={() => {
                            onApply(f.filters);
                            setIsOpen(false);
                          }}
                          className="block truncate text-left text-sm font-medium text-blue-600 hover:underline"
                        >
                          {f.name}
                        </button>
                        {!owned(f) && (
                          <span className="text-xs text-gray-500">
                            Shared by {f.owner_username}
                          </span>
                        )}
                        {owned(f) && f.shared && (
                          <span className="text-xs text-gray-500">Shared</span>
                        )}
                      </div>

                      {owned(f) && (
                        <span className="flex shrink-0 gap-2 text-xs">
                          <button
                            type="button"
                            aria-label={`Update ${f.name} to current selection`}
                            onClick={() =>
                              void mutate(() =>
                                updateSavedFilter(f.id, { filters: currentParams }),
                              )
                            }
                            className="text-gray-500 hover:underline"
                          >
                            Update
                          </button>
                          <button
                            type="button"
                            aria-label={`Rename ${f.name}`}
                            onClick={() =>
                              setPending({ kind: "rename", id: f.id, name: f.name })
                            }
                            className="text-gray-500 hover:underline"
                          >
                            Rename
                          </button>
                          <button
                            type="button"
                            aria-label={`${f.shared ? "Unshare" : "Share"} ${f.name}`}
                            onClick={() =>
                              void mutate(() =>
                                updateSavedFilter(f.id, { shared: !f.shared }),
                              )
                            }
                            className="text-gray-500 hover:underline"
                          >
                            {f.shared ? "Unshare" : "Share"}
                          </button>
                          <button
                            type="button"
                            aria-label={`Delete ${f.name}`}
                            onClick={() => setPending({ kind: "delete", id: f.id })}
                            className="text-red-600 hover:underline"
                          >
                            Delete
                          </button>
                        </span>
                      )}
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}

          <div className="border-t border-gray-100 pt-3">
            <label
              htmlFor="saved-filter-name"
              className="mb-1 block text-xs font-medium text-gray-500"
            >
              Name
            </label>
            <div className="flex gap-1.5">
              <input
                id="saved-filter-name"
                type="text"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") save();
                  if (e.key === "Escape") setIsOpen(false);
                }}
                placeholder="Save current selection as…"
                className="block w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
              <button
                type="button"
                onClick={save}
                disabled={!newName.trim()}
                className="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:bg-gray-300"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
