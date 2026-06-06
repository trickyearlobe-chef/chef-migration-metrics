import { useState, useEffect, useCallback, useMemo } from "react";
import {
  fetchTestKitchenConfig,
  saveTestKitchenConfig,
  deleteTestKitchenConfig,
  fetchCredentials,
  fetchPlatformMappingStatus,
  testHypervisorConnection,
  runOrphanSweep,
  ApiError,
} from "../api";
import type {
  TestKitchenConfig,
  ImageEntry,
  PlatformMapEntry,
  PlatformMapTransport,
  Credential,
  PlatformMappingStatusResponse,
  HypervisorTestConnectionResponse,
  SweepResult,
} from "../types";

const DRIVERS = ["proxmox", "vcenter", "vra", "ec2", "vagrant"];

const DRIVER_SETTING_HINTS: Record<string, string[]> = {
  proxmox: ["proxmox_url", "proxmox_token_id", "node"],
  vcenter: [
    "vcenter_host",
    "vcenter_username",
    "vcenter_disable_ssl_verify",
    "clone_type",
    "datacenter",
    "cluster",
    "resource_pool",
    "folder",
  ],
  ec2: [
    "region",
    "instance_type",
    "associate_public_ip",
    "subnet_id",
    "security_group_ids",
  ],
  vra: ["base_url", "username", "tenant"],
};

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";
const INPUT_FLEX_CLASS =
  "block min-w-0 rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

function emptyTransport(): PlatformMapTransport {
  return { username: "", password_credential: "", ssh_key_credential: "" };
}

function emptyImage(): ImageEntry {
  return {
    name: "",
    id: "",
    driver_settings: {},
    transport: emptyTransport(),
    chef_download_urls: {},
  };
}

function emptyConfig(): TestKitchenConfig {
  return {
    enabled: false,
    driver: "",
    timeout_minutes: 30,
    driver_settings: {},
    driver_secrets: {},
    image_field_name: "",
    chef_license_key_credential: "",
    images: [],
    platform_map: [],
    start_rate_window_minutes: 0,
    start_rate_max_per_window: 0,
  };
}

type KVPair = { key: string; value: string };

function recordToKV(rec: Record<string, unknown> | null | undefined): KVPair[] {
  if (!rec) return [];
  return Object.entries(rec).map(([key, value]) => ({
    key,
    value: String(value ?? ""),
  }));
}

function kvToRecord(pairs: KVPair[]): Record<string, string> {
  const rec: Record<string, string> = {};
  for (const { key, value } of pairs) {
    const k = key.trim();
    if (k) rec[k] = value;
  }
  return rec;
}

function PlusIcon() {
  return (
    <svg
      className="h-4 w-4"
      fill="none"
      viewBox="0 0 24 24"
      strokeWidth={2}
      stroke="currentColor"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        d="M12 4.5v15m7.5-7.5h-15"
      />
    </svg>
  );
}

function XIcon() {
  return (
    <svg
      className="h-4 w-4"
      fill="none"
      viewBox="0 0 24 24"
      strokeWidth={1.5}
      stroke="currentColor"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        d="M6 18 18 6M6 6l12 12"
      />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg
      className="h-4 w-4"
      fill="none"
      viewBox="0 0 24 24"
      strokeWidth={1.5}
      stroke="currentColor"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        d="m14.74 9-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 0 1-2.244 2.077H8.084a2.25 2.25 0 0 1-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 0 0-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 0 1 3.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 0 0-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 0 0-7.5 0"
      />
    </svg>
  );
}

function AddButton({
  onClick,
  disabled,
  children,
}: {
  onClick: () => void;
  disabled: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="mt-3 inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-50"
    >
      <PlusIcon />
      {children}
    </button>
  );
}

