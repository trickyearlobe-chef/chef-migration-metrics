import { useCallback, useEffect, useState } from "react";
import { fetchGitURLs, saveGitURLs } from "../api";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

export function AdminGitURLsPage() {
  const [urls, setUrls] = useState<string[]>([]);
  const [saved, setSaved] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchGitURLs()
      .then((data) => {
        if (cancelled) return;
        setUrls(data ?? []);
        setSaved(data ?? []);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof Error ? err.message : "Failed to load git URLs.",
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

  const isDirty = JSON.stringify(urls) !== JSON.stringify(saved);

  function handleChange(index: number, value: string) {
    setUrls((prev) => prev.map((u, i) => (i === index ? value : u)));
    setSuccess(false);
  }

  function handleRemove(index: number) {
    setUrls((prev) => prev.filter((_, i) => i !== index));
    setSuccess(false);
  }

  function handleAdd() {
    setUrls((prev) => [...prev, ""]);
    setSuccess(false);
  }

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    const trimmed = urls.map((u) => u.trim()).filter(Boolean);
    try {
      const updated = await saveGitURLs(trimmed);
      setUrls(updated ?? trimmed);
      setSaved(updated ?? trimmed);
      setSuccess(true);
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error ? err.message : "Failed to save git URLs.",
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading git URLs\u2026" />;
  if (loadError)
    return (
      <ErrorAlert
        message="Failed to load git URLs"
        detail={loadError}
        onRetry={load}
      />
    );

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Git Base URLs</h2>
        <p className="mt-1 text-sm text-gray-500">
          Base URLs searched when resolving cookbook Git repositories. The
          collector tries each URL in order when looking for a cookbook's repo.
        </p>
      </div>

      {/* URL list */}
      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="divide-y divide-gray-100">
          {urls.length === 0 ? (
            <p className="px-4 py-8 text-center text-sm text-gray-400">
              No git base URLs configured. Add one below.
            </p>
          ) : (
            urls.map((url, i) => (
              <div key={i} className="flex items-center gap-2 px-4 py-3">
                <input
                  type="url"
                  value={url}
                  onChange={(e) => handleChange(i, e.target.value)}
                  placeholder="https://github.com/my-org"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
                <button
                  type="button"
                  onClick={() => handleRemove(i)}
                  disabled={saving}
                  title="Remove"
                  className="shrink-0 rounded p-1.5 text-gray-400 hover:bg-red-50 hover:text-red-500 disabled:opacity-40"
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
                      d="M6 18 18 6M6 6l12 12"
                    />
                  </svg>
                </button>
              </div>
            ))
          )}
        </div>

        {/* Add button */}
        <div className="border-t border-gray-100 px-4 py-3">
          <button
            type="button"
            onClick={handleAdd}
            disabled={saving}
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
            Add URL
          </button>
        </div>
      </div>

      {/* Save error */}
      {saveError && <ErrorAlert message="Failed to save" detail={saveError} />}

      {/* Success banner */}
      {success && (
        <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          Git URLs saved successfully.
        </div>
      )}

      {/* Save button */}
      <div className="flex justify-end">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving || !isDirty}
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
        >
          {saving && (
            <svg
              className="h-4 w-4 animate-spin"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
              />
            </svg>
          )}
          {saving ? "Saving…" : "Save"}
        </button>
      </div>
    </div>
  );
}
