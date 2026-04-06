import { useCallback, useEffect, useState } from "react";
import { fetchTargetVersions, saveTargetVersions } from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";
import { isValidSemver } from "../semver";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

export function AdminTargetVersionsPage() {
  const [versions, setVersions] = useState<string[]>([]);
  const [saved, setSaved] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [newVersion, setNewVersion] = useState("");
  const [addError, setAddError] = useState<string | null>(null);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchTargetVersions()
      .then((data) => {
        if (cancelled) return;
        setVersions(data ?? []);
        setSaved(data ?? []);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof Error ? err.message : "Failed to load target versions.",
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

  const isDirty = JSON.stringify(versions) !== JSON.stringify(saved);

  function handleAdd() {
    const trimmed = newVersion.trim();
    if (!trimmed) return;
    if (!isValidSemver(trimmed)) {
      setAddError("Invalid version format. Use MAJOR.MINOR.PATCH (e.g. 18.5.0).");
      return;
    }
    if (versions.includes(trimmed)) {
      setAddError("Version already in list.");
      return;
    }
    setVersions((prev) => [...prev, trimmed]);
    setNewVersion("");
    setAddError(null);
    setSuccess(false);
  }

  function handleRemove(version: string) {
    setVersions((prev) => prev.filter((v) => v !== version));
    setSuccess(false);
  }

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    try {
      const { value: updated } = await saveTargetVersions(versions);
      setVersions(updated ?? versions);
      setSaved(updated ?? versions);
      setSuccess(true);
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save target versions.",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading target versions…" />;
  if (loadError)
    return (
      <ErrorAlert
        message="Failed to load target versions"
        detail={loadError}
        onRetry={load}
      />
    );

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Target Chef Versions</h2>
        <p className="mt-1 text-sm text-gray-500">
          The target Chef Infra Client versions to benchmark compatibility against.
        </p>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <div className="space-y-4">
          {versions.length === 0 ? (
            <p className="text-sm text-gray-400">No target versions configured. Add one below.</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {versions.map((v) => (
                <span
                  key={v}
                  className="inline-flex items-center gap-1 rounded-full bg-blue-100 px-3 py-1 text-sm font-medium text-blue-800"
                >
                  {v}
                  <button
                    type="button"
                    onClick={() => handleRemove(v)}
                    disabled={saving}
                    className="ml-1 text-blue-500 hover:text-blue-700 disabled:opacity-40"
                    title="Remove"
                  >
                    <svg
                      className="h-3.5 w-3.5"
                      fill="none"
                      viewBox="0 0 24 24"
                      strokeWidth={2}
                      stroke="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        d="M6 18 18 6M6 6l12 12"
                      />
                    </svg>
                  </button>
                </span>
              ))}
            </div>
          )}

          <div className="flex gap-2">
            <input
              type="text"
              value={newVersion}
              onChange={(e) => {
                setNewVersion(e.target.value);
                setAddError(null);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  handleAdd();
                }
              }}
              placeholder="e.g. 18.5.0"
              className={INPUT_CLASS}
              disabled={saving}
            />
            <button
              type="button"
              onClick={handleAdd}
              disabled={saving || !newVersion.trim()}
              className="shrink-0 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-40"
            >
              Add
            </button>
          </div>
          {addError && <p className="text-xs text-red-600">{addError}</p>}
        </div>
      </div>

      {saveError && <ErrorAlert message="Failed to save" detail={saveError} />}

      {success && (
        <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          Settings saved successfully.
        </div>
      )}

      <div className="flex justify-end">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving || !isDirty}
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
        >
          {saving && <InlineSpinner />}
          {saving ? "Saving…" : "Save"}
        </button>
      </div>
    </div>
  );
}
