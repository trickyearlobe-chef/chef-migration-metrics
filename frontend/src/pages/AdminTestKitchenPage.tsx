import { useState, useEffect, useCallback } from "react";
import {
  fetchTestKitchenConfig,
  saveTestKitchenConfig,
  deleteTestKitchenConfig,
  fetchCredentials,
  ApiError,
} from "../api";
import type {
  TestKitchenConfig,
  PlatformMapEntry,
  PlatformMapTransport,
  Credential,
} from "../types";

const DRIVERS = [
  "dokken",
  "vcenter",
  "vra",
  "ec2",
  "azurerm",
  "google",
  "vagrant",
  "openstack",
  "custom",
];

const DRIVER_SETTING_HINTS: Record<string, string[]> = {
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
  azurerm: ["subscription_id", "location", "machine_size"],
  google: ["project", "zone", "machine_type"],
};

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

function emptyTransport(): PlatformMapTransport {
  return { username: "", password_credential: "", ssh_key_credential: "" };
}

function emptyPlatform(): PlatformMapEntry {
  return {
    kitchen_name: "",
    image: "",
    driver_settings: {},
    transport: emptyTransport(),
  };
}

function emptyConfig(): TestKitchenConfig {
  return {
    enabled: false,
    driver: "dokken",
    timeout_minutes: 30,
    driver_settings: {},
    driver_secrets: {},
    image_field_name: "",
    platform_map: [],
  };
}

type KVPair = { key: string; value: string };

