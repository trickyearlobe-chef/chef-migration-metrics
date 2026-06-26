import { useCallback, useEffect, useState } from "react";
import { fetchReadinessConfig, saveReadinessConfig, type ReadinessConfig } from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

export function AdminReadinessPage() {
  const [config, setConfig] = useState<ReadinessConfig>({
    install_path_linux: "/hab",
    install_path_windows: "C:\\hab",
    install_size_mb_linux: 3072,
    install_size_mb_windows: 6144,
    min_remaining_free_percent: 20,
    review_blocks_readiness: false,
  });
  const [saved, setSaved] = useState<ReadinessConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchReadinessConfig()
      .then((data) => {
        if (cancelled) return;
        setConfig(data);
        setSaved(data);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof Error ? err.message : "Failed to load readiness settings.",
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
  const hasNonDefaultPath =
    config.install_path_linux !== "/hab" || config.install_path_windows !== "C:\\hab";

  function handleChange<K extends keyof ReadinessConfig>(key: K, value: ReadinessConfig[K]) {
    setConfig((prev) => ({ ...prev, [key]: value }));
    setSuccess(false);
  }

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    try {
      const { value: updated } = await saveReadinessConfig(config);
      setConfig(updated);
      setSaved(updated);
      setSuccess(true);
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save readiness settings.",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading readiness settings…" />;
  if (loadError)
    return (
      <ErrorAlert
        message="Failed to load readiness settings"
        detail={loadError}
        onRetry={load}
      />
    );

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Upgrade Readiness</h2>
        <p className="mt-1 text-sm text-gray-500">
          Disk space thresholds used to evaluate whether nodes have sufficient
          space for the Chef Client bundle installation.
        </p>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-4 text-sm font-semibold text-gray-900">Install Size</h3>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">
              Linux Install Size (MB)
            </label>
            <input
              type="number"
              min={1}
              value={config.install_size_mb_linux}
              onChange={(e) => handleChange("install_size_mb_linux", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Disk space in MB required for the Chef Client bundle on Linux.
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Windows Install Size (MB)
            </label>
            <input
              type="number"
              min={1}
              value={config.install_size_mb_windows}
              onChange={(e) => handleChange("install_size_mb_windows", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              Disk space in MB required for the Chef Client bundle on Windows.
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Minimum Remaining Free (%)
            </label>
            <input
              type="number"
              min={0}
              max={99}
              value={config.min_remaining_free_percent}
              onChange={(e) => handleChange("min_remaining_free_percent", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
            <p className="mt-1 text-xs text-gray-500">
              After reserving install size, at least this percentage of the filesystem must remain free.
              Both the absolute size and this percentage must pass.
            </p>
          </div>
        </div>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-4 text-sm font-semibold text-gray-900">Install Paths</h3>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">
              Linux Install Path
            </label>
            <input
              type="text"
              value={config.install_path_linux}
              onChange={(e) => handleChange("install_path_linux", e.target.value)}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">
              Windows Install Path
            </label>
            <input
              type="text"
              value={config.install_path_windows}
              onChange={(e) => handleChange("install_path_windows", e.target.value)}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </div>

          {hasNonDefaultPath && (
            <div className="rounded-md border border-amber-300 bg-amber-50 p-4">
              <div className="flex">
                <svg
                  className="h-5 w-5 shrink-0 text-amber-500"
                  viewBox="0 0 20 20"
                  fill="currentColor"
                  aria-hidden="true"
                >
                  <path
                    fillRule="evenodd"
                    d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.168 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 6a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 6zm0 9a1 1 0 100-2 1 1 0 000 2z"
                    clipRule="evenodd"
                  />
                </svg>
                <div className="ml-3">
                  <h3 className="text-sm font-medium text-amber-800">
                    Non-default install path — significant risk
                  </h3>
                  <p className="mt-1 text-xs text-amber-700">
                    While symlinking the install directory to another filesystem is supported,
                    it does not guarantee that path-dependent behaviour within Chef and its
                    cookbooks will continue to work correctly. Many cookbooks hardcode assumptions
                    about the default install location. On <strong>Windows</strong>, relocating
                    the install directory also changes the configuration directory, causing
                    failures with <code>knife bootstrap</code>. Only override paths after fully
                    understanding these risks and testing in a representative environment.
                  </p>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-4 text-sm font-semibold text-gray-900">Readiness Policy</h3>
        <label className="flex items-start gap-3">
          <input
            type="checkbox"
            checked={config.review_blocks_readiness}
            onChange={(e) => handleChange("review_blocks_readiness", e.target.checked)}
            className="mt-0.5 h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50"
            disabled={saving}
          />
          <span>
            <span className="block text-sm font-medium text-gray-700">
              Review-level cookbooks block readiness
            </span>
            <span className="mt-1 block text-xs text-gray-500">
              When enabled, a node whose only issue is Review-level CookStyle
              offences is marked <strong>Needs review</strong> (not Ready). When
              disabled (default), Review-level cookbooks count as compatible and
              readiness is gated only by Blockers and disk space. Applies at the
              next readiness evaluation.
            </span>
          </span>
        </label>
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
