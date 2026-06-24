import { useCallback, useEffect, useState } from "react";
import {
  fetchAnalysisTools,
  saveAnalysisTools,
  type AnalysisToolsConfig,
} from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

export function AdminAnalysisToolsPage() {
  const [config, setConfig] = useState<AnalysisToolsConfig>({
    embedded_bin_dir: "",
    cookstyle_enabled: true,
    cookstyle_timeout_minutes: 10,
  });
  const [saved, setSaved] = useState<AnalysisToolsConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchAnalysisTools()
      .then((data) => {
        if (cancelled) return;
        setConfig(data.value);
        setSaved(data.value);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof Error ? err.message : "Failed to load analysis tools settings.",
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

  function handleChange<K extends keyof AnalysisToolsConfig>(
    key: K,
    value: AnalysisToolsConfig[K],
  ) {
    setConfig((prev) => ({ ...prev, [key]: value }));
    setSuccess(false);
  }

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    const payload: AnalysisToolsConfig = { ...config };
    try {
      const { value: updated } = await saveAnalysisTools(payload);
      setConfig(updated);
      setSaved(updated);
      setSuccess(true);
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save analysis tools settings.",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading analysis tools settings…" />;
  if (loadError)
    return (
      <ErrorAlert
        message="Failed to load analysis tools settings"
        detail={loadError}
        onRetry={load}
      />
    );

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Analysis Tools</h2>
        <p className="mt-1 text-sm text-gray-500">
          Shared settings for the CookStyle and Test Kitchen analysis tools.
        </p>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">
              Embedded Binary Directory
            </label>
            <input
              type="text"
              value={config.embedded_bin_dir}
              onChange={(e) => handleChange("embedded_bin_dir", e.target.value)}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Path to the directory containing the embedded Chef tools (cookstyle, kitchen). Leave
              empty to use system PATH.
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
