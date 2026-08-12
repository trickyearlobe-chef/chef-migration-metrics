// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import {
  createMyApiToken,
  destroyMyApiToken,
  fetchMyApiTokens,
} from "../api";
import type { ApiToken, CreatedApiToken } from "../types";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";

// Somebody's own record: the credentials they have made for tools they are
// holding. Every person who can sign in can reach this, because making one is
// not an administrator's act — it is handing your own access to a tool, and
// only you know what for.

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

function formatWhen(value?: string): string {
  if (!value) return "never";
  const when = new Date(value);
  if (Number.isNaN(when.getTime())) return value;
  return when.toLocaleString();
}

export function AccountPage() {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [canWrite, setCanWrite] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  // The one response that carries a secret. Held in memory only, and cleared
  // the moment somebody dismisses it: there is nowhere to fetch it back from,
  // which is the point.
  const [minted, setMinted] = useState<CreatedApiToken | null>(null);

  const [confirming, setConfirming] = useState<string | null>(null);
  const [destroyError, setDestroyError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setLoadError(null);
    fetchMyApiTokens()
      .then((data) => setTokens(data?.tokens ?? []))
      .catch((err: unknown) =>
        setLoadError(
          err instanceof Error
            ? err.message
            : "Failed to load your credentials.",
        ),
      )
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => load(), [load]);

  async function handleCreate() {
    const trimmed = name.trim();
    if (!trimmed) return;
    setCreating(true);
    setCreateError(null);
    try {
      const created = await createMyApiToken(trimmed, canWrite);
      setMinted(created);
      setName("");
      setCanWrite(false);
      load();
    } catch (err: unknown) {
      setCreateError(
        err instanceof Error ? err.message : "Failed to create the credential.",
      );
    } finally {
      setCreating(false);
    }
  }

  async function handleDestroy(id: string) {
    setDestroyError(null);
    try {
      await destroyMyApiToken(id);
      setConfirming(null);
      load();
    } catch (err: unknown) {
      setDestroyError(
        err instanceof Error ? err.message : "Failed to destroy the credential.",
      );
    }
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6 p-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">
          API credentials
        </h1>
        <p className="mt-1 text-sm text-gray-600">
          A credential is another way into your own account — the same access
          you have on screen, for a tool such as an AI assistant in your editor.
          It is not a separate account and cannot see more than you can.
        </p>
      </div>

      {minted && (
        <div className="rounded-md border border-amber-300 bg-amber-50 p-4">
          <h2 className="text-sm font-semibold text-amber-900">
            Copy this now — it cannot be shown again
          </h2>
          <p className="mt-1 text-sm text-amber-800">
            Nothing stores this secret. If you lose it, destroy the credential
            and make another.
          </p>
          <code className="mt-3 block break-all rounded bg-white px-3 py-2 font-mono text-sm text-gray-900">
            {minted.secret}
          </code>
          <button
            onClick={() => setMinted(null)}
            className="mt-3 rounded-md bg-amber-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-amber-700"
          >
            I have copied it
          </button>
        </div>
      )}

      <div className="rounded-md border border-gray-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-gray-900">
          Create a credential
        </h2>
        {createError && (
          <div className="mt-3">
            <ErrorAlert message={createError} />
          </div>
        )}
        <div className="mt-3 space-y-3">
          <div>
            <label
              htmlFor="credential-name"
              className="block text-sm font-medium text-gray-700"
            >
              Name
            </label>
            <input
              id="credential-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="editor on my laptop"
              className={INPUT_CLASS}
            />
            <p className="mt-1 text-xs text-gray-500">
              How you will recognise it later, so you know which one to destroy.
            </p>
          </div>

          <div className="flex items-start gap-2">
            <input
              id="credential-can-write"
              type="checkbox"
              checked={canWrite}
              onChange={(e) => setCanWrite(e.target.checked)}
              className="mt-1 h-4 w-4 rounded border-gray-300"
            />
            <label
              htmlFor="credential-can-write"
              className="text-sm text-gray-700"
            >
              Let it record findings in the failure register
              <span className="mt-0.5 block text-xs text-gray-500">
                Off by default. A credential that can write may add to the
                failure register and nothing else, and anything it writes is
                marked as written by this credential rather than by you.
              </span>
            </label>
          </div>

          <button
            onClick={handleCreate}
            disabled={creating || !name.trim()}
            className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-gray-300"
          >
            {creating ? "Creating…" : "Create credential"}
          </button>
        </div>
      </div>

      <div className="rounded-md border border-gray-200 bg-white">
        <h2 className="border-b border-gray-200 px-4 py-3 text-sm font-semibold text-gray-900">
          Your credentials
        </h2>

        {destroyError && (
          <div className="p-4">
            <ErrorAlert message={destroyError} />
          </div>
        )}

        {loading && (
          <div className="p-4">
            <LoadingSpinner message="Loading your credentials…" />
          </div>
        )}

        {/* A failure to load is reported as one. An empty list here would say
            "you have none", which is a different statement and not one this
            page can make when it could not read them. */}
        {!loading && loadError && (
          <div className="p-4">
            <ErrorAlert message={loadError} onRetry={load} />
          </div>
        )}

        {!loading && !loadError && tokens.length === 0 && (
          <p className="p-4 text-sm text-gray-500">
            No credentials yet. Create one above to use this service from an
            editor assistant or a script.
          </p>
        )}

        {!loading && !loadError && tokens.length > 0 && (
          <ul className="divide-y divide-gray-200">
            {tokens.map((token) => (
              <li key={token.id} className="px-4 py-3">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium text-gray-900">
                      {token.name}
                    </div>
                    <div className="mt-0.5 text-xs text-gray-500">
                      Created {formatWhen(token.created_at)} · Last used{" "}
                      {formatWhen(token.last_used_at)}
                    </div>
                    <span
                      className={`mt-1.5 inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ${
                        token.can_write
                          ? "bg-amber-100 text-amber-800"
                          : "bg-gray-100 text-gray-600"
                      }`}
                    >
                      {token.can_write
                        ? "Can record findings"
                        : "Read only"}
                    </span>
                  </div>

                  {confirming === token.id ? (
                    <div className="flex shrink-0 items-center gap-2">
                      <button
                        onClick={() => handleDestroy(token.id)}
                        className="rounded-md bg-red-600 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-red-700"
                      >
                        Destroy it
                      </button>
                      <button
                        onClick={() => setConfirming(null)}
                        className="rounded-md border border-gray-300 px-2.5 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50"
                      >
                        Keep it
                      </button>
                    </div>
                  ) : (
                    <button
                      onClick={() => setConfirming(token.id)}
                      className="shrink-0 rounded-md border border-red-300 px-2.5 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50"
                    >
                      Destroy
                    </button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
