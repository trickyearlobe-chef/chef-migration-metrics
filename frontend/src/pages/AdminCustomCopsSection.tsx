// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect, useCallback } from "react";
import {
  fetchCustomCops,
  createCustomCop,
  updateCustomCop,
  deleteCustomCop,
} from "../api";
import type { CustomCopDefinition, CopClassification } from "../types";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";

// ---------------------------------------------------------------------------
// AdminCustomCopsSection — CRUD for custom cop definitions
// ---------------------------------------------------------------------------

export function AdminCustomCopsSection() {
  const [cops, setCops] = useState<CustomCopDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<CustomCopDefinition | null>(null);
  const [isNew, setIsNew] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await fetchCustomCops();
      setCops(resp.data ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load custom cops");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleNew = () => {
    setEditing({
      cop_name: "Custom/",
      description: "",
      pattern_type: "regex",
      pattern: "",
      file_glob: "*.rb",
      classification: "blocker",
      enabled: true,
    });
    setIsNew(true);
  };

  const handleEdit = (cop: CustomCopDefinition) => {
    setEditing({ ...cop });
    setIsNew(false);
  };

  const handleDelete = async (copName: string) => {
    if (!confirm(`Delete custom cop "${copName}"?`)) return;
    try {
      await deleteCustomCop(copName);
      load();
    } catch {
      setError("Failed to delete custom cop");
    }
  };

  const handleSave = async () => {
    if (!editing) return;
    setError(null);
    try {
      if (isNew) {
        await createCustomCop(editing);
      } else {
        await updateCustomCop(editing.cop_name, editing);
      }
      setEditing(null);
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save custom cop");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-700">Custom Cops</h3>
        <button
          onClick={handleNew}
          className="rounded bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700"
        >
          + Add Custom Cop
        </button>
      </div>

      <p className="text-xs text-gray-500">
        Define pattern matchers for issues not covered by cookstyle (e.g. Ruby 3 breaking changes).
        Custom cops are scanned alongside cookstyle during analysis.
      </p>

      {loading && <LoadingSpinner />}
      {error && <ErrorAlert message={error} />}

      {!loading && cops.length === 0 && !editing && (
        <div className="rounded border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-400">
          No custom cops defined yet.
        </div>
      )}

      {!loading && cops.length > 0 && (
        <div className="overflow-x-auto rounded border border-gray-200">
          <table className="min-w-full divide-y divide-gray-200 text-sm">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-3 py-2 text-left font-medium text-gray-600">Name</th>
                <th className="px-3 py-2 text-left font-medium text-gray-600">Pattern</th>
                <th className="px-3 py-2 text-left font-medium text-gray-600">Classification</th>
                <th className="px-3 py-2 text-center font-medium text-gray-600">Enabled</th>
                <th className="px-3 py-2 text-center font-medium text-gray-600">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {cops.map((cop) => (
                <tr key={cop.cop_name} className="hover:bg-gray-50">
                  <td className="px-3 py-2">
                    <div className="font-mono text-xs">{cop.cop_name}</div>
                    <div className="text-xs text-gray-400 truncate max-w-xs">
                      {cop.description}
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    <code className="rounded bg-gray-100 px-1 py-0.5 text-xs">
                      {cop.pattern_type}: {cop.pattern}
                    </code>
                    <div className="text-xs text-gray-400 mt-0.5">
                      glob: {cop.file_glob}
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    <ClassBadge classification={cop.classification} />
                  </td>
                  <td className="px-3 py-2 text-center">
                    {cop.enabled ? (
                      <span className="text-green-600">✓</span>
                    ) : (
                      <span className="text-gray-300">✗</span>
                    )}
                  </td>
                  <td className="px-3 py-2 text-center">
                    <button
                      onClick={() => handleEdit(cop)}
                      className="mr-2 text-xs text-blue-600 hover:underline"
                    >
                      Edit
                    </button>
                    <button
                      onClick={() => handleDelete(cop.cop_name)}
                      className="text-xs text-red-500 hover:underline"
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Edit/Create form */}
      {editing && (
        <CustomCopForm
          cop={editing}
          isNew={isNew}
          onChange={setEditing}
          onSave={handleSave}
          onCancel={() => setEditing(null)}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Form
// ---------------------------------------------------------------------------

function CustomCopForm({
  cop,
  isNew,
  onChange,
  onSave,
  onCancel,
}: {
  cop: CustomCopDefinition;
  isNew: boolean;
  onChange: (c: CustomCopDefinition) => void;
  onSave: () => void;
  onCancel: () => void;
}) {
  const update = (field: keyof CustomCopDefinition, value: string | boolean) => {
    onChange({ ...cop, [field]: value });
  };

  return (
    <div className="rounded-lg border border-blue-200 bg-blue-50/50 p-4 space-y-3">
      <h4 className="text-sm font-medium text-gray-700">
        {isNew ? "New Custom Cop" : `Edit: ${cop.cop_name}`}
      </h4>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <FormField label="Cop Name" hint="Must start with Custom/">
          <input
            type="text"
            className="input-sm"
            value={cop.cop_name}
            onChange={(e) => update("cop_name", e.target.value)}
            disabled={!isNew}
            placeholder="Custom/Ruby3/NilMatch"
          />
        </FormField>

        <FormField label="Description">
          <input
            type="text"
            className="input-sm"
            value={cop.description}
            onChange={(e) => update("description", e.target.value)}
            placeholder="Short explanation of what this detects"
          />
        </FormField>

        <FormField label="Pattern Type">
          <select
            className="input-sm"
            value={cop.pattern_type}
            onChange={(e) => update("pattern_type", e.target.value)}
          >
            <option value="regex">Regex</option>
            <option value="literal">Literal</option>
          </select>
        </FormField>

        <FormField label="Pattern">
          <input
            type="text"
            className="input-sm"
            value={cop.pattern}
            onChange={(e) => update("pattern", e.target.value)}
            placeholder={cop.pattern_type === "regex" ? "=~" : "nil.=~"}
          />
        </FormField>

        <FormField label="File Glob">
          <input
            type="text"
            className="input-sm"
            value={cop.file_glob}
            onChange={(e) => update("file_glob", e.target.value)}
            placeholder="*.rb"
          />
        </FormField>

        <FormField label="Classification">
          <select
            className="input-sm"
            value={cop.classification}
            onChange={(e) => update("classification", e.target.value)}
          >
            <option value="blocker">Blocker</option>
            <option value="review">Review</option>
            <option value="noise">Noise</option>
          </select>
        </FormField>

        <FormField label="Removed In (optional)" hint="Chef version where feature was removed">
          <input
            type="text"
            className="input-sm"
            value={cop.removed_in ?? ""}
            onChange={(e) => update("removed_in", e.target.value)}
            placeholder="18.0"
          />
        </FormField>

        <FormField label="Min Target Version (optional)" hint="Only flag for this version+">
          <input
            type="text"
            className="input-sm"
            value={cop.target_chef_version_min ?? ""}
            onChange={(e) => update("target_chef_version_min", e.target.value)}
            placeholder="18.0"
          />
        </FormField>
      </div>

      <div className="flex items-center gap-2">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={cop.enabled}
            onChange={(e) => update("enabled", e.target.checked)}
          />
          Enabled
        </label>
      </div>

      <div className="flex justify-end gap-2 pt-2">
        <button
          onClick={onCancel}
          className="rounded px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100"
        >
          Cancel
        </button>
        <button
          onClick={onSave}
          className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
        >
          {isNew ? "Create" : "Save"}
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

function FormField({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="block text-xs font-medium text-gray-600">
        {label}
        {hint && <span className="ml-1 font-normal text-gray-400">({hint})</span>}
      </label>
      <div className="mt-1">{children}</div>
    </div>
  );
}

function ClassBadge({ classification }: { classification: CopClassification }) {
  const styles: Record<string, string> = {
    blocker: "bg-red-100 text-red-700",
    review: "bg-amber-100 text-amber-700",
    noise: "bg-gray-100 text-gray-500",
  };
  return (
    <span
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${styles[classification] ?? "bg-gray-100 text-gray-500"}`}
    >
      {classification}
    </span>
  );
}
