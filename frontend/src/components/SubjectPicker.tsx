// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from "react";
import { fetchCookbooks, fetchGitRepos } from "../api";

/** What a verdict is about: the repo where the fix is made, or the cookbook
 * itself where no repo has been collected for it. */
export interface Subject {
  name: string;
  type: "git_repo" | "cookbook";
  /** The repo URL, or a note about why this is a cookbook subject. */
  detail: string;
}

interface Props {
  value: string;
  onChange: (subject: Subject | null, rawName: string) => void;
  inputId?: string;
}

const SUGGESTION_LIMIT = 8;

/**
 * Picks something that actually exists to record a verdict against.
 *
 * Searches repos and cookbooks together rather than making somebody choose a
 * kind first — at the point of recording you know the name of the thing that
 * broke, not whether CMM happens to hold a repo for it.
 *
 * A name typed by hand and never matched is the failure this prevents: it is
 * stored, shown in the register, and silently changes nobody's readiness,
 * because the evaluator resolves a verdict by name and never finds it.
 */
export function SubjectPicker({ value, onChange, inputId }: Props) {
  const [query, setQuery] = useState(value);
  const [results, setResults] = useState<Subject[]>([]);
  const [open, setOpen] = useState(false);
  const [searching, setSearching] = useState(false);
  const [confirmed, setConfirmed] = useState<Subject | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  // Debounced: the catalogue runs to thousands of rows, so this searches
  // rather than loading a list, and not once per keystroke.
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

      Promise.all([
        fetchGitRepos({ search: query.trim(), per_page: SUGGESTION_LIMIT }),
        fetchCookbooks({ search: query.trim(), per_page: SUGGESTION_LIMIT * 4 }),
      ])
        .then(([repoPage, cookbookPage]) => {
          const repos: Subject[] = (repoPage.data ?? []).map((r) => ({
            name: r.name,
            type: "git_repo",
            detail: r.git_repo_url,
          }));

          // Cookbooks come back per version; the register is never keyed on a
          // version, so they collapse to distinct names. A cookbook that
          // already has a repo is offered as the repo — that is where the fix
          // is made.
          const repoNames = new Set(repos.map((r) => r.name));
          const seen = new Set<string>();
          const cookbooks: Subject[] = [];
          for (const cb of cookbookPage.data ?? []) {
            if (repoNames.has(cb.name) || seen.has(cb.name)) continue;
            seen.add(cb.name);
            cookbooks.push({
              name: cb.name,
              type: "cookbook",
              detail: "no repo collected for this cookbook",
            });
          }

          setResults([...repos, ...cookbooks].slice(0, SUGGESTION_LIMIT * 2));
        })
        .catch((e: unknown) =>
          setLoadError(e instanceof Error ? e.message : "Failed to search."),
        )
        .finally(() => setSearching(false));
    }, 250);

    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [query, open]);

  function choose(subject: Subject) {
    setConfirmed(subject);
    setQuery(subject.name);
    setOpen(false);
    onChange(subject, subject.name);
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
        onChange={(e) => {
          setQuery(e.target.value);
          setConfirmed(null);
          setOpen(true);
          // Reported up unconfirmed, so the form refuses to submit until
          // something from the catalogue has been chosen.
          onChange(null, e.target.value);
        }}
        onFocus={() => setOpen(true)}
        placeholder="Start typing a repo or cookbook name…"
        className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      />

      {confirmed && (
        <p
          className="mt-1 text-xs text-green-700"
          data-testid="subject-confirmed"
        >
          {confirmed.type === "cookbook"
            ? `${confirmed.name} — cookbook, no repo collected`
            : `${confirmed.name} — ${confirmed.detail}`}
        </p>
      )}

      {!confirmed && query.trim() !== "" && !open && (
        <p className="mt-1 text-xs text-amber-700">
          Choose one from the list — a name typed by hand may match nothing.
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
              Nothing collected matches “{query}”. Only repos and cookbooks CMM
              has collected can carry a verdict.
            </li>
          )}
          {results.map((subject) => (
            <li key={`${subject.type}|${subject.name}`}>
              <button
                type="button"
                onClick={() => choose(subject)}
                className="block w-full px-3 py-2 text-left text-sm hover:bg-blue-50"
              >
                <span className="font-medium text-gray-800">
                  {subject.name}
                </span>
                <span
                  className={`ml-2 rounded-full px-1.5 py-0.5 text-[10px] font-medium ${
                    subject.type === "cookbook"
                      ? "bg-amber-100 text-amber-800"
                      : "bg-gray-100 text-gray-600"
                  }`}
                >
                  {subject.type === "cookbook" ? "cookbook" : "repo"}
                </span>
                <span className="block truncate text-xs text-gray-500">
                  {subject.detail}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
