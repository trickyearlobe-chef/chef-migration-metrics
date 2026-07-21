import { useCallback, useEffect, useState } from "react";
import { fetchCollection, saveCollection, type CollectionConfig } from "../api";
import {
  ErrorAlert,
  InlineSpinner,
  LoadingSpinner,
} from "../components/Feedback";
import { CronDescription } from "../components/CronDescription";
import { EventIngestSettings } from "../components/EventIngestSettings";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

// AdminCollectionPage groups two unrelated config sections under tabs. Config UI
// is due a broader refactor (see plans/todo-tech-debt.md); this is the natural
// home for the ingest toggles for now.
export function AdminCollectionPage() {
  const [tab, setTab] = useState<"collection" | "ingest">("collection");
  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-4">
          <button
            type="button"
            onClick={() => setTab("collection")}
            className={`border-b-2 px-1 py-2 text-sm font-medium ${
              tab === "collection"
                ? "border-blue-600 text-blue-600"
                : "border-transparent text-gray-500 hover:text-gray-700"
            }`}
          >
            Collection
          </button>
          <button
            type="button"
            onClick={() => setTab("ingest")}
            className={`border-b-2 px-1 py-2 text-sm font-medium ${
              tab === "ingest"
                ? "border-blue-600 text-blue-600"
                : "border-transparent text-gray-500 hover:text-gray-700"
            }`}
          >
            Event ingest
          </button>
        </nav>
      </div>
      {tab === "collection" ? <CollectionSettingsForm /> : <EventIngestSettings />}
    </div>
  );
}

function CollectionSettingsForm() {
  const [config, setConfig] = useState<CollectionConfig>({
    schedule: "",
    stale_node_threshold_days: 30,
    stale_node_warning_hours: 72,
    stale_node_critical_days: 7,
    stale_cookbook_threshold_days: 30,
    skip_server_cookbook_download: false,
    delete_server_cookbooks_after_scan: false,
  });
  const [saved, setSaved] = useState<CollectionConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchCollection()
      .then((data) => {
        if (cancelled) return;
        setConfig(data);
        setSaved(data);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof Error
              ? err.message
              : "Failed to load collection settings.",
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

  function handleChange<K extends keyof CollectionConfig>(
    key: K,
    value: CollectionConfig[K],
  ) {
    setConfig((prev) => ({ ...prev, [key]: value }));
    setSuccess(false);
  }

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    const payload: CollectionConfig = {
      ...config,
      delete_server_cookbooks_after_scan:
        config.delete_server_cookbooks_after_scan ?? false,
    };
    try {
      const { value: updated } = await saveCollection(payload);
      setConfig(updated);
      setSaved(updated);
      setSuccess(true);
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error
          ? err.message
          : "Failed to save collection settings.",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading collection settings…" />;
  if (loadError)
    return (
      <ErrorAlert
        message="Failed to load collection settings"
        detail={loadError}
        onRetry={load}
      />
    );

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">
          Collection Settings
        </h2>
        <p className="mt-1 text-sm text-gray-500">
          Controls the background data collection schedule and staleness
          thresholds.
        </p>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">
              Collection Schedule
            </label>
            <input
              type="text"
              value={config.schedule}
              onChange={(e) => handleChange("schedule", e.target.value)}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Cron expression with 5 space-separated fields (e.g. 0 * * * *)
            </p>
            <CronDescription expression={config.schedule} />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Stale Node Threshold (days)
            </label>
            <input
              type="number"
              min={1}
              value={config.stale_node_threshold_days}
              onChange={(e) =>
                handleChange(
                  "stale_node_threshold_days",
                  Number(e.target.value),
                )
              }
              className={INPUT_CLASS}
              disabled={saving}
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Stale Node Warning (hours)
            </label>
            <input
              type="number"
              min={1}
              value={config.stale_node_warning_hours}
              onChange={(e) =>
                handleChange("stale_node_warning_hours", Number(e.target.value))
              }
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Nodes missing longer than this are shown as &lsquo;Missing&rsquo;
              (amber)
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Stale Node Critical (days)
            </label>
            <input
              type="number"
              min={1}
              value={config.stale_node_critical_days}
              onChange={(e) =>
                handleChange("stale_node_critical_days", Number(e.target.value))
              }
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Nodes missing longer than this are shown as &lsquo;Gone&rsquo;
              (red). Must be greater than warning threshold.
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Stale Cookbook Threshold (days)
            </label>
            <input
              type="number"
              min={1}
              value={config.stale_cookbook_threshold_days}
              onChange={(e) =>
                handleChange(
                  "stale_cookbook_threshold_days",
                  Number(e.target.value),
                )
              }
              className={INPUT_CLASS}
              disabled={saving}
            />
          </div>

          <div className="flex items-center justify-between">
            <label className="text-sm font-medium text-gray-700">
              Skip Server Cookbook Download
            </label>
            <button
              type="button"
              role="switch"
              aria-checked={config.skip_server_cookbook_download}
              onClick={() =>
                handleChange(
                  "skip_server_cookbook_download",
                  !config.skip_server_cookbook_download,
                )
              }
              disabled={saving}
              className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 ${
                config.skip_server_cookbook_download
                  ? "bg-blue-600"
                  : "bg-gray-200"
              }`}
            >
              <span
                className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                  config.skip_server_cookbook_download
                    ? "translate-x-6"
                    : "translate-x-1"
                }`}
              />
            </button>
          </div>

          <div className="flex items-start justify-between gap-4">
            <div>
              <label className="text-sm font-medium text-gray-700">
                Delete Server Cookbooks After Scan
              </label>
              <p className="mt-1 text-xs text-gray-500">
                When enabled, cookbook files are deleted immediately after
                scanning to save disk space.
              </p>
            </div>
            <button
              type="button"
              role="switch"
              aria-checked={config.delete_server_cookbooks_after_scan ?? false}
              onClick={() =>
                handleChange(
                  "delete_server_cookbooks_after_scan",
                  !(config.delete_server_cookbooks_after_scan ?? false),
                )
              }
              disabled={saving}
              className={`relative mt-0.5 inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 ${
                (config.delete_server_cookbooks_after_scan ?? false)
                  ? "bg-blue-600"
                  : "bg-gray-200"
              }`}
            >
              <span
                className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                  (config.delete_server_cookbooks_after_scan ?? false)
                    ? "translate-x-6"
                    : "translate-x-1"
                }`}
              />
            </button>
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