function RemoveButton({
  onClick,
  disabled,
  title,
}: {
  onClick: () => void;
  disabled: boolean;
  title?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title ?? "Remove"}
      className="shrink-0 rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-red-600 disabled:opacity-50"
    >
      <XIcon />
    </button>
  );
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AdminTestKitchenPage() {
  const [config, setConfig] = useState<TestKitchenConfig>(emptyConfig());

  // Top-level driver state.
  const [driverSettings, setDriverSettings] = useState<KVPair[]>([]);
  const [driverSecrets, setDriverSecrets] = useState<KVPair[]>([]);

  // Per-image state: driver settings JSON + download URL KV pairs + advanced toggle.
  const [imageDriverJson, setImageDriverJson] = useState<
    Record<number, string>
  >({});
  const [imageDownloadUrls, setImageDownloadUrls] = useState<
    Record<number, KVPair[]>
  >({});
  const [imageAdvanced, setImageAdvanced] = useState<Record<number, boolean>>(
    {},
  );

  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);

  // Hypervisor test connection state
  const [testingConnection, setTestingConnection] = useState(false);
  const [connectionResult, setConnectionResult] =
    useState<HypervisorTestConnectionResponse | null>(null);

  // Platform mapping status
  const [mappingStatus, setMappingStatus] =
    useState<PlatformMappingStatusResponse | null>(null);

  const applyConfig = useCallback(
    (c: TestKitchenConfig) => {
      setConfig(c);
      const ds = recordToKV(c.driver_settings);
      setDriverSettings(ds.length > 0 ? ds : []);
      const sec = recordToKV(c.driver_secrets);
      setDriverSecrets(sec.length > 0 ? sec : []);

      // Init per-image state.
      const djMap: Record<number, string> = {};
      const duMap: Record<number, KVPair[]> = {};
      (c.images ?? []).forEach((img, i) => {
        djMap[i] = JSON.stringify(img.driver_settings ?? {}, null, 2);
        duMap[i] = recordToKV(img.chef_download_urls ?? {});
      });
      setImageDriverJson(djMap);
      setImageDownloadUrls(duMap);
      setImageAdvanced({});
    },
    [],
  );

  const loadConfig = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setSuccess(null);
    setWarnings([]);

    Promise.all([fetchTestKitchenConfig(), fetchCredentials({ per_page: 500 })])
      .then(([tkRes, credRes]) => {
        if (cancelled) return;
        applyConfig(tkRes);
        setCredentials(credRes.data ?? []);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(
            err instanceof Error ? err.message : "Failed to load config",
          );
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [applyConfig]);

  const loadMappingStatus = useCallback(() => {
    fetchPlatformMappingStatus()
      .then(setMappingStatus)
      .catch(() => setMappingStatus(null));
  }, []);

  useEffect(() => {
    const cancel = loadConfig();
    loadMappingStatus();
    return cancel;
  }, [loadConfig, loadMappingStatus]);

  // --- Config helpers ---

  function updateConfig(patch: Partial<TestKitchenConfig>) {
    setConfig((prev) => ({ ...prev, ...patch }));
  }

  function handleDriverChange(driver: string) {
    updateConfig({ driver });
    if (driverSettings.length === 0 && DRIVER_SETTING_HINTS[driver]) {
      setDriverSettings(
        DRIVER_SETTING_HINTS[driver].map((k) => ({ key: k, value: "" })),
      );
    }
  }

  // --- Driver settings ---
  function updateSetting(idx: number, field: "key" | "value", val: string) {
    setDriverSettings((prev) => {
      const next = [...prev];
      next[idx] = { ...next[idx], [field]: val };
      return next;
    });
  }
  function removeSetting(idx: number) {
    setDriverSettings((prev) => prev.filter((_, i) => i !== idx));
  }
  function addSetting() {
    setDriverSettings((prev) => [...prev, { key: "", value: "" }]);
  }

  // --- Driver secrets ---
  function updateSecret(idx: number, field: "key" | "value", val: string) {
    setDriverSecrets((prev) => {
      const next = [...prev];
      next[idx] = { ...next[idx], [field]: val };
      return next;
    });
  }
  function removeSecret(idx: number) {
    setDriverSecrets((prev) => prev.filter((_, i) => i !== idx));
  }
  function addSecret() {
    setDriverSecrets((prev) => [...prev, { key: "", value: "" }]);
  }

  // --- Image helpers ---
  function updateImage(idx: number, patch: Partial<ImageEntry>) {
    setConfig((prev) => {
      const images = [...(prev.images ?? [])];
      images[idx] = { ...images[idx], ...patch };
      return { ...prev, images };
    });
  }
  function updateImageTransport(
    idx: number,
    patch: Partial<PlatformMapTransport>,
  ) {
    setConfig((prev) => {
      const images = [...(prev.images ?? [])];
      const existing = images[idx].transport ?? emptyTransport();
      images[idx] = { ...images[idx], transport: { ...existing, ...patch } };
      return { ...prev, images };
    });
  }
  function removeImage(idx: number) {
    setConfig((prev) => ({
      ...prev,
      images: (prev.images ?? []).filter((_, i) => i !== idx),
    }));
    setImageDriverJson((prev) => {
      const next: Record<number, string> = {};
      Object.entries(prev).forEach(([k, v]) => {
        const n = Number(k);
        if (n < idx) next[n] = v;
        else if (n > idx) next[n - 1] = v;
      });
      return next;
    });
    setImageDownloadUrls((prev) => {
      const next: Record<number, KVPair[]> = {};
      Object.entries(prev).forEach(([k, v]) => {
        const n = Number(k);
        if (n < idx) next[n] = v;
        else if (n > idx) next[n - 1] = v;
      });
      return next;
    });
    setImageAdvanced((prev) => {
      const next: Record<number, boolean> = {};
      Object.entries(prev).forEach(([k, v]) => {
        const n = Number(k);
        if (n < idx) next[n] = v;
        else if (n > idx) next[n - 1] = v;
      });
      return next;
    });
  }
  function addImage() {
    const newIdx = (config.images ?? []).length;
    setConfig((prev) => ({
      ...prev,
      images: [...(prev.images ?? []), emptyImage()],
    }));
    setImageDriverJson((prev) => ({ ...prev, [newIdx]: "{}" }));
    setImageDownloadUrls((prev) => ({ ...prev, [newIdx]: [] }));
  }
  function updateImageDownloadUrls(idx: number, pairs: KVPair[]) {
    setImageDownloadUrls((prev) => ({ ...prev, [idx]: pairs }));
  }
  function toggleImageAdvanced(idx: number) {
    setImageAdvanced((prev) => ({ ...prev, [idx]: !prev[idx] }));
  }

  // --- Save ---
  async function handleSave() {
    setSaving(true);
    setError(null);
    setSuccess(null);
    setWarnings([]);

    const images = (config.images ?? []).map((img, i) => {
      let ds: Record<string, unknown> = {};
      try {
        ds = JSON.parse(imageDriverJson[i] ?? "{}");
      } catch {
        // leave empty on parse error
      }
      const urlPairs = imageDownloadUrls[i] ?? [];
      return {
        ...img,
        driver_settings: ds,
        chef_download_urls: kvToRecord(urlPairs),
      };
    });

    const payload: TestKitchenConfig = {
      ...config,
      driver_settings: kvToRecord(driverSettings),
      driver_secrets: kvToRecord(driverSecrets),
      images,
    };

    try {
      const res = await saveTestKitchenConfig(payload);
      applyConfig(res.value);
      setWarnings([]);
      setSuccess("Configuration saved successfully.");
      loadMappingStatus();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(`Save failed: ${err.message}`);
      } else {
        setError(err instanceof Error ? err.message : "Failed to save config");
      }
    } finally {
      setSaving(false);
    }
  }

  // --- Revert ---
  async function handleRevert() {
    if (
      !window.confirm(
        "This will reset the Test Kitchen configuration to defaults. Continue?",
      )
    )
      return;

    setSaving(true);
    setError(null);
    setSuccess(null);
    setWarnings([]);

    try {
      await deleteTestKitchenConfig();
      setSuccess("Test Kitchen configuration reset to defaults.");
      loadConfig();
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(`Revert failed: ${err.message}`);
      } else {
        setError(
          err instanceof Error ? err.message : "Failed to revert config",
        );
      }
    } finally {
      setSaving(false);
    }
  }

  const credentialNames = credentials.map((c) => c.name);

  // --- Test hypervisor connection ---
  async function handleTestConnection() {
    setTestingConnection(true);
    setConnectionResult(null);
    try {
      const result = await testHypervisorConnection();
      setConnectionResult(result);
    } catch (err: unknown) {
      setConnectionResult({
        status: "error",
        message: err instanceof Error ? err.message : "Connection test failed",
      });
    } finally {
      setTestingConnection(false);
    }
  }

  const imageNames = (config.images ?? [])
    .map((img) => img.name)
    .filter(Boolean);

  // --- Loading state ---
  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <svg
          className="h-6 w-6 animate-spin text-blue-600"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
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
        <span className="ml-2 text-sm text-gray-500">
          Loading configuration…
        </span>
      </div>
    );
  }

  // --- Render ---
  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-800">
            Test Kitchen Configuration
          </h2>
          <p className="text-sm text-gray-500">
            Configuration is stored in the encrypted database config store.
          </p>
        </div>
      </div>

      {/* Banners */}
      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}
      {success && (
        <div className="rounded-md border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700">
          {success}
        </div>
      )}
      {warnings.length > 0 && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">
          <p className="font-medium">Warnings:</p>
          <ul className="mt-1 list-inside list-disc">
            {warnings.map((w, i) => (
              <li key={i}>{w}</li>
            ))}
          </ul>
        </div>
      )}

      {/* Section 1: Driver Configuration */}
      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-gray-500">
          Driver Configuration
        </h3>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">
              Driver
            </label>
            <select
              value={config.driver}
              onChange={(e) => handleDriverChange(e.target.value)}
              disabled={saving}
              className={INPUT_CLASS}
            >
              {config.driver === "" && (
                <option value="" disabled>
                  — Select driver —
                </option>
              )}
              {DRIVERS.map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">
              Timeout (minutes)
            </label>
            <input
              type="number"
              min={1}
              value={config.timeout_minutes}
              onChange={(e) =>
                updateConfig({ timeout_minutes: Number(e.target.value) || 30 })
              }
              disabled={saving}
              className={INPUT_CLASS}
            />
          </div>

          <div className="flex items-center gap-2 sm:col-span-2">
            <input
              type="checkbox"
              id="tk-enabled"
              checked={config.enabled === true}
              onChange={(e) => updateConfig({ enabled: e.target.checked })}
              disabled={saving}
              className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            <label
              htmlFor="tk-enabled"
              className="text-sm font-medium text-gray-700"
            >
              Enabled
            </label>
          </div>
        </div>
      </div>

      {/* Section 1b: VM Start-Rate Limiting */}
      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-1 text-sm font-semibold uppercase tracking-wider text-gray-500">
          VM Start-Rate Limiting
        </h3>
        <p className="mb-4 text-sm text-gray-500">
          Caps cumulative DHCP lease consumption: no more than{" "}
          <em>max starts</em> in any trailing <em>window</em>, evenly paced.
          Set the window to your DHCP lease time and max starts to the usable
          pool size. Leave either at 0 to disable. Changes apply with no
          restart.
        </p>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label
              htmlFor="tk-start-rate-window"
              className="mb-1 block text-sm font-medium text-gray-700"
            >
              Window (minutes) — DHCP lease time
            </label>
            <input
              id="tk-start-rate-window"
              type="number"
              min={0}
              value={config.start_rate_window_minutes}
              onChange={(e) =>
                updateConfig({
                  start_rate_window_minutes: Number(e.target.value) || 0,
                })
              }
              disabled={saving}
              className={INPUT_CLASS}
            />
          </div>

          <div>
            <label
              htmlFor="tk-start-rate-max"
              className="mb-1 block text-sm font-medium text-gray-700"
            >
              Max starts per window — usable pool size
            </label>
            <input
              id="tk-start-rate-max"
              type="number"
              min={0}
              value={config.start_rate_max_per_window}
              onChange={(e) =>
                updateConfig({
                  start_rate_max_per_window: Number(e.target.value) || 0,
                })
              }
              disabled={saving}
              className={INPUT_CLASS}
            />
          </div>
        </div>
      </div>

      {/* Section 2: Driver Settings */}
      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-gray-500">
          Driver Settings
        </h3>
        {driverSettings.length === 0 && (
          <p className="mb-3 text-sm text-gray-400">
            No driver settings configured.
          </p>
        )}
        <div className="space-y-2">
          {driverSettings.map((pair, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <input
                type="text"
                value={pair.key}
                onChange={(e) => updateSetting(idx, "key", e.target.value)}
                placeholder="Key"
                disabled={saving}
                className={INPUT_CLASS}
              />
              <input
                type="text"
                value={pair.value}
                onChange={(e) => updateSetting(idx, "value", e.target.value)}
                placeholder="Value"
                disabled={saving}
                className={INPUT_CLASS}
              />
              <RemoveButton
                onClick={() => removeSetting(idx)}
                disabled={saving}
              />
            </div>
          ))}
        </div>
        <AddButton onClick={addSetting} disabled={saving}>
          Add Setting
        </AddButton>
      </div>

      {/* Section 3: Driver Secrets */}
      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-gray-500">
          Driver Secrets
        </h3>
        {driverSecrets.length === 0 && (
          <p className="mb-3 text-sm text-gray-400">
            No driver secrets configured.
          </p>
        )}
        <div className="space-y-2">
          {driverSecrets.map((pair, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <input
                type="text"
                value={pair.key}
                onChange={(e) => updateSecret(idx, "key", e.target.value)}
                placeholder="Setting key"
                disabled={saving}
                className={INPUT_CLASS}
              />
              <select
                value={pair.value}
                onChange={(e) => updateSecret(idx, "value", e.target.value)}
                disabled={saving}
                className={INPUT_CLASS}
              >
                <option value="">— select credential —</option>
                {credentialNames.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
              <RemoveButton
                onClick={() => removeSecret(idx)}
                disabled={saving}
              />
            </div>
          ))}
        </div>
        <AddButton onClick={addSecret} disabled={saving}>
          Add Secret
        </AddButton>
      </div>

      {/* Test Connection */}
      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-500">
              Connection Test
            </h3>
            <p className="mt-1 text-xs text-gray-400">
              Verify connectivity to the hypervisor and discover available templates.
            </p>
          </div>
          <button
            onClick={handleTestConnection}
            disabled={testingConnection || saving}
            className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
          >
            {testingConnection ? (
              <>
                <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Testing…
              </>
            ) : (
              "Test Connection"
            )}
          </button>
        </div>

        {connectionResult && (
          <div className="mt-4">
            {connectionResult.status === "ok" && (
              <div className="rounded-md border border-green-200 bg-green-50 p-4">
                <div className="flex items-center gap-2">
                  <svg className="h-5 w-5 text-green-600" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <span className="text-sm font-medium text-green-800">
                    Connected to {connectionResult.hypervisor_type} — {connectionResult.template_count} template{connectionResult.template_count !== 1 ? "s" : ""} found
                  </span>
                </div>
                {connectionResult.templates && connectionResult.templates.length > 0 && (
                  <div className="mt-3">
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="border-b border-green-200 text-left text-green-700">
                          <th className="pb-1 pr-4">Name</th>
                          <th className="pb-1 pr-4">Guest OS</th>
                          <th className="pb-1">ID</th>
                        </tr>
                      </thead>
                      <tbody className="text-green-900">
                        {connectionResult.templates.map((tmpl) => (
                          <tr key={tmpl.id} className="border-b border-green-100 last:border-0">
                            <td className="py-1 pr-4 font-medium">{tmpl.name}</td>
                            <td className="py-1 pr-4">{tmpl.guest_os || "—"}</td>
                            <td className="py-1 font-mono text-green-700">{tmpl.id}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            )}
            {connectionResult.status === "error" && (
              <div className="rounded-md border border-red-200 bg-red-50 p-4">
                <div className="flex items-center gap-2">
                  <svg className="h-5 w-5 text-red-600" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
                  </svg>
                  <span className="text-sm font-medium text-red-800">Connection failed</span>
                </div>
                <p className="mt-2 text-sm text-red-700">{connectionResult.message}</p>
              </div>
            )}
            {connectionResult.status === "not_configured" && (
              <div className="rounded-md border border-yellow-200 bg-yellow-50 p-4">
                <div className="flex items-center gap-2">
                  <svg className="h-5 w-5 text-yellow-600" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126z" />
                  </svg>
                  <span className="text-sm font-medium text-yellow-800">{connectionResult.message}</span>
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Section 4: Provisioner */}
      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-1 text-sm font-semibold uppercase tracking-wider text-gray-500">
          Provisioner
        </h3>
        <p className="mb-4 text-xs text-gray-400">
          Chef client versions come from <code>target_chef_versions</code> in
          your config file. Set a license key credential as a fallback for
          platforms without a per-image download URL.
        </p>
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">
            Chef License Key Credential
          </label>
          <select
            value={config.chef_license_key_credential ?? ""}
            onChange={(e) =>
              updateConfig({ chef_license_key_credential: e.target.value })
            }
            disabled={saving}
            className={INPUT_CLASS}
          >
            <option value="">
              — none (use per-image download URLs only) —
            </option>
            {credentialNames.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Section 5: Images */}
      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-1 text-sm font-semibold uppercase tracking-wider text-gray-500">
          Images
        </h3>
        <p className="mb-4 text-xs text-gray-400">
          Define infrastructure images once. Each image has a driver ID,
          optional transport credentials, and per-version Chef download URLs.
        </p>
        {(config.images ?? []).length === 0 && (
          <p className="mb-3 text-sm text-gray-400">No images defined.</p>
        )}
        <div className="space-y-4">
          {(config.images ?? []).map((img, idx) => (
            <div
              key={idx}
              className="rounded-md border border-gray-200 bg-gray-50 p-4"
            >
              <div className="mb-3 flex items-center justify-between">
                <span className="text-sm font-medium text-gray-700">
                  Image {idx + 1}
                  {img.name ? ` — ${img.name}` : ""}
                </span>
                <button
                  type="button"
                  onClick={() => removeImage(idx)}
                  disabled={saving}
                  title="Remove image"
                  className="rounded p-1 text-gray-400 hover:bg-gray-200 hover:text-red-600 disabled:opacity-50"
                >
                  <TrashIcon />
                </button>
              </div>

              {/* Name + ID */}
              <div className="mb-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-600">
                    Name (unique label)
                  </label>
                  <input
                    type="text"
                    value={img.name}
                    onChange={(e) => updateImage(idx, { name: e.target.value })}
                    placeholder="e.g. alma10"
                    disabled={saving}
                    className={INPUT_CLASS}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-600">
                    Infrastructure ID
                  </label>
                  {connectionResult?.templates && connectionResult.templates.length > 0 ? (
                    <select
                      value={img.id}
                      onChange={(e) => updateImage(idx, { id: e.target.value })}
                      disabled={saving}
                      className={INPUT_CLASS}
                    >
                      <option value="">— select template —</option>
                      {connectionResult.templates.map((tmpl) => (
                        <option key={tmpl.id} value={tmpl.name}>
                          {tmpl.name}
                        </option>
                      ))}
                      {img.id && !connectionResult.templates.some((t) => t.name === img.id) && (
                        <option value={img.id}>{img.id} (custom)</option>
                      )}
                    </select>
                  ) : (
                    <input
                      type="text"
                      value={img.id}
                      onChange={(e) => updateImage(idx, { id: e.target.value })}
                      placeholder="e.g. template-rhel9"
                      disabled={saving}
                      className={INPUT_CLASS}
                    />
                  )}
                </div>
              </div>

              {/* Baked-in Chef toggle */}
              <div className="mb-3">
                <label className="flex items-center gap-2 text-xs font-medium text-gray-600">
                  <input
                    type="checkbox"
                    checked={img.install_method === "baked_in"}
                    onChange={(e) =>
                      updateImage(idx, {
                        install_method: e.target.checked ? "baked_in" : "download",
                        chef_client_path: e.target.checked ? (img.chef_client_path || "/opt/chef/bin/chef-client") : "",
                      })
                    }
                    disabled={saving}
                    className="rounded border-gray-300"
                  />
                  Chef is baked into this image (no download/install needed)
                </label>
                {img.install_method === "baked_in" && (
                  <div className="mt-2">
                    <label className="mb-1 block text-xs font-medium text-gray-600">
                      chef-client binary path
                    </label>
                    <input
                      type="text"
                      value={img.chef_client_path ?? ""}
                      onChange={(e) => updateImage(idx, { chef_client_path: e.target.value })}
                      placeholder="/opt/chef/bin/chef-client"
                      disabled={saving}
                      className={INPUT_CLASS}
                    />
                  </div>
                )}
              </div>

              {/* Chef Download URLs */}
              <div className="mb-3">
                <label className="mb-1 block text-xs font-medium text-gray-600">
                  Chef Download URLs (version → URL)
                </label>
                <div className="space-y-1">
                  {(imageDownloadUrls[idx] ?? []).map((pair, uidx) => (
                    <div key={uidx} className="flex items-center gap-2">
                      <input
                        type="text"
                        value={pair.key}
                        onChange={(e) => {
                          const next = [...(imageDownloadUrls[idx] ?? [])];
                          next[uidx] = { ...next[uidx], key: e.target.value };
                          updateImageDownloadUrls(idx, next);
                        }}
                        placeholder="19.2.12"
                        disabled={saving}
                        className={`${INPUT_CLASS} w-32 shrink-0`}
                      />
                      <input
                        type="text"
                        value={pair.value}
                        onChange={(e) => {
                          const next = [...(imageDownloadUrls[idx] ?? [])];
                          next[uidx] = { ...next[uidx], value: e.target.value };
                          updateImageDownloadUrls(idx, next);
                        }}
                        placeholder="https://packages.example.com/chef-19.rpm"
                        disabled={saving}
                        className={INPUT_CLASS}
                      />
                      <RemoveButton
                        onClick={() => {
                          updateImageDownloadUrls(
                            idx,
                            (imageDownloadUrls[idx] ?? []).filter(
                              (_, i) => i !== uidx,
                            ),
                          );
                        }}
                        disabled={saving}
                      />
                    </div>
                  ))}
                </div>
                <AddButton
                  onClick={() =>
                    updateImageDownloadUrls(idx, [
                      ...(imageDownloadUrls[idx] ?? []),
                      { key: "", value: "" },
                    ])
                  }
                  disabled={saving}
                >
                  Add URL
                </AddButton>
              </div>

              {/* Transport + Driver Settings (advanced toggle) */}
              <button
                type="button"
                onClick={() => toggleImageAdvanced(idx)}
                className="text-xs font-medium text-blue-600 hover:text-blue-800"
              >
                {imageAdvanced[idx]
                  ? "▾ Hide Advanced"
                  : "▸ Show Advanced (transport & driver settings)"}
              </button>
              {imageAdvanced[idx] && (
                <div className="mt-3 space-y-3">
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                    <div>
                      <label className="mb-1 block text-xs font-medium text-gray-600">
                        Transport User
                      </label>
                      <input
                        type="text"
                        value={img.transport?.username ?? ""}
                        onChange={(e) =>
                          updateImageTransport(idx, {
                            username: e.target.value,
                          })
                        }
                        placeholder="e.g. root"
                        disabled={saving}
                        className={INPUT_CLASS}
                      />
                    </div>
                    <div>
                      <label className="mb-1 block text-xs font-medium text-gray-600">
                        Password Credential
                      </label>
                      <select
                        value={img.transport?.password_credential ?? ""}
                        onChange={(e) =>
                          updateImageTransport(idx, {
                            password_credential: e.target.value,
                          })
                        }
                        disabled={saving}
                        className={INPUT_CLASS}
                      >
                        <option value="">— none —</option>
                        {credentialNames.map((name) => (
                          <option key={name} value={name}>
                            {name}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div>
                      <label className="mb-1 block text-xs font-medium text-gray-600">
                        SSH Key Credential
                      </label>
                      <select
                        value={img.transport?.ssh_key_credential ?? ""}
                        onChange={(e) =>
                          updateImageTransport(idx, {
                            ssh_key_credential: e.target.value,
                          })
                        }
                        disabled={saving}
                        className={INPUT_CLASS}
                      >
                        <option value="">— none —</option>
                        {credentialNames.map((name) => (
                          <option key={name} value={name}>
                            {name}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-gray-600">
                      Per-Image Driver Settings (JSON)
                    </label>
                    <textarea
                      value={imageDriverJson[idx] ?? "{}"}
                      onChange={(e) =>
                        setImageDriverJson((prev) => ({
                          ...prev,
                          [idx]: e.target.value,
                        }))
                      }
                      rows={4}
                      disabled={saving}
                      className={`${INPUT_CLASS} font-mono text-xs`}
                    />
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
        <AddButton onClick={addImage} disabled={saving}>
          Add Image
        </AddButton>
      </div>

      {/* Section 6: Platform Map */}
      <PlatformMapSection
        mappingStatus={mappingStatus}
        config={config}
        setConfig={setConfig}
        imageNames={imageNames}
        saving={saving}
      />

      {/* Section 7: Orphan Sweep */}
      <OrphanSweepSection />

      {/* Footer: Save + Reset */}
      <div className="flex items-center justify-between border-t border-gray-200 pt-4">
        <button
          type="button"
          onClick={handleRevert}
          disabled={saving}
          title="Clear Test Kitchen configuration and reset to defaults"
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-50"
        >
          Reset to Defaults
        </button>
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
        >
          {saving ? "Saving…" : "Save Configuration"}
        </button>
      </div>
    </div>
  );
}

function OrphanSweepSection() {
  const [dryRun, setDryRun] = useState(true);
  const [sweeping, setSweeping] = useState(false);
  const [result, setResult] = useState<SweepResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function handleSweep() {
    setSweeping(true);
    setError(null);
    setResult(null);
    try {
      const res = await runOrphanSweep(dryRun);
      setResult(res);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Sweep failed");
    } finally {
      setSweeping(false);
    }
  }

  function formatAge(seconds: number): string {
    const mins = Math.round(seconds / 60);
    if (mins < 60) return `${mins}m`;
    const hrs = Math.floor(mins / 60);
    const rem = mins % 60;
    return `${hrs}h ${rem}m`;
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
      <h3 className="mb-1 text-sm font-semibold uppercase tracking-wider text-gray-500">
        VM Orphan Sweep
      </h3>
      <p className="mb-4 text-xs text-gray-400">
        Scan the hypervisor for stale Test Kitchen VMs and optionally destroy
        them.
      </p>

      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <input
            type="checkbox"
            id="sweep-dry-run"
            checked={dryRun}
            onChange={(e) => setDryRun(e.target.checked)}
            disabled={sweeping}
            className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
          />
          <label
            htmlFor="sweep-dry-run"
            className="text-sm font-medium text-gray-700"
          >
            Dry Run
          </label>
        </div>
        <button
          type="button"
          onClick={handleSweep}
          disabled={sweeping}
          className="inline-flex items-center rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50"
        >
          {sweeping ? "Sweeping…" : "Run Sweep"}
        </button>
      </div>

      {error && (
        <div className="mt-4 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </div>
      )}

      {result && (
        <div className="mt-4 space-y-3">
          <div className="flex flex-wrap gap-3 text-sm">
            {result.dry_run && (
              <span className="inline-flex rounded-full bg-purple-100 px-2 py-0.5 text-xs font-medium text-purple-700">
                dry run
              </span>
            )}
            <span className="text-gray-600">
              Scanned: <strong>{result.scanned}</strong>
            </span>
            <span className="text-gray-600">
              Destroyed: <strong>{result.destroyed}</strong>
            </span>
            <span className="text-gray-600">
              Too young: <strong>{result.skipped_too_young}</strong>
            </span>
            <span className="text-gray-600">
              Unparsed: <strong>{result.skipped_unparsed}</strong>
            </span>
            {result.errors > 0 && (
              <span className="text-red-600">
                Errors: <strong>{result.errors}</strong>
              </span>
            )}
          </div>

          {result.details.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-gray-200 text-[11px] uppercase tracking-wider text-gray-500">
                    <th className="pb-2 pr-3 font-medium">VM Name</th>
                    <th className="pb-2 pr-3 font-medium">Age</th>
                    <th className="pb-2 pr-3 font-medium">Action</th>
                    <th className="pb-2 font-medium">Error</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {result.details.map((d, i) => (
                    <tr key={i}>
                      <td className="py-1.5 pr-3 font-medium text-gray-800">
                        {d.vm_name}
                      </td>
                      <td className="py-1.5 pr-3 tabular-nums text-gray-600">
                        {formatAge(d.age_seconds)}
                      </td>
                      <td className="py-1.5 pr-3">
                        <span
                          className={`inline-flex rounded-full px-1.5 py-0.5 text-[10px] font-medium ${
                            d.action === "destroyed"
                              ? "bg-red-100 text-red-700"
                              : d.action === "would_destroy"
                                ? "bg-amber-100 text-amber-700"
                                : "bg-gray-100 text-gray-600"
                          }`}
                        >
                          {d.action}
                        </span>
                      </td>
                      <td className="py-1.5 text-red-600">
                        {d.error ?? "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

const OS_FAMILY_BADGE_COLORS: Record<string, string> = {
  rhel: "bg-red-100 text-red-700",
  windows: "bg-blue-100 text-blue-700",
  debian: "bg-orange-100 text-orange-700",
  suse: "bg-green-100 text-green-700",
};

function OsFamilyBadge({ family }: { family: string }) {
  const cls =
    OS_FAMILY_BADGE_COLORS[family.toLowerCase()] ?? "bg-gray-100 text-gray-600";
  return (
    <span
      className={`inline-flex rounded-full px-1.5 py-0.5 text-[10px] font-medium ${cls}`}
    >
      {family}
    </span>
  );
}

const SOURCE_BADGE_COLORS: Record<string, string> = {
  kitchen: "bg-blue-100 text-blue-700",
  nodes: "bg-purple-100 text-purple-700",
  both: "bg-green-100 text-green-700",
};

function SourceBadge({ source }: { source: string }) {
  const cls = SOURCE_BADGE_COLORS[source] ?? "bg-gray-100 text-gray-600";
  return (
    <span
      className={`inline-flex rounded-full px-1.5 py-0.5 text-[10px] font-medium ${cls}`}
    >
      {source}
    </span>
  );
}

function PlatformMapSection({
  mappingStatus,
  config,
  setConfig,
  imageNames,
  saving,
}: {
  mappingStatus: PlatformMappingStatusResponse | null;
  config: TestKitchenConfig;
  setConfig: React.Dispatch<React.SetStateAction<TestKitchenConfig>>;
  imageNames: string[];
  saving: boolean;
}) {
  const discoveredPlatforms = mappingStatus?.discovered_platforms ?? [];

  const initMappings = useCallback((): Record<string, string> => {
    const m: Record<string, string> = {};
    for (const p of discoveredPlatforms) {
      m[p.platform_name] =
        p.mapping_status === "mapped" ? p.matched_image : "";
    }
    return m;
  }, [discoveredPlatforms]);

  const [platformMappings, setPlatformMappings] = useState<
    Record<string, string>
  >(initMappings);

  useEffect(() => {
    setPlatformMappings(initMappings());
  }, [initMappings]);

  const syncToConfig = useCallback(
    (mappings: Record<string, string>) => {
      setConfig((prev) => {
        const entries: PlatformMapEntry[] = Object.entries(mappings).map(
          ([name, image]) =>
            image
              ? { kitchen_name: name, image }
              : { kitchen_name: name, image: "", skip: true },
        );
        return { ...prev, platform_map: entries };
      });
    },
    [setConfig],
  );

  function handleImageChange(platformName: string, image: string) {
    const updated = { ...platformMappings, [platformName]: image };
    setPlatformMappings(updated);
    syncToConfig(updated);
  }

  const sortedPlatforms = useMemo(() => {
    return [...discoveredPlatforms].sort((a, b) => {
      const aHasImage = platformMappings[a.platform_name] ? 0 : 1;
      const bHasImage = platformMappings[b.platform_name] ? 0 : 1;
      if (aHasImage !== bHasImage) return aHasImage - bHasImage;
      return a.platform_name.localeCompare(b.platform_name);
    });
  }, [discoveredPlatforms, platformMappings]);

  const hasNoPlatforms =
    discoveredPlatforms.length === 0 &&
    (config.platform_map ?? []).length === 0;

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
      <div className="mb-4 flex items-start justify-between">
        <div>
          <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-500">
            Platform Map
          </h3>
          <p className="mt-1 text-xs text-gray-400">
            Discovered platforms from kitchen configs and nodes. Select an image
            or leave as skip.
          </p>
        </div>
        {mappingStatus && (
          <div className="flex gap-2 text-xs">
            <span className="inline-flex items-center rounded-full bg-green-100 px-2.5 py-0.5 font-medium text-green-700">
              {mappingStatus.mapped_count} mapped
            </span>
            <span className="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-0.5 font-medium text-gray-600">
              {mappingStatus.skipped_count} skipped
            </span>
            {mappingStatus.unmapped_count > 0 && (
              <span className="inline-flex items-center rounded-full bg-red-100 px-2.5 py-0.5 font-medium text-red-700">
                {mappingStatus.unmapped_count} unmapped
              </span>
            )}
          </div>
        )}
      </div>

      {hasNoPlatforms && (
        <p className="text-sm text-gray-400">
          No platforms discovered yet. Run kitchen analysis to discover
          platforms.
        </p>
      )}

      {sortedPlatforms.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-gray-200 text-[11px] uppercase tracking-wider text-gray-500">
                <th className="pb-2 pr-3 font-medium">Platform</th>
                <th className="pb-2 pr-3 font-medium">Source</th>
                <th className="pb-2 pr-3 font-medium">OS</th>
                <th className="pb-2 pr-3 font-medium">Count</th>
                <th className="pb-2 font-medium">Image</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {sortedPlatforms.map((p) => (
                <tr key={p.platform_name}>
                  <td className="py-2 pr-3 font-medium text-gray-800">
                    {p.display_name ? (
                      <span title={p.platform_name} className="cursor-help border-b border-dotted border-gray-400">
                        {p.display_name}
                      </span>
                    ) : (
                      p.platform_name
                    )}
                  </td>
                  <td className="py-2 pr-3">
                    <SourceBadge source={p.source} />
                  </td>
                  <td className="py-2 pr-3">
                    <OsFamilyBadge family={p.os_family} />
                  </td>
                  <td className="py-2 pr-3 tabular-nums text-gray-600">
                    {p.cookbook_count > 0 &&
                      `${p.cookbook_count} cookbook${p.cookbook_count !== 1 ? "s" : ""}`}
                    {p.cookbook_count > 0 && p.node_count > 0 && ", "}
                    {p.node_count > 0 &&
                      `${p.node_count} node${p.node_count !== 1 ? "s" : ""}`}
                  </td>
                  <td className="py-2">
                    <select
                      value={platformMappings[p.platform_name] ?? ""}
                      onChange={(e) =>
                        handleImageChange(p.platform_name, e.target.value)
                      }
                      disabled={saving}
                      className={INPUT_FLEX_CLASS + " flex-1 min-w-0"}
                    >
                      <option value="">— skip —</option>
                      {imageNames.map((name) => (
                        <option key={name} value={name}>
                          {name}
                        </option>
                      ))}
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
