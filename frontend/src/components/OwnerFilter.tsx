// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchOwners } from "../api";
import type { Owner } from "../types";

// ---------------------------------------------------------------------------
// The ownership filter for the list views — "what's mine" and "what has
// nobody", asked from one control.
//
// Both questions live in one control for a reason: the API rejects `owner` and
// `unowned` together with a 400, so the two selections have to be mutually
// exclusive by construction. Split across two controls, that rule could only be
// enforced by catching the error after somebody had already asked for it.
//
// Owners are searched on the server, not filtered in the browser. The estate
// carries thousands, so any page of them held locally would silently answer
// "no such person" for anybody who did not make the first page.
// ---------------------------------------------------------------------------

/** The whole selection, so the two mutually exclusive halves move together. */
export interface OwnerSelection {
  owners: string[];
  unowned: boolean;
}

interface OwnerFilterProps {
  owners: string[];
  unowned: boolean;
  onChange: (next: OwnerSelection) => void;
  /** How long to wait after a keystroke before searching. */
  debounceMs?: number;
  /** How many owners a search returns. */
  limit?: number;
}

export function OwnerFilter({
  owners,
  unowned,
  onChange,
  debounceMs = 300,
  limit = 50,
}: OwnerFilterProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [results, setResults] = useState<Owner[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);

  const containerRef = useRef<HTMLDivElement>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    function handleOutsideClick(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setIsOpen(false);
      }
    }
    document.addEventListener("mousedown", handleOutsideClick);
    return () => document.removeEventListener("mousedown", handleOutsideClick);
  }, []);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") setIsOpen(false);
    }
    if (isOpen) {
      document.addEventListener("keydown", handleKeyDown);
      return () => document.removeEventListener("keydown", handleKeyDown);
    }
  }, [isOpen]);

  const runSearch = useCallback(
    (term: string) => {
      setLoading(true);
      setLoadFailed(false);
      fetchOwners({ search: term, per_page: limit, sort: "name", order: "asc" })
        .then((res) => setResults(res.data ?? []))
        .catch(() => {
          // An unreadable catalogue must not render as an empty one: the two
          // read the same on screen and mean opposite things.
          setResults([]);
          setLoadFailed(true);
        })
        .finally(() => setLoading(false));
    },
    [limit],
  );

  // Search on open (so there is a list without typing) and on every change to
  // the term, debounced.
  useEffect(() => {
    if (!isOpen) return;
    if (timerRef.current) clearTimeout(timerRef.current);
    if (search === "") {
      runSearch("");
      return;
    }
    timerRef.current = setTimeout(() => runSearch(search), debounceMs);
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [isOpen, search, debounceMs, runSearch]);

  function toggleOwner(name: string) {
    // Ticking an owner answers the opposite question, so the unowned one goes.
    const next = owners.includes(name)
      ? owners.filter((o) => o !== name)
      : [...owners, name];
    onChange({ owners: next, unowned: false });
  }

  function toggleUnowned() {
    // The exclusion, enforced here rather than by the API's 400.
    onChange({ owners: [], unowned: !unowned });
  }

  function removeOwner(name: string) {
    onChange({ owners: owners.filter((o) => o !== name), unowned });
  }

  const selectionCount = owners.length + (unowned ? 1 : 0);
  const buttonLabel = unowned
    ? "Owner: nobody"
    : owners.length > 0
      ? `Owner (${owners.length})`
      : "Owner";

  return (
    <div ref={containerRef} className="relative">
      <label className="mb-1 block text-xs font-medium text-gray-500">
        Owner
      </label>
      <button
        type="button"
        onClick={() => setIsOpen((o) => !o)}
        className="block w-40 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-left text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      >
        {buttonLabel}
      </button>

      {isOpen && (
        <div className="absolute left-0 top-full z-10 mt-1 w-64 rounded-md border border-gray-300 bg-white py-1 shadow-lg">
          <label className="flex cursor-pointer items-center gap-2 border-b border-gray-100 px-3 py-2 text-sm hover:bg-gray-50">
            <input
              type="checkbox"
              checked={unowned}
              onChange={toggleUnowned}
              className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            <span className="font-medium">No owner</span>
          </label>

          <div className="px-2 py-2">
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              disabled={unowned}
              placeholder="Search owners…"
              className="block w-full rounded-md border border-gray-300 px-2 py-1 text-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50 disabled:text-gray-400"
            />
          </div>

          <div className="max-h-52 overflow-auto">
            {loading && (
              <div className="px-3 py-2 text-sm text-gray-400">Searching…</div>
            )}
            {!loading && loadFailed && (
              <div className="px-3 py-2 text-sm text-red-600">
                Could not load owners.
              </div>
            )}
            {!loading && !loadFailed && results.length === 0 && (
              <div className="px-3 py-2 text-sm text-gray-400">
                No owners match.
              </div>
            )}
            {!loading &&
              results.map((o) => (
                <label
                  key={o.name}
                  className={`flex items-center gap-2 px-3 py-1.5 text-sm ${
                    unowned ? "text-gray-400" : "cursor-pointer hover:bg-gray-50"
                  }`}
                >
                  <input
                    type="checkbox"
                    checked={owners.includes(o.name)}
                    onChange={() => toggleOwner(o.name)}
                    disabled={unowned}
                    className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                  />
                  <span>
                    {o.display_name || o.name}
                    {o.display_name && (
                      <span className="ml-1 text-gray-400">({o.name})</span>
                    )}
                  </span>
                </label>
              ))}
          </div>
        </div>
      )}

      {selectionCount > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {unowned && (
            <span className="inline-flex items-center gap-0.5 rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-800">
              No owner
              <button
                type="button"
                onClick={() => onChange({ owners: [], unowned: false })}
                className="ml-0.5 text-amber-700 hover:text-amber-900"
                aria-label="Remove no owner"
              >
                &times;
              </button>
            </span>
          )}
          {owners.map((name) => (
            <span
              key={name}
              className="inline-flex items-center gap-0.5 rounded-full bg-blue-100 px-2 py-0.5 text-xs text-blue-800"
            >
              {name}
              <button
                type="button"
                onClick={() => removeOwner(name)}
                className="ml-0.5 text-blue-600 hover:text-blue-900"
                aria-label={`Remove ${name}`}
              >
                &times;
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
