import { useCallback, useEffect, useState } from "react";
import {
  fetchAnalysisTools,
  saveAnalysisTools,
  type AnalysisToolsConfig,
  type CookstyleFailurePreset,
  COOKSTYLE_PRESETS,
} from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";
import { CookstyleFailureRulesGrid } from "../components/CookstyleFailureRulesGrid";

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

export function AdminAnalysisToolsPage() {
  const [config, setConfig] = useState<AnalysisToolsConfig>({
    embedded_bin_dir: "",
    cookstyle_enabled: true,
    cookstyle_timeout_minutes: 10,
  });
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

  const configDirty = JSON.stringify(config) !== JSON.stringify(saved);
  const rulesDirty = JSON.stringify(normalizeRules(failureRules)) !== savedRulesJSON;
  const isDirty = configDirty || rulesDirty;

  function handleChange<K extends keyof AnalysisToolsConfig>(
    key: K,
    value: AnalysisToolsConfig[K],
  ) {
    setConfig((prev) => ({ ...prev, [key]: value }));
    setSuccessMsg(null);
  }

  function handleRulesChange(preset: CookstyleFailurePreset, rules: Record<string, string[]>) {
    setFailurePreset(preset);
    setFailureRules(rules);
    setSuccessMsg(null);
  }

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    setSuccessMsg(null);
    const payload: AnalysisToolsConfig = {
      ...config,
      cookstyle_enabled: config.cookstyle_enabled ?? true,
      cookstyle_failure_preset: failurePreset === "custom" ? "" : failurePreset,
      cookstyle_failure_rules: failurePreset === "custom" ? failureRules : undefined,
    };
    try {
      const { value: updated, verdictsChanged } = await saveAnalysisTools(payload);
      setConfig(updated);
      setSaved(updated);
      const newRules = failureRules;
      setSavedRulesJSON(JSON.stringify(normalizeRules(newRules)));
      const verdictText =
        verdictsChanged != null && verdictsChanged > 0
          ? ` ${verdictsChanged} cookbook verdict${verdictsChanged === 1 ? "" : "s"} changed.`
          : "";
      setSuccessMsg(`Failure rules updated.${verdictText}`);
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

  const cookstyleEnabled = config.cookstyle_enabled ?? true;

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Analysis Tools</h2>
        <p className="mt-1 text-sm text-gray-500">
          Controls the CookStyle and Test Kitchen analysis tool configuration. Test Kitchen driver
          settings are managed separately on the Test Kitchen page.
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

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-4 text-lg font-medium text-gray-900">CookStyle Failure Rules</h3>
        <CookstyleFailureRulesGrid
          preset={failurePreset}
          rules={failureRules}
          onChange={handleRulesChange}
          disabled={saving}
        />
      </div>

      {saveError && <ErrorAlert message="Failed to save" detail={saveError} />}

      {successMsg && (
        <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          {successMsg}
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
