// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from "react";
import { fetchGitRepos } from "../api";
import type { GitRepoListItem } from "../types";

interface Props {
  value: string;
  onChange: (repo: GitRepoListItem | null, rawName: string) => void;
  disabled?: boolean;
  inputId?: string;
}

const SUGGESTION_LIMIT = 8;

/**
 * Picks a git repo that actually exists.
 *
 * Typing the name by hand looked harmless and was not: a verdict recorded
 * against a name no repo carries is stored, shown in the register, and
 * silently changes nobody's readiness — because the readiness evaluator
 * resolves a cookbook to its repo by name and never finds it. Choosing from
 * the catalogue removes that whole class of entry rather than reporting it
 * afterwards.
 */
export function GitRepoPicker({ value, onChange, disabled, inputId }: Props) {
  const [query, setQuery] = useState(value);
  const [results, setResults] = useState<GitRepoListItem[]>([]);
  const [open, setOpen] = useState(false);
  const [searching, setSearching] = useState(false);
  const [confirmed, setConfirmed] = useState<GitRepoListItem | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  // The catalogue runs to thousands of repos, so this searches rather than
  // loading a list. Debounced so a typed name is not a request per keystroke.
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (timer.current) clearTimeout(timer.current);
    if (!open || query.trim() === "") {
      setResults([]);
      return;
    }
    timer.current = setTimeout(() => {
      setSearching(true);
      setLoadError(null);
      fetchGitRepos({ search: query.trim(), per_page: SUGGESTION_LIMIT })
        .then((r) => setResults(r.data ?? []))
        .catch((e: unknown) =>
          setLoadError(
            e instanceof Error ? e.message : "Failed to search repos.",
          ),
        )
        .finally(() => setSearching(false));
    }, 250);

    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [query, open]);

  function choose(repo: GitRepoListItem) {
    setConfirmed(repo);
    setQuery(repo.name);
    setOpen(false);
    onChange(repo, repo.name);
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
        disabled={disabled}
        onChange={(e) => {
          setQuery(e.target.value);
          setConfirmed(null);
          setOpen(true);
          // Reported up as an unconfirmed name so the form can refuse to
          // submit until something from the catalogue has been chosen.
          onChange(null, e.target.value);
        }}
        onFocus={() => setOpen(true)}
        placeholder="Start typing a repo name…"
        className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50"
      />

      {confirmed && (
        <p className="mt-1 text-xs text-green-700" data-testid="repo-confirmed">
          {confirmed.name} — {confirmed.git_repo_url}
        </p>
      )}

      {!confirmed && query.trim() !== "" && !open && (
        <p className="mt-1 text-xs text-amber-700">
          Choose a repo from the list — a name typed by hand may match nothing.
        </p>
      )}

      {open && query.trim() !== "" && (
        <ul className="absolute z-10 mt-1 max-h-64 w-full overflow-y-auto rounded-md border border-gray-200 bg-white shadow-lg">
          {searching && (
            <li className="px-3 py-2 text-xs text-gray-500">Searching…</li>
          )}
          {loadError && (
            <li className="px-3 py-2 text-xs text-red-700">{loadError}</li>
          )}
          {!searching && !loadError && results.length === 0 && (
            <li className="px-3 py-2 text-xs text-gray-500">
              No repo matches “{query}”. Only repos CMM has collected can carry
              a verdict.
            </li>
          )}
          {results.map((repo) => (
            <li key={`${repo.name}|${repo.git_repo_url}`}>
              <button
                type="button"
                onClick={() => choose(repo)}
                className="block w-full px-3 py-2 text-left text-sm hover:bg-blue-50"
              >
                <span className="font-medium text-gray-800">{repo.name}</span>
                <span className="block truncate text-xs text-gray-500">
                  {repo.git_repo_url}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
