import { useCallback, useEffect, useState } from "react";
import { fetchExportsConfig, saveExportsConfig, type ExportsConfig } from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

export function AdminExportsPage() {
  const [config, setConfig] = useState<ExportsConfig>({
    max_rows: 10000,
    async_threshold: 1000,
    output_directory: "",
    retention_hours: 24,
  });
  const [saved, setSaved] = useState<ExportsConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchExportsConfig()
      .then((data) => {
        if (cancelled) return;
        setConfig(data);
        setSaved(data);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof Error ? err.message : "Failed to load export settings.",
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

  const isDirty = JSON.stringify(config) !== JSON.stringify(saved);

  function handleChange<K extends keyof ExportsConfig>(key: K, value: ExportsConfig[K]) {
    setConfig((prev) => ({ ...prev, [key]: value }));
    setSuccess(false);
  }

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    try {
      const { value: updated } = await saveExportsConfig(config);
      setConfig(updated);
      setSaved(updated);
      setSuccess(true);
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save export settings.",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading export settings…" />;
  if (loadError)
    return (
      <ErrorAlert
        message="Failed to load export settings"
        detail={loadError}
        onRetry={load}
      />
    );

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Export Settings</h2>
        <p className="mt-1 text-sm text-gray-500">
          Controls data export behaviour including file size limits and retention.
        </p>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">
              Maximum Rows Per Export
            </label>
            <input
              type="number"
              min={1}
              value={config.max_rows}
              onChange={(e) => handleChange("max_rows", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Maximum number of rows returned in a single export operation.
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Async Export Threshold (rows)
            </label>
            <input
              type="number"
              min={1}
              value={config.async_threshold}
              onChange={(e) => handleChange("async_threshold", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Exports larger than this threshold run asynchronously in the background.
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Export File Retention (hours)
            </label>
            <input
              type="number"
              min={1}
              value={config.retention_hours}
              onChange={(e) => handleChange("retention_hours", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Number of hours to retain generated export files before automatic deletion.
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">Output Directory</label>
            <input
              type="text"
              value={config.output_directory}
              onChange={(e) => handleChange("output_directory", e.target.value)}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Directory where export files are written. Leave empty to use the default.
            </p>
          </div>
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