function recordToKV(rec: Record<string, unknown>): KVPair[] {
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

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AdminTestKitchenPage() {
  const [config, setConfig] = useState<TestKitchenConfig>(emptyConfig());
  const [source, setSource] = useState<"database" | "file">("file");
  const [updatedAt, setUpdatedAt] = useState<string | undefined>();
  const [updatedBy, setUpdatedBy] = useState<string | undefined>();

  const [driverSettings, setDriverSettings] = useState<KVPair[]>([]);
  const [driverSecrets, setDriverSecrets] = useState<KVPair[]>([]);
  const [platformAdvanced, setPlatformAdvanced] = useState<
    Record<number, boolean>
  >({});
  const [platformDriverJson, setPlatformDriverJson] = useState<
    Record<number, string>
  >({});

  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);

  const applyConfig = useCallback(
    (
      c: TestKitchenConfig,
      src: "database" | "file",
      at?: string,
      by?: string,
    ) => {
      setConfig(c);
      setSource(src);
      setUpdatedAt(at);
      setUpdatedBy(by);
      const ds = recordToKV(c.driver_settings);
      setDriverSettings(ds.length > 0 ? ds : []);
      const sec = recordToKV(c.driver_secrets);
      setDriverSecrets(sec.length > 0 ? sec : []);
      const jsonMap: Record<number, string> = {};
      (c.platform_map ?? []).forEach((p, i) => {
        jsonMap[i] = JSON.stringify(p.driver_settings ?? {}, null, 2);
      });
      setPlatformDriverJson(jsonMap);
      setPlatformAdvanced({});
    },
    [],
  );

  const loadConfig = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setSuccess(null);
    setWarnings([]);

    Promise.all([
      fetchTestKitchenConfig(),
      fetchCredentials({ per_page: 500 }),
    ])
      .then(([tkRes, credRes]) => {
        if (cancelled) return;
        applyConfig(
          tkRes.config,
          tkRes.source,
          tkRes.updated_at,
          tkRes.updated_by,
        );
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

  useEffect(() => {
    const cancel = loadConfig();
    return cancel;
  }, [loadConfig]);

  // --- Helpers ---

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

  function updatePlatform(idx: number, patch: Partial<PlatformMapEntry>) {
    setConfig((prev) => {
      const platforms = [...(prev.platform_map ?? [])];
      platforms[idx] = { ...platforms[idx], ...patch };
      return { ...prev, platform_map: platforms };
    });
  }

  function updateTransport(idx: number, patch: Partial<PlatformMapTransport>) {
    setConfig((prev) => {
      const platforms = [...(prev.platform_map ?? [])];
      const existing = platforms[idx].transport ?? emptyTransport();
      platforms[idx] = {
        ...platforms[idx],
        transport: { ...existing, ...patch },
      };
      return { ...prev, platform_map: platforms };
    });
  }

  function removePlatform(idx: number) {
    setConfig((prev) => ({
      ...prev,
      platform_map: (prev.platform_map ?? []).filter((_, i) => i !== idx),
    }));
    setPlatformDriverJson((prev) => {
      const next: Record<number, string> = {};
      Object.entries(prev).forEach(([k, v]) => {
        const n = Number(k);
        if (n < idx) next[n] = v;
        else if (n > idx) next[n - 1] = v;
      });
      return next;
    });
    setPlatformAdvanced((prev) => {
      const next: Record<number, boolean> = {};
      Object.entries(prev).forEach(([k, v]) => {
        const n = Number(k);
        if (n < idx) next[n] = v;
        else if (n > idx) next[n - 1] = v;
      });
      return next;
    });
  }

  function addPlatform() {
    const newIdx = (config.platform_map ?? []).length;
    setConfig((prev) => ({
      ...prev,
      platform_map: [...(prev.platform_map ?? []), emptyPlatform()],
    }));
    setPlatformDriverJson((prev) => ({ ...prev, [newIdx]: "{}" }));
  }

  function togglePlatformAdvanced(idx: number) {
    setPlatformAdvanced((prev) => ({ ...prev, [idx]: !prev[idx] }));
  }

  function updatePlatformDriverJson(idx: number, val: string) {
    setPlatformDriverJson((prev) => ({ ...prev, [idx]: val }));
  }

  // --- Save ---
  async function handleSave() {
    setSaving(true);
    setError(null);
    setSuccess(null);
    setWarnings([]);

    const platforms = (config.platform_map ?? []).map((p, i) => {
      let ds: Record<string, unknown> = {};
      try {
        ds = JSON.parse(platformDriverJson[i] ?? "{}");
      } catch {
        // leave empty on parse error
      }
      return { ...p, driver_settings: ds };
    });

    const payload: TestKitchenConfig = {
      ...config,
      driver_settings: kvToRecord(driverSettings),
      driver_secrets: kvToRecord(driverSecrets),
      platform_map: platforms,
    };

    try {
      const res = await saveTestKitchenConfig(payload);
      applyConfig(
        res.config,
        res.source as "database" | "file",
        res.updated_at,
        res.updated_by,
      );
      setWarnings(res.warnings ?? []);
      setSuccess("Configuration saved successfully.");
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
        "This will delete the database configuration and revert to the file-based config. Continue?",
      )
    )
      return;

    setSaving(true);
    setError(null);
    setSuccess(null);
    setWarnings([]);

    try {
      await deleteTestKitchenConfig();
      setSuccess("Reverted to file configuration.");
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
            Currently using:{" "}
            <span className="font-medium text-gray-700">
              {source} config
            </span>
            {updatedBy && (
              <span className="ml-2 text-gray-400">
                · last saved by {updatedBy}
                {updatedAt && (
                  <>
                    {" "}
                    at {new Date(updatedAt).toLocaleString()}
                  </>
                )}
              </span>
            )}
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

          {config.driver === "custom" && (
            <div className="sm:col-span-2">
              <label className="mb-1 block text-sm font-medium text-gray-700">
                Image Field Name
              </label>
              <input
                type="text"
                value={config.image_field_name}
                onChange={(e) =>
                  updateConfig({ image_field_name: e.target.value })
                }
                disabled={saving}
                placeholder="e.g. image_id"
                className={INPUT_CLASS}
              />
            </div>
          )}
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
              <button
                type="button"
                onClick={() => removeSetting(idx)}
                disabled={saving}
                title="Remove"
                className="shrink-0 rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-red-600 disabled:opacity-50"
              >
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
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={addSetting}
          disabled={saving}
          className="mt-3 inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-50"
        >
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
          Add Setting
        </button>
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
              <button
                type="button"
                onClick={() => removeSecret(idx)}
                disabled={saving}
                title="Remove"
                className="shrink-0 rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-red-600 disabled:opacity-50"
              >
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
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={addSecret}
          disabled={saving}
          className="mt-3 inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-50"
        >
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
          Add Secret
        </button>
      </div>

      {/* Section 4: Platform Map */}
      <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-gray-500">
          Platform Map
        </h3>
        {(config.platform_map ?? []).length === 0 && (
          <p className="mb-3 text-sm text-gray-400">
            No platforms configured.
          </p>
        )}
        <div className="space-y-4">
          {(config.platform_map ?? []).map((plat, idx) => (
            <div
              key={idx}
              className="rounded-md border border-gray-200 bg-gray-50 p-4"
            >
              <div className="mb-3 flex items-center justify-between">
                <span className="text-sm font-medium text-gray-700">
                  Platform {idx + 1}
                </span>
                <button
                  type="button"
                  onClick={() => removePlatform(idx)}
                  disabled={saving}
                  title="Remove platform"
                  className="rounded p-1 text-gray-400 hover:bg-gray-200 hover:text-red-600 disabled:opacity-50"
                >
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
                </button>
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-600">
                    Kitchen Name
                  </label>
                  <input
                    type="text"
                    value={plat.kitchen_name}
                    onChange={(e) =>
                      updatePlatform(idx, { kitchen_name: e.target.value })
                    }
                    placeholder="e.g. centos-7"
                    disabled={saving}
                    className={INPUT_CLASS}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-600">
                    Image
                  </label>
                  <input
                    type="text"
                    value={plat.image}
                    onChange={(e) =>
                      updatePlatform(idx, { image: e.target.value })
                    }
                    placeholder="e.g. centos-7-template"
                    disabled={saving}
                    className={INPUT_CLASS}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-gray-600">
                    Transport User
                  </label>
                  <input
                    type="text"
                    value={plat.transport?.username ?? ""}
                    onChange={(e) =>
                      updateTransport(idx, { username: e.target.value })
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
                    value={plat.transport?.password_credential ?? ""}
                    onChange={(e) =>
                      updateTransport(idx, {
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
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium text-gray-600">
                    SSH Key Credential
                  </label>
                  <select
                    value={plat.transport?.ssh_key_credential ?? ""}
                    onChange={(e) =>
                      updateTransport(idx, {
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

              {/* Per-platform driver settings (JSON textarea) */}
              <div className="mt-3">
                <button
                  type="button"
                  onClick={() => togglePlatformAdvanced(idx)}
                  className="text-xs font-medium text-blue-600 hover:text-blue-800"
                >
                  {platformAdvanced[idx]
                    ? "\u25be Hide Advanced"
                    : "\u25b8 Show Advanced (per-platform driver settings)"}
                </button>
                {platformAdvanced[idx] && (
                  <div className="mt-2">
                    <label className="mb-1 block text-xs font-medium text-gray-600">
                      Driver Settings (JSON)
                    </label>
                    <textarea
                      value={platformDriverJson[idx] ?? "{}"}
                      onChange={(e) =>
                        updatePlatformDriverJson(idx, e.target.value)
                      }
                      rows={4}
                      disabled={saving}
                      className={`${INPUT_CLASS} font-mono text-xs`}
                    />
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={addPlatform}
          disabled={saving}
          className="mt-3 inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-50"
        >
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
          Add Platform
        </button>
      </div>

      {/* Footer: Save + Revert */}
      <div className="flex items-center justify-between border-t border-gray-200 pt-4">
        <button
          type="button"
          onClick={handleRevert}
          disabled={saving || source === "file"}
          title={
            source === "file"
              ? "Already using file config"
              : "Delete database config and revert to file"
          }
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-50"
        >
          Revert to File Config
        </button>
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
        >
          {saving ? "Saving\u2026" : "Save Configuration"}
        </button>
      </div>
    </div>
  );
}
