// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import {
  fetchIngestConfig,
  saveIngestConfig,
  type IngestConfig,
} from "../api";
import { invalidateFeatures } from "../hooks/useFeatures";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "./Feedback";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

function Toggle({
  label,
  description,
  checked,
  onChange,
  disabled,
}: {
  label: string;
  description?: string;
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <label className="text-sm font-medium text-gray-700">{label}</label>
        {description && (
          <p className="mt-1 text-xs text-gray-500">{description}</p>
        )}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        onClick={() => onChange(!checked)}
        disabled={disabled}
        className={`relative mt-0.5 inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 ${
          checked ? "bg-blue-600" : "bg-gray-200"
        }`}
      >
        <span
          className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
            checked ? "translate-x-6" : "translate-x-1"
          }`}
        />
      </button>
    </div>
  );
}

// EventIngestSettings edits the ingest section (the passive telemetry sink + the
// Run events feature gates). Self-contained load/save against
// /admin/config/ingest — separate from the collection form it sits beside.
export function EventIngestSettings() {
  const [config, setConfig] = useState<IngestConfig | null>(null);
  const [saved, setSaved] = useState<IngestConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchIngestConfig()
      .then((data) => {
        if (cancelled) return;
        setConfig(data);
        setSaved(data);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof Error ? err.message : "Failed to load ingest settings.",
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

  if (loading) return <LoadingSpinner message="Loading ingest settings…" />;
  if (loadError)
    return (
      <ErrorAlert
        message="Failed to load ingest settings"
        detail={loadError}
        onRetry={load}
      />
    );
  if (!config) return null;

  const bool = (v: boolean | null | undefined) => v ?? false;
  const isDirty = JSON.stringify(config) !== JSON.stringify(saved);

  function set<K extends keyof IngestConfig>(key: K, value: IngestConfig[K]) {
    setConfig((prev) => (prev ? { ...prev, [key]: value } : prev));
    setSuccess(false);
  }

  async function handleSave() {
    if (!config) return;
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    try {
      const { value: updated } = await saveIngestConfig(config);
      setConfig(updated);
      setSaved(updated);
      setSuccess(true);
      // The visibility flag drives the nav + Node Detail Runs tab; drop the
      // cached features so the next page load reflects the change.
      invalidateFeatures();
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save ingest settings.",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Event Ingest</h2>
        <p className="mt-1 text-sm text-gray-500">
          The passive run-telemetry sink and the Run events feature. Kept off by
          default — enable to accept events and reveal the views.
        </p>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <div className="space-y-5">
          <Toggle
            label="Accept telemetry (ingest enabled)"
            description="Open the POST /api/v1/ingest sink to receive Chef run events."
            checked={bool(config.enabled)}
            onChange={(v) => set("enabled", v)}
            disabled={saving}
          />

          <Toggle
            label="Show Run events"
            description="Reveal the Run events view and the Node Detail Runs tab. Independent of accepting telemetry — data can accrue while hidden. Nav/tab visibility updates on the next page load."
            checked={bool(config.show_run_events)}
            onChange={(v) => set("show_run_events", v)}
            disabled={saving}
          />

          <Toggle
            label="Ingest failures only"
            description="Discard successful converge events at the sink and keep only failures — a firehose-relief valve for high-volume fleets."
            checked={bool(config.failures_only)}
            onChange={(v) => set("failures_only", v)}
            disabled={saving}
          />

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Retention (days)
            </label>
            <input
              type="number"
              min={1}
              value={config.retention_days}
              onChange={(e) => set("retention_days", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Whole-day partitions older than this are dropped.
            </p>
          </div>
        </div>
      </div>

      {saveError && <ErrorAlert message="Failed to save" detail={saveError} />}

      {success && (
        <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          Ingest settings saved.
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
