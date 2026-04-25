// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import React, { useCallback, useEffect, useState } from "react";
import {
  fetchPlatformDisplayNames,
  updatePlatformDisplayNames,
  resetPlatformDisplayNames,
} from "../api";
import type { DisplayNameMapping } from "../api";
import {
  ErrorAlert,
  InlineSpinner,
  LoadingSpinner,
} from "../components/Feedback";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

const EMPTY_FORM = { platform: "", version_prefix: "", display_name: "" };

export function AdminPlatformDisplayNamesPage() {
  const [mappings, setMappings] = useState<DisplayNameMapping[]>([]);
  const [isDefault, setIsDefault] = useState(true);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editForm, setEditForm] = useState(EMPTY_FORM);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetchPlatformDisplayNames()
      .then((data) => {
        if (cancelled) return;
        setMappings(data.mappings);
        setIsDefault(data.is_default);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(
            err instanceof Error
              ? err.message
              : "Failed to load platform display names.",
          );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => load(), [load]);

  useEffect(() => {
    if (!success) return;
    const timer = setTimeout(() => setSuccess(null), 3000);
    return () => clearTimeout(timer);
  }, [success]);

  function handleAddMapping() {
    setEditingIndex(-1);
    setEditForm(EMPTY_FORM);
    setSuccess(null);
  }

  function handleEdit(index: number) {
    setEditingIndex(index);
    setEditForm({ ...mappings[index] });
    setSuccess(null);
  }

  function handleDelete(index: number) {
    setMappings((prev) => prev.filter((_, i) => i !== index));
    if (editingIndex === index) {
      setEditingIndex(null);
      setEditForm(EMPTY_FORM);
    } else if (editingIndex !== null && editingIndex > index) {
      setEditingIndex(editingIndex - 1);
    }
    setSuccess(null);
  }

  function handleCancelEdit() {
    setEditingIndex(null);
    setEditForm(EMPTY_FORM);
  }

  function handleConfirmEdit() {
    const trimmed = {
      platform: editForm.platform.trim(),
      version_prefix: editForm.version_prefix.trim(),
      display_name: editForm.display_name.trim(),
    };
    if (!trimmed.platform || !trimmed.version_prefix || !trimmed.display_name)
      return;

    if (editingIndex === -1) {
      setMappings((prev) => [...prev, trimmed]);
    } else if (editingIndex !== null) {
      setMappings((prev) =>
        prev.map((m, i) => (i === editingIndex ? trimmed : m)),
      );
    }
    setEditingIndex(null);
    setEditForm(EMPTY_FORM);
    setSuccess(null);
  }

  function validate(): string | null {
    for (const m of mappings) {
      if (
        !m.platform.trim() ||
        !m.version_prefix.trim() ||
        !m.display_name.trim()
      ) {
        return "All fields (platform, version prefix, display name) are required.";
      }
    }
    const seen = new Set<string>();
    for (const m of mappings) {
      const key = `${m.platform}::${m.version_prefix}`;
      if (seen.has(key)) {
        return `Duplicate mapping: platform "${m.platform}" with version prefix "${m.version_prefix}".`;
      }
      seen.add(key);
    }
    return null;
  }

  async function handleSave() {
    const validationError = validate();
    if (validationError) {
      setError(validationError);
      return;
    }
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const data = await updatePlatformDisplayNames(mappings);
      setMappings(data.mappings);
      setIsDefault(data.is_default);
      setSuccess("Platform display names saved successfully.");
    } catch (err: unknown) {
      setError(
        err instanceof Error
          ? err.message
          : "Failed to save platform display names.",
      );
    } finally {
      setSaving(false);
    }
  }

  async function handleReset() {
    if (
      !window.confirm(
        "Reset all platform display names to defaults? This will discard any custom mappings.",
      )
    ) {
      return;
    }
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const data = await resetPlatformDisplayNames();
      setMappings(data.mappings);
      setIsDefault(data.is_default);
      setEditingIndex(null);
      setEditForm(EMPTY_FORM);
      setSuccess("Platform display names reset to defaults.");
    } catch (err: unknown) {
      setError(
        err instanceof Error
          ? err.message
          : "Failed to reset platform display names.",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading)
    return <LoadingSpinner message="Loading platform display names…" />;

  const editRow = (
    <tr className="bg-blue-50">
      <td className="px-4 py-2">
        <input
          type="text"
          value={editForm.platform}
          onChange={(e) =>
            setEditForm((f) => ({ ...f, platform: e.target.value }))
          }
          placeholder="e.g. centos"
          className={INPUT_CLASS}
          data-testid="edit-platform"
          disabled={saving}
        />
      </td>
      <td className="px-4 py-2">
        <input
          type="text"
          value={editForm.version_prefix}
          onChange={(e) =>
            setEditForm((f) => ({ ...f, version_prefix: e.target.value }))
          }
          placeholder="e.g. 8"
          className={INPUT_CLASS}
          data-testid="edit-version-prefix"
          disabled={saving}
        />
      </td>
      <td className="px-4 py-2">
        <input
          type="text"
          value={editForm.display_name}
          onChange={(e) =>
            setEditForm((f) => ({ ...f, display_name: e.target.value }))
          }
          placeholder="e.g. CentOS 8 (EOL)"
          className={INPUT_CLASS}
          data-testid="edit-display-name"
          disabled={saving}
        />
      </td>
      <td className="px-4 py-2">
        <div className="flex gap-2">
          <button
            type="button"
            onClick={handleConfirmEdit}
            disabled={
              saving ||
              !editForm.platform.trim() ||
              !editForm.version_prefix.trim() ||
              !editForm.display_name.trim()
            }
            className="rounded-md bg-blue-600 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-40"
          >
            OK
          </button>
          <button
            type="button"
            onClick={handleCancelEdit}
            disabled={saving}
            className="rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-40"
          >
            Cancel
          </button>
        </div>
      </td>
    </tr>
  );

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">
            Platform Display Names
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Map opaque OS version strings to human-friendly labels.
          </p>
        </div>
        {!isDefault && (
          <button
            type="button"
            onClick={handleReset}
            disabled={saving}
            className="shrink-0 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-40"
          >
            Reset to Defaults
          </button>
        )}
      </div>

      {error && <ErrorAlert message={error} onRetry={() => setError(null)} />}

      {success && (
        <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          {success}
        </div>
      )}

      <div className="rounded-lg border border-gray-200 bg-white shadow-sm overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                Platform
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                Version Prefix
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                Display Name
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {mappings.length === 0 && editingIndex === null && (
              <tr>
                <td
                  colSpan={4}
                  className="px-4 py-8 text-center text-sm text-gray-400"
                >
                  No mappings configured. Add one below.
                </td>
              </tr>
            )}
            {mappings.map((m, i) =>
              editingIndex === i ? (
                <React.Fragment key={i}>{editRow}</React.Fragment>
              ) : (
                <tr key={i} className="hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm text-gray-900">
                    {m.platform}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-900">
                    {m.version_prefix}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-900">
                    {m.display_name}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-2">
                      <button
                        type="button"
                        onClick={() => handleEdit(i)}
                        disabled={saving || editingIndex !== null}
                        className="text-xs font-medium text-blue-600 hover:text-blue-700 disabled:opacity-40"
                      >
                        Edit
                      </button>
                      <button
                        type="button"
                        onClick={() => handleDelete(i)}
                        disabled={saving}
                        className="text-xs font-medium text-red-600 hover:text-red-700 disabled:opacity-40"
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ),
            )}
            {editingIndex === -1 && editRow}
          </tbody>
        </table>

        <div className="border-t border-gray-100 px-4 py-3">
          <button
            type="button"
            onClick={handleAddMapping}
            disabled={saving || editingIndex !== null}
            className="flex items-center gap-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 disabled:opacity-40"
          >
            <svg
              className="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={2}
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M12 4.5v15m7.5-7.5h-15"
              />
            </svg>
            Add Mapping
          </button>
        </div>
      </div>

      <div className="flex justify-end">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving || editingIndex !== null}
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
        >
          {saving && <InlineSpinner />}
          {saving ? "Saving…" : "Save Changes"}
        </button>
      </div>
    </div>
  );
}
