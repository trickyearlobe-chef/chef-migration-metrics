// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from "react";
import { fetchOwners } from "../api";
import type { HolderType, Owner } from "../types";

interface Props {
  holderType: HolderType | "";
  value: string;
  /** resolved is false while the text names nothing CMM knows about. */
  onChange: (value: string, resolved: boolean) => void;
  inputId?: string;
}

const SUGGESTION_LIMIT = 8;

/**
 * The reference for whoever is on an entry.
 *
 * A ticket is free text on purpose — it addresses a system CMM does not read,
 * so there is nothing to resolve it against and holding the string is the
 * whole contract.
 *
 * An owner is not. Typing a person's name by hand produces a commitment held
 * by a string nobody can be reached through, and it stays that way: nothing
 * ever reconciles it, so the entry reads as assigned while nobody has been
 * asked. It also quietly breaks grouping the register by assignee, because a
 * name that resolves to no owner cannot join anything.
 */
export function HolderRefInput({
  holderType,
  value,
  onChange,
  inputId,
}: Props) {
  const [query, setQuery] = useState(value);
  const [results, setResults] = useState<Owner[]>([]);
  const [open, setOpen] = useState(false);
  const [searching, setSearching] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);

  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const picking = holderType === "owner";

  // Keep in step when the kind changes underneath, or when a stored value is
  // loaded into an edit.
  useEffect(() => {
    setQuery(value);
  }, [value]);

  useEffect(() => {
    if (timer.current) clearTimeout(timer.current);
    if (!picking || !open || query.trim() === "") {
      setResults([]);
      return;
    }
    timer.current = setTimeout(() => {
      setSearching(true);
      setLoadError(null);
      fetchOwners({ search: query.trim(), per_page: SUGGESTION_LIMIT })
        .then((r) => setResults(r.data ?? []))
        .catch((e: unknown) =>
          setLoadError(
            e instanceof Error ? e.message : "Failed to search owners.",
          ),
        )
        .finally(() => setSearching(false));
    }, 250);

    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [query, open, picking]);

  // A ticket, or a kind with no catalogue behind it, is plain text.
  if (!picking) {
    return (
      <input
        id={inputId}
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value, true)}
        className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      />
    );
  }

  return (
    <div className="relative">
      <input
        id={inputId}
        type="text"
        role="combobox"
        aria-expanded={open}
        aria-autocomplete="list"
        autoComplete="off"
        value={query}
        placeholder="Start typing an owner…"
        onChange={(e) => {
          setQuery(e.target.value);
          setOpen(true);
          onChange(e.target.value, false);
        }}
        onFocus={() => setOpen(true)}
        className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      />

      {open && query.trim() !== "" && (
        <ul className="absolute z-10 mt-1 max-h-56 w-full overflow-y-auto rounded-md border border-gray-200 bg-white shadow-lg">
          {searching && (
            <li className="px-3 py-2 text-xs text-gray-500">Searching…</li>
          )}
          {loadError && (
            <li className="px-3 py-2 text-xs text-red-700">{loadError}</li>
          )}
          {!searching && !loadError && results.length === 0 && (
            <li className="px-3 py-2 text-xs text-gray-500">
              No owner matches “{query}”. Only owners CMM knows about can be
              put on an entry.
            </li>
          )}
          {results.map((owner) => (
            <li key={owner.name}>
              <button
                type="button"
                onClick={() => {
                  setQuery(owner.name);
                  setOpen(false);
                  onChange(owner.name, true);
                }}
                className="block w-full px-3 py-2 text-left text-sm hover:bg-blue-50"
              >
                <span className="font-medium text-gray-800">{owner.name}</span>
                {owner.display_name && owner.display_name !== owner.name && (
                  <span className="ml-2 text-xs text-gray-500">
                    {owner.display_name}
                  </span>
                )}
                {owner.contact_email && (
                  <span className="block truncate text-xs text-gray-500">
                    {owner.contact_email}
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
