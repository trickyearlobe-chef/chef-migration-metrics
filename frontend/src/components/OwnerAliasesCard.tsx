// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  ALIAS_TYPES,
  createOwnerAlias,
  deleteOwnerAlias,
  fetchOwnerAliases,
} from "../api";
import type { OwnerAlias } from "../types";
import { ErrorAlert, LoadingSpinner } from "./Feedback";

const ALIAS_TYPE_STYLES: Record<string, string> = {
  email: "bg-sky-100 text-sky-700",
  git_email: "bg-violet-100 text-violet-700",
  git_name: "bg-indigo-100 text-indigo-700",
  username: "bg-amber-100 text-amber-700",
  custom: "bg-gray-100 text-gray-700",
};

const message = (error: unknown, fallback: string) =>
  error instanceof Error ? error.message : fallback;

function TypeBadge({ value }: { value: string }) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
        ALIAS_TYPE_STYLES[value] ?? "bg-gray-100 text-gray-700"
      }`}
    >
      {value}
    </span>
  );
}

/**
 * The identities one owner is known by, editable on that owner's own page.
 *
 * Assignments and aliases answer different questions — what somebody owns,
 * versus what they are called elsewhere — so both belong on the person, and
 * there is no owner field to fill in here: the page is already the owner.
 */
export function OwnerAliasesCard({
  ownerName,
  canEdit,
}: {
  ownerName: string;
  canEdit: boolean;
}) {
  const [aliases, setAliases] = useState<OwnerAlias[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [aliasType, setAliasType] = useState<string>("email");
  const [aliasValue, setAliasValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [removingId, setRemovingId] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setLoadError(null);
    return fetchOwnerAliases(ownerName)
      .then((response) => setAliases(response.aliases ?? []))
      .catch((error: unknown) =>
        setLoadError(message(error, "Failed to load aliases.")),
      )
      .finally(() => setLoading(false));
  }, [ownerName]);

  useEffect(() => {
    void load();
  }, [load]);

  async function handleAdd(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = aliasValue.trim();
    if (!value) return;
    setSaving(true);
    setSaveError(null);
    try {
      await createOwnerAlias({
        owner_name: ownerName,
        alias_type: aliasType,
        alias_value: value,
      });
      setAliasValue("");
      await load();
    } catch (error: unknown) {
      setSaveError(message(error, "Failed to add the alias."));
    } finally {
      setSaving(false);
    }
  }

  async function handleRemove(alias: OwnerAlias) {
    setRemovingId(alias.id);
    setSaveError(null);
    try {
      await deleteOwnerAlias(alias.id);
      await load();
    } catch (error: unknown) {
      setSaveError(message(error, "Failed to remove the alias."));
    } finally {
      setRemovingId(null);
    }
  }

  return (
    <div className="card">
      <h3 className="card-header">Identity Aliases</h3>
      <p className="mt-1 text-sm text-gray-500">
        What this person is called elsewhere — email addresses, git commit
        names and addresses, usernames, and whatever raw string an ownership
        source used. Imports and git history resolve to this owner through
        these.
      </p>

      {loading ? (
        <LoadingSpinner message="Loading aliases…" />
      ) : loadError ? (
        <ErrorAlert message={loadError} onRetry={() => void load()} />
      ) : aliases.length === 0 ? (
        <p className="mt-4 text-sm text-gray-400">
          No aliases recorded. Nothing else resolves to this owner yet.
        </p>
      ) : (
        <ul className="mt-4 divide-y divide-gray-100">
          {aliases.map((alias) => (
            <li
              key={alias.id}
              className="flex flex-wrap items-center gap-3 py-2"
            >
              <TypeBadge value={alias.alias_type} />
              <span className="font-medium text-gray-800">
                {alias.alias_value}
              </span>
              <span className="text-xs text-gray-400">
                added by {alias.source}
              </span>
              {canEdit && (
                <button
                  type="button"
                  onClick={() => void handleRemove(alias)}
                  disabled={removingId === alias.id}
                  className="ml-auto text-xs text-red-600 hover:text-red-800 hover:underline disabled:opacity-50"
                >
                  {removingId === alias.id ? "Removing…" : "Remove"}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      {canEdit && (
        <form
          onSubmit={handleAdd}
          className="mt-4 flex flex-wrap items-end gap-3 border-t border-gray-100 pt-4"
        >
          <div>
            <label
              htmlFor="owner-alias-type"
              className="mb-1 block text-xs font-medium text-gray-600"
            >
              Alias type
            </label>
            <select
              id="owner-alias-type"
              value={aliasType}
              onChange={(event) => setAliasType(event.target.value)}
              className="rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            >
              {ALIAS_TYPES.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </select>
          </div>
          <div className="min-w-[16rem] flex-1">
            <label
              htmlFor="owner-alias-value"
              className="mb-1 block text-xs font-medium text-gray-600"
            >
              Alias value
            </label>
            <input
              id="owner-alias-value"
              type="text"
              value={aliasValue}
              onChange={(event) => setAliasValue(event.target.value)}
              placeholder="e.g. thomas.smith@example-corp.test"
              className="w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </div>
          <button
            type="submit"
            disabled={saving || !aliasValue.trim()}
            className="rounded-md bg-green-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-green-700 disabled:opacity-50"
          >
            {saving ? "Adding…" : "Add alias"}
          </button>
        </form>
      )}

      {saveError && (
        <div className="mt-3">
          <ErrorAlert message={saveError} />
        </div>
      )}
    </div>
  );
}
