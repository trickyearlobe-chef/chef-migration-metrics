// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useMemo, useState } from "react";
import { fetchApiDocument } from "../api";
import type { ApiOperation, OpenApiDocument } from "../types";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";

// The service's own description, rendered for a person rather than a client
// generator. Read-only by construction: there is no control here that calls
// anything, because the page is served to a signed-in person and a "try" button
// would fire real requests as them.
//
// Hand-rolled rather than a renderer library. Measured 2026-08-12, the cheapest
// of the three real ones added 97 packages to a tree of 371 — and the document
// carries no parameters, request bodies or schemas, so what they exist to
// display is not there. See plans/todo-documentation.md for the numbers and for
// what would have to change first.

const METHOD_ORDER = ["get", "post", "put", "patch", "delete", "head", "options"];

const METHOD_STYLE: Record<string, string> = {
  get: "bg-sky-100 text-sky-800",
  post: "bg-emerald-100 text-emerald-800",
  put: "bg-amber-100 text-amber-800",
  patch: "bg-amber-100 text-amber-800",
  delete: "bg-red-100 text-red-800",
};

const ROLE_STYLE: Record<string, string> = {
  admin: "bg-purple-100 text-purple-700",
  operator: "bg-blue-100 text-blue-700",
  authenticated: "bg-gray-100 text-gray-600",
  public: "bg-green-100 text-green-700",
};

interface Entry {
  path: string;
  method: string;
  op: ApiOperation;
}

// groupOf names the area an address belongs to, taken from the path because the
// document carries no tags. /api/v1/admin/... groups as "admin" rather than by
// what follows it, so everything an administrator touches reads as one section.
function groupOf(path: string): string {
  const parts = path.replace(/^\/api\/v\d+\//, "").split("/");
  return parts[0] || "root";
}

function entriesOf(doc: OpenApiDocument | null): Entry[] {
  if (!doc?.paths) return [];
  const out: Entry[] = [];
  for (const [path, item] of Object.entries(doc.paths)) {
    for (const [method, op] of Object.entries(item ?? {})) {
      if (!op) continue;
      out.push({ path, method: method.toLowerCase(), op });
    }
  }
  return out.sort(
    (a, b) =>
      a.path.localeCompare(b.path) ||
      METHOD_ORDER.indexOf(a.method) - METHOD_ORDER.indexOf(b.method),
  );
}

export function ApiDocsPage() {
  const [doc, setDoc] = useState<OpenApiDocument | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  const load = useCallback(() => {
    setLoading(true);
    setLoadError(null);
    fetchApiDocument()
      .then(setDoc)
      .catch((err: unknown) =>
        setLoadError(
          err instanceof Error
            ? err.message
            : "Failed to load the API description.",
        ),
      )
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => load(), [load]);

  const entries = useMemo(() => entriesOf(doc), [doc]);

  // Match the summary as well as the address: somebody who knew the address
  // would not have opened this page.
  const matched = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return entries;
    return entries.filter(
      (e) =>
        e.path.toLowerCase().includes(q) ||
        (e.op.summary ?? "").toLowerCase().includes(q) ||
        (e.op.operationId ?? "").toLowerCase().includes(q),
    );
  }, [entries, query]);

  const grouped = useMemo(() => {
    const map = new Map<string, Entry[]>();
    for (const e of matched) {
      const g = groupOf(e.path);
      const list = map.get(g);
      if (list) list.push(e);
      else map.set(g, [e]);
    }
    return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  }, [matched]);

  return (
    <div className="mx-auto max-w-5xl space-y-5 p-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">
          {doc?.info?.title ?? "API"} API
        </h1>
        {doc?.info?.description && (
          <p className="mt-1 text-sm text-gray-600">{doc.info.description}</p>
        )}
        <p className="mt-1 text-xs text-gray-500">
          {/* The version is load-bearing: a description is only true of the
              build that served it. */}
          Generated from this build
          {doc?.info?.version ? ` — version ${doc.info.version}` : ""}. Machine
          readable at{" "}
          <a
            className="font-mono text-blue-600 hover:underline"
            href="/api/v1/openapi.json"
          >
            /api/v1/openapi.json
          </a>
          .
        </p>
      </div>

      {loading && <LoadingSpinner message="Loading the API description…" />}

      {/* A failure to load is reported as one. An empty list would say this
          service serves nothing. */}
      {!loading && loadError && (
        <ErrorAlert message={loadError} onRetry={load} />
      )}

      {!loading && !loadError && (
        <>
          <div className="flex items-baseline gap-3">
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Filter by address or by what it is for…"
              aria-label="Filter the API description"
              className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
            <span className="shrink-0 text-xs text-gray-500">
              {matched.length} of {entries.length}
            </span>
          </div>

          {matched.length === 0 && (
            <p className="rounded-md border border-gray-200 bg-white p-4 text-sm text-gray-500">
              No addresses match that.
            </p>
          )}

          {grouped.map(([group, groupEntries]) => (
            <section
              key={group}
              className="overflow-hidden rounded-md border border-gray-200 bg-white"
            >
              <h2 className="border-b border-gray-200 bg-gray-50 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500">
                {group}
              </h2>
              <ul className="divide-y divide-gray-100">
                {groupEntries.map((e) => (
                  <li key={`${e.method} ${e.path}`} className="px-4 py-2.5">
                    <div className="flex items-start gap-3">
                      <span
                        className={`mt-0.5 w-16 shrink-0 rounded px-1.5 py-0.5 text-center font-mono text-[11px] font-semibold uppercase ${
                          METHOD_STYLE[e.method] ?? "bg-gray-100 text-gray-700"
                        }`}
                      >
                        {e.method}
                      </span>
                      <div className="min-w-0 flex-1">
                        <code className="break-all font-mono text-sm text-gray-900">
                          {e.path}
                        </code>
                        {e.op.summary && (
                          <p className="mt-0.5 text-sm text-gray-600">
                            {e.op.summary}
                          </p>
                        )}
                      </div>
                      {e.op["x-required-role"] && (
                        <span
                          className={`mt-0.5 shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${
                            ROLE_STYLE[e.op["x-required-role"]] ??
                            "bg-gray-100 text-gray-600"
                          }`}
                          title="The access this operation needs"
                        >
                          {e.op["x-required-role"]}
                        </span>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </>
      )}
    </div>
  );
}
