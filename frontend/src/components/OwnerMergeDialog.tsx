// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";
import { fetchOwners, mergeOwners } from "../api";
import type { MergeOwnersResult, Owner } from "../types";
import { ErrorAlert } from "./Feedback";

const message = (error: unknown, fallback: string) =>
  error instanceof Error ? error.message : fallback;

/**
 * Folds one owner into another.
 *
 * The target is chosen by searching owners rather than aliases: an owner that
 * has no alias yet is exactly the one somebody comes here to correct, and an
 * alias search cannot find it.
 */
export function OwnerMergeDialog({
  fromOwner,
  intoOwner,
  onCancel,
  onMerged,
}: {
  fromOwner: string;
  /** Preset target. When given, the pair can be swapped rather than searched. */
  intoOwner?: string;
  onCancel: () => void;
  onMerged: (result: MergeOwnersResult) => void;
}) {
  const [source, setSource] = useState(fromOwner);
  const [target, setTarget] = useState(intoOwner ?? "");
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [matches, setMatches] = useState<Owner[]>([]);
  const [searching, setSearching] = useState(false);
  const [merging, setMerging] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSwap = intoOwner !== undefined;

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(search.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [search]);

  useEffect(() => {
    if (canSwap || !debounced) {
      setMatches([]);
      return;
    }
    let cancelled = false;
    setSearching(true);
    fetchOwners({ search: debounced, per_page: 8 })
      .then((response) => {
        if (!cancelled) {
          setMatches((response.data ?? []).filter((o) => o.name !== source));
        }
      })
      .catch((e: unknown) => !cancelled && setError(message(e, "Failed to search owners.")))
      .finally(() => !cancelled && setSearching(false));
    return () => {
      cancelled = true;
    };
  }, [debounced, canSwap, source]);

  async function handleMerge() {
    if (!target) return;
    setMerging(true);
    setError(null);
    try {
      onMerged(await mergeOwners({ from_owner: source, into_owner: target }));
    } catch (e: unknown) {
      setError(message(e, "Failed to merge the owners."));
      setMerging(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl">
        <h3 className="text-lg font-bold text-gray-800">Merge owners</h3>

        <p className="mt-3 text-sm text-gray-600">
          Everything <span className="font-medium">{source}</span> owns moves to{" "}
          <span className="font-medium">{target || "the owner you choose"}</span>
          , along with every identity {source} is known by.{" "}
          <span className="font-medium">{source}</span> is then removed.
        </p>
        <p className="mt-2 text-sm text-gray-500">
          The name {source} is kept as an alias of the owner you choose, so the
          next ownership import resolves it there instead of recreating the
          person.
        </p>

        {canSwap ? (
          <div className="mt-4 flex items-center gap-3 rounded-md border border-gray-200 bg-gray-50 px-3 py-2 text-sm">
            <span className="text-gray-700">
              {source} <span className="text-gray-400">→</span> {target}
            </span>
            <button
              type="button"
              onClick={() => {
                const previous = source;
                setSource(target);
                setTarget(previous);
              }}
              disabled={merging}
              className="ml-auto rounded-md border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Swap direction
            </button>
          </div>
        ) : (
          <div className="mt-4">
            <label
              htmlFor="merge-target-search"
              className="mb-1 block text-xs font-medium text-gray-600"
            >
              Merge into
            </label>
            <input
              id="merge-target-search"
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search owners by name"
              className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
            {searching && (
              <p className="mt-2 text-xs text-gray-500">Searching…</p>
            )}
            {matches.length > 0 && (
              <ul className="mt-2 max-h-48 divide-y divide-gray-100 overflow-y-auto rounded-md border border-gray-200">
                {matches.map((owner) => (
                  <li key={owner.name}>
                    <button
                      type="button"
                      onClick={() => setTarget(owner.name)}
                      className={`flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-gray-50 ${
                        target === owner.name ? "bg-blue-50" : ""
                      }`}
                    >
                      <span className="font-medium text-gray-800">
                        {owner.name}
                      </span>
                      {owner.display_name && (
                        <span className="text-xs text-gray-500">
                          {owner.display_name}
                        </span>
                      )}
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {!searching && debounced && matches.length === 0 && (
              <p className="mt-2 text-xs text-gray-500">
                No owner matches “{debounced}”.
              </p>
            )}
          </div>
        )}

        {error && (
          <div className="mt-3">
            <ErrorAlert message={error} />
          </div>
        )}

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            disabled={merging}
            className="rounded-md border border-gray-300 px-4 py-2 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleMerge()}
            disabled={merging || !target}
            className="rounded-md bg-blue-600 px-4 py-2 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
          >
            {merging ? "Merging…" : "Merge owners"}
          </button>
        </div>
      </div>
    </div>
  );
}
