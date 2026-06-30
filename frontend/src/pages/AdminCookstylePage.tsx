// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import {
  fetchAnalysisTools,
  saveAnalysisTools,
  rescanAllCookstyle,
  type AnalysisToolsConfig,
  type CookstyleFailurePreset,
  COOKSTYLE_PRESETS,
} from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";
import { CookstyleFailureRulesGrid } from "../components/CookstyleFailureRulesGrid";
import { AdminCustomCopsSection } from "./AdminCustomCopsSection";
import { AdminCopClassificationsSection } from "./AdminCopClassificationsSection";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

function detectPreset(rules: Record<string, string[]>): CookstyleFailurePreset {
  for (const name of ["strict", "default", "relaxed"] as const) {
    const preset = COOKSTYLE_PRESETS[name];
    if (JSON.stringify(normalizeRules(preset)) === JSON.stringify(normalizeRules(rules))) {
      return name;
    }
  }
  return "custom";
}

function normalizeRules(rules: Record<string, string[]>): Record<string, string[]> {
  const sorted: Record<string, string[]> = {};
  for (const key of Object.keys(rules).sort()) {
    sorted[key] = [...(rules[key] ?? [])].sort();
  }
  return sorted;
}

export function AdminCookstylePage() {
  const [config, setConfig] = useState<AnalysisToolsConfig | null>(null);
  const [saved, setSaved] = useState<AnalysisToolsConfig | null>(null);
  const [failurePreset, setFailurePreset] = useState<CookstyleFailurePreset>("default");
  const [failureRules, setFailureRules] = useState<Record<string, string[]>>(
    COOKSTYLE_PRESETS["default"],
  );
  const [savedRulesJSON, setSavedRulesJSON] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const [rescanningAll, setRescanningAll] = useState(false);
  const [rescanAllMsg, setRescanAllMsg] = useState<string | null>(null);
  const [showRescanAllConfirm, setShowRescanAllConfirm] = useState(false);

  const handleRescanAll = useCallback(() => {
    setRescanningAll(true);
    setRescanAllMsg(null);
    setShowRescanAllConfirm(false);
    rescanAllCookstyle()
      .then((res) => setRescanAllMsg(res.message))
      .catch((e: Error) => setRescanAllMsg(`Rescan all failed: ${e.message}`))
      .finally(() => setRescanningAll(false));
  }, []);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchAnalysisTools()
      .then((data) => {
        if (cancelled) return;
        setConfig(data.value);
        setSaved(data.value);
        const effectiveRules = data.effective_failure_rules ?? COOKSTYLE_PRESETS["default"];
        setFailureRules(effectiveRules);
        setSavedRulesJSON(JSON.stringify(normalizeRules(effectiveRules)));
        setFailurePreset(detectPreset(effectiveRules));
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof Error ? err.message : "Failed to load CookStyle settings.",
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

  if (loading) return <LoadingSpinner message="Loading CookStyle settings…" />;
  if (loadError)
    return (
      <ErrorAlert
        message="Failed to load CookStyle settings"
        detail={loadError}
        onRetry={load}
      />
    );
  if (!config) return null;

  const cookstyleEnabled = config.cookstyle_enabled ?? true;
  const configDirty = JSON.stringify(config) !== JSON.stringify(saved);
  const rulesDirty = JSON.stringify(normalizeRules(failureRules)) !== savedRulesJSON;
  const isDirty = configDirty || rulesDirty;

  function handleChange<K extends keyof AnalysisToolsConfig>(
    key: K,
    value: AnalysisToolsConfig[K],
  ) {
    setConfig((prev) => (prev ? { ...prev, [key]: value } : prev));
    setSuccessMsg(null);
  }

  function handleRulesChange(preset: CookstyleFailurePreset, rules: Record<string, string[]>) {
    setFailurePreset(preset);
    setFailureRules(rules);
    setSuccessMsg(null);
  }

  async function handleSave() {
    if (!config) return;
    setSaving(true);
    setSaveError(null);
    setSuccessMsg(null);
    const payload: AnalysisToolsConfig = {
      ...config,
      cookstyle_enabled: config.cookstyle_enabled ?? true,
      cookstyle_failure_preset: failurePreset === "custom" ? "" : failurePreset,
      cookstyle_failure_rules: failurePreset === "custom" ? failureRules : undefined,
      // Drop blank lines the operator left while editing the path list.
      cookstyle_addon_cop_paths: (config.cookstyle_addon_cop_paths ?? [])
        .map((p) => p.trim())
        .filter((p) => p !== ""),
    };
    try {
      const { value: updated, verdictsChanged } = await saveAnalysisTools(payload);
      setConfig(updated);
      setSaved(updated);
      setSavedRulesJSON(JSON.stringify(normalizeRules(failureRules)));
      const verdictText =
        verdictsChanged != null && verdictsChanged > 0
          ? ` ${verdictsChanged} cookbook verdict${verdictsChanged === 1 ? "" : "s"} changed.`
          : "";
      setSuccessMsg(`Settings saved successfully.${verdictText}`);
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save CookStyle settings.",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-6">
      <div className="max-w-3xl">
        <h2 className="text-xl font-semibold text-gray-900">CookStyle</h2>
        <p className="mt-1 text-sm text-gray-500">
          Controls CookStyle scanning behaviour and failure rules. CookStyle analyses cookbook code
          for deprecations, correctness issues, and style violations.
        </p>
      </div>

      <div className="max-w-2xl rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <label className="text-sm font-medium text-gray-700">CookStyle Scanning Enabled</label>
            <button
              type="button"
              role="switch"
              aria-checked={cookstyleEnabled}
              onClick={() => handleChange("cookstyle_enabled", !cookstyleEnabled)}
              disabled={saving}
              className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 ${
                cookstyleEnabled ? "bg-blue-600" : "bg-gray-200"
              }`}
            >
              <span
                className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                  cookstyleEnabled ? "translate-x-6" : "translate-x-1"
                }`}
              />
            </button>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              CookStyle Timeout (minutes)
            </label>
            <input
              type="number"
              min={1}
              value={config.cookstyle_timeout_minutes}
              onChange={(e) =>
                handleChange("cookstyle_timeout_minutes", Number(e.target.value))
              }
              className={INPUT_CLASS}
              disabled={saving}
            />
          </div>
        </div>
      </div>

      {/* Addon cop files — operator-supplied RuboCop cops loaded from disk */}
      <div className="max-w-2xl rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="text-base font-semibold text-gray-800">Addon Cop Files</h3>
        <p className="mt-1 text-sm text-gray-500">
          On-disk RuboCop cop files (real <code>.rb</code> cop classes) loaded into every
          scan. One entry per line — each may be a file, a directory (expanded to its{" "}
          <code>*.rb</code> files), or a glob. Files must already be deployed on the app
          host; cops are never uploaded through the UI.
        </p>
        <p className="mt-1 text-sm text-gray-500">
          Namespace cops under <code>Chef/Custom/…</code> — CMM enables each cop by name
          automatically, so a required cop is not left dormant.
        </p>
        <textarea
          aria-label="Addon cop paths"
          rows={4}
          spellCheck={false}
          value={(config.cookstyle_addon_cop_paths ?? []).join("\n")}
          onChange={(e) =>
            handleChange(
              "cookstyle_addon_cop_paths",
              e.target.value.split("\n"),
            )
          }
          placeholder={"/var/lib/chef-migration-metrics/addon-cops/*.rb"}
          className={`${INPUT_CLASS} mt-3 font-mono`}
          disabled={saving}
        />
        <p className="mt-2 text-xs text-gray-400">
          A cop file that fails to load is isolated — the affected cookbook is still scanned
          without it, and the failure is logged rather than marking the cookbook as errored.
          Use the <strong>Save</strong> button below to apply changes.
        </p>
      </div>

      {/* Rescan all — destructive maintenance action */}
      <div className="max-w-3xl rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="text-base font-semibold text-gray-800">Rescan All CookStyle</h3>
            <p className="mt-1 text-sm text-gray-500">
              Invalidate all cached CookStyle results, complexity scores, and autocorrect previews
              across every cookbook. A collection run will be triggered immediately to rescan all cookbooks.
            </p>
            <p className="mt-1 text-xs text-gray-400">
              This is useful after upgrading CookStyle, changing target Chef versions, or when
              scan results appear stale.
            </p>
          </div>
          <button
            onClick={() => setShowRescanAllConfirm(true)}
            disabled={rescanningAll}
            className="shrink-0 inline-flex items-center gap-1.5 rounded-md border border-purple-300 bg-white px-4 py-2 text-sm font-medium text-purple-700 shadow-sm hover:bg-purple-50 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {rescanningAll ? "Requesting…" : "Rescan All Cookbooks"}
          </button>
        </div>

        {showRescanAllConfirm && (
          <div className="mt-4 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            <p className="font-medium">Are you sure?</p>
            <p className="mt-1 text-amber-600">
              This will delete all cached CookStyle results, complexity scores, and autocorrect
              previews, then trigger an immediate collection run to rescan everything. This may
              take a significant amount of time depending on the number of cookbooks.
            </p>
            <div className="mt-3 flex gap-2">
              <button
                onClick={handleRescanAll}
                className="rounded-md bg-purple-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-purple-700"
              >
                Yes, Rescan All
              </button>
              <button
                onClick={() => setShowRescanAllConfirm(false)}
                className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {rescanAllMsg && (
          <div className={`mt-4 rounded-md border px-4 py-3 text-sm ${rescanAllMsg.startsWith("Rescan all failed")
            ? "border-red-200 bg-red-50 text-red-800"
            : "border-green-200 bg-green-50 text-green-800"
            }`}>
            {rescanAllMsg}
          </div>
        )}
      </div>

      {/* Separator */}
      <hr className="my-6 border-gray-200" />

      {/* Cop classifications — the primary classification surface */}
      <AdminCopClassificationsSection />

      {/* Separator */}
      <hr className="my-6 border-gray-200" />

      {/* Custom Cops section */}
      <AdminCustomCopsSection />

      {/* Separator */}
      <hr className="my-6 border-gray-200" />

      {/* Fallback rules — de-emphasised; applies only to unclassified cops */}
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-6 shadow-sm">
        <h3 className="text-lg font-medium text-gray-900">Fallback Rules</h3>
        <p className="mb-4 mt-1 text-sm text-gray-500">
          Severity-based pass/fail, applied <strong>only to unclassified cops</strong> —
          those with no operator override, <code>RemovedIn</code> mapping, or curated
          default. Classify a cop above and these rules no longer apply to it.
        </p>
        <CookstyleFailureRulesGrid
          preset={failurePreset}
          rules={failureRules}
          onChange={handleRulesChange}
          disabled={saving}
        />

        {saveError && (
          <div className="mt-4">
            <ErrorAlert message="Failed to save" detail={saveError} />
          </div>
        )}

        {successMsg && (
          <div className="mt-4 rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
            {successMsg}
          </div>
        )}

        <div className="mt-4 flex justify-end">
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
    </div>
  );
}
