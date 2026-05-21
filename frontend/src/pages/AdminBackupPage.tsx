// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import {
  fetchBackupConfig,
  saveBackupConfig,
  fetchBackups,
  fetchBackupStatus,
  createBackup,
  deleteBackup,
  restoreBackup,
  type BackupConfig,
  type BackupItem,
} from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";
import { CronDescription } from "../components/CronDescription";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString();
}

const statusColors: Record<string, string> = {
  succeeded: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
  running: "bg-blue-100 text-blue-800",
  pending: "bg-yellow-100 text-yellow-800",
  restoring: "bg-purple-100 text-purple-800",
};

export function AdminBackupPage() {
  // --- Config state ---
  const [config, setConfig] = useState<BackupConfig>({
    enabled: false,
    dir: "",
    max_generations: 7,
    schedule: "0 2 * * *",
    pg_dump_path: "",
    pg_restore_path: "",
  });
  const [savedConfig, setSavedConfig] = useState<BackupConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [configError, setConfigError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);

  // --- Backup list state ---
  const [backups, setBackups] = useState<BackupItem[]>([]);
  const [backupDir, setBackupDir] = useState<string>("");
  const [backupsLoading, setBackupsLoading] = useState(true);
  const [backupsError, setBackupsError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [confirmRestore, setConfirmRestore] = useState<string | null>(null);

  // --- Load config ---
  const loadConfig = useCallback(() => {
    setConfigLoading(true);
    setConfigError(null);
    fetchBackupConfig()
      .then((data) => {
        setConfig(data);
        setSavedConfig(data);
      })
      .catch((err: unknown) => {
        setConfigError(err instanceof Error ? err.message : "Failed to load backup config.");
      })
      .finally(() => setConfigLoading(false));
  }, []);

  // --- Load backups ---
  const loadBackups = useCallback(() => {
    setBackupsLoading(true);
    setBackupsError(null);
    fetchBackups()
      .then((resp) => {
        setBackups(resp.backups);
        setBackupDir(resp.backup_dir);
      })
      .catch((err: unknown) => {
        setBackupsError(err instanceof Error ? err.message : "Failed to load backups.");
      })
      .finally(() => setBackupsLoading(false));
  }, []);

  useEffect(() => {
    loadConfig();
    loadBackups();
  }, [loadConfig, loadBackups]);

  // Poll for status when a backup is running
  useEffect(() => {
    if (!backups.some((b) => b.status === "running" || b.status === "pending")) return;
    const interval = setInterval(() => {
      fetchBackupStatus().then((s) => {
        if (!s.active) {
          loadBackups();
          clearInterval(interval);
        }
      });
    }, 3000);
    return () => clearInterval(interval);
  }, [backups, loadBackups]);

  const isDirty = JSON.stringify(config) !== JSON.stringify(savedConfig);

  async function handleSaveConfig() {
    setSaving(true);
    setSaveError(null);
    setSaveSuccess(false);
    try {
      const { value: updated } = await saveBackupConfig(config);
      setConfig(updated);
      setSavedConfig(updated);
      setSaveSuccess(true);
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : "Failed to save backup config.");
    } finally {
      setSaving(false);
    }
  }

  async function handleCreateBackup() {
    setCreating(true);
    setActionError(null);
    try {
      await createBackup();
      loadBackups();
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : "Failed to create backup.");
    } finally {
      setCreating(false);
    }
  }

  async function handleDelete(id: string) {
    setActionError(null);
    try {
      await deleteBackup(id);
      loadBackups();
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : "Failed to delete backup.");
    }
  }

  async function handleRestore(id: string) {
    setActionError(null);
    try {
      await restoreBackup(id);
      setConfirmRestore(null);
      // App will restart — show message
      setActionError("Restore initiated. The application will restart shortly.");
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : "Restore failed.");
    }
  }

  if (configLoading) return <LoadingSpinner message="Loading backup settings…" />;

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Backup & Restore</h1>
        <p className="mt-1 text-sm text-gray-500">
          Manage database backups and configure scheduled snapshots.
        </p>
      </div>

      {/* Configuration Section */}
      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <h2 className="text-lg font-medium text-gray-900 mb-4">Configuration</h2>
        {configError && <ErrorAlert message={configError} />}

        <div className="space-y-4 max-w-lg">
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={config.enabled}
              onChange={(e) => {
                setConfig((prev) => ({ ...prev, enabled: e.target.checked }));
                setSaveSuccess(false);
              }}
              className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            <span className="text-sm font-medium text-gray-700">Enable scheduled backups</span>
          </label>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Max generations to retain
            </label>
            <input
              type="number"
              min={1}
              value={config.max_generations}
              onChange={(e) => {
                setConfig((prev) => ({ ...prev, max_generations: parseInt(e.target.value) || 1 }));
                setSaveSuccess(false);
              }}
              className={INPUT_CLASS}
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Schedule (cron)
            </label>
            <input
              type="text"
              value={config.schedule}
              onChange={(e) => {
                setConfig((prev) => ({ ...prev, schedule: e.target.value }));
                setSaveSuccess(false);
              }}
              placeholder="0 2 * * *"
              className={INPUT_CLASS}
            />
            <p className="mt-1 text-xs text-gray-500">Standard 5-field cron (min hour dom month dow). Default: 0 2 * * * (daily at 02:00)</p>
            <CronDescription expression={config.schedule} />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Backup directory (leave empty for default)
            </label>
            <input
              type="text"
              value={config.dir}
              onChange={(e) => {
                setConfig((prev) => ({ ...prev, dir: e.target.value }));
                setSaveSuccess(false);
              }}
              placeholder="<data_dir>/backups"
              className={INPUT_CLASS}
            />
          </div>

          {saveError && <ErrorAlert message={saveError} />}
          {saveSuccess && (
            <p className="text-sm text-green-600">✓ Settings saved successfully.</p>
          )}

          <button
            onClick={handleSaveConfig}
            disabled={!isDirty || saving}
            className="inline-flex items-center rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving && <InlineSpinner />}
            Save Settings
          </button>
        </div>
      </section>

      {/* Backups Section */}
      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-medium text-gray-900">Backups</h2>
          <button
            onClick={handleCreateBackup}
            disabled={creating}
            className="inline-flex items-center rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-green-700 disabled:opacity-50"
          >
            {creating && <InlineSpinner />}
            Create Backup Now
          </button>
        </div>

        {actionError && <ErrorAlert message={actionError} />}
        {backupsError && <ErrorAlert message={backupsError} />}

        {backupDir && (
          <p className="text-xs text-gray-500 mb-3">
            <span className="font-medium">Location:</span>{" "}
            <code className="bg-gray-100 px-1 py-0.5 rounded">{backupDir}</code>
          </p>
        )}

        {backupsLoading ? (
          <LoadingSpinner message="Loading backups…" />
        ) : backups.length === 0 ? (
          <p className="text-sm text-gray-500">No backups yet. Create one to get started.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 text-sm">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-2 text-left font-medium text-gray-500">Created</th>
                  <th className="px-4 py-2 text-left font-medium text-gray-500">Status</th>
                  <th className="px-4 py-2 text-left font-medium text-gray-500">Size</th>
                  <th className="px-4 py-2 text-left font-medium text-gray-500">Version</th>
                  <th className="px-4 py-2 text-left font-medium text-gray-500">Initiated By</th>
                  <th className="px-4 py-2 text-right font-medium text-gray-500">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {backups.map((b) => (
                  <tr key={b.id}>
                    <td className="px-4 py-2 whitespace-nowrap">{formatDate(b.created_at)}</td>
                    <td className="px-4 py-2">
                      <span
                        className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${statusColors[b.status] ?? "bg-gray-100 text-gray-800"}`}
                      >
                        {b.status}
                      </span>
                      {b.error && (
                        <p className="mt-1 text-xs text-red-600 max-w-md truncate" title={b.error}>
                          {b.error}
                        </p>
                      )}
                    </td>
                    <td className="px-4 py-2 whitespace-nowrap">
                      {b.size_bytes > 0 ? formatBytes(b.size_bytes) : "—"}
                    </td>
                    <td className="px-4 py-2 whitespace-nowrap text-xs">
                      <span title={`App: ${b.app_version || "?"}, Schema: ${b.schema_version}`}>
                        {b.app_version || "?"} / s{b.schema_version}
                      </span>
                    </td>
                    <td className="px-4 py-2">{b.initiated_by || "—"}</td>
                    <td className="px-4 py-2 text-right space-x-2">
                      {b.status === "succeeded" && (
                        <>
                          {confirmRestore === b.id ? (
                            <span className="inline-flex items-center gap-2">
                              <span className="text-xs text-red-600 font-medium">
                                This will overwrite all data!
                              </span>
                              <button
                                onClick={() => handleRestore(b.id)}
                                className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700"
                              >
                                Confirm
                              </button>
                              <button
                                onClick={() => setConfirmRestore(null)}
                                className="rounded bg-gray-200 px-2 py-1 text-xs text-gray-700 hover:bg-gray-300"
                              >
                                Cancel
                              </button>
                            </span>
                          ) : (
                            <button
                              onClick={() => setConfirmRestore(b.id)}
                              className="rounded bg-amber-100 px-2 py-1 text-xs text-amber-800 hover:bg-amber-200"
                            >
                              Restore
                            </button>
                          )}
                        </>
                      )}
                      <button
                        onClick={() => handleDelete(b.id)}
                        className="rounded bg-red-100 px-2 py-1 text-xs text-red-800 hover:bg-red-200"
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
      </section>
    </div>
  );
}
