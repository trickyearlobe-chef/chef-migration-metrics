import { useCallback, useEffect, useState } from "react";
import {
  fetchServerConfig,
  saveServerConfig,
  type ServerConfig,
} from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";
const SELECT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50 bg-white";

function SectionCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-100 px-4 py-3">
        <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
      </div>
      <div className="space-y-4 p-4">{children}</div>
    </div>
  );
}

function FieldRow({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-gray-700">{label}</label>
      {children}
      {hint && <p className="mt-1 text-xs text-gray-400">{hint}</p>}
    </div>
  );
}

export function AdminServerPage() {
  const [config, setConfig] = useState<ServerConfig | null>(null);
  const [saved, setSaved] = useState<ServerConfig | null>(null);
  const [restartRequired, setRestartRequired] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [newDomain, setNewDomain] = useState("");

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchServerConfig()
      .then((data) => {
        if (cancelled) return;
        setConfig(data);
        setSaved(data);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(err instanceof Error ? err.message : "Failed to load server config.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => load(), [load]);

  const isDirty = JSON.stringify(config) !== JSON.stringify(saved);

  function setField<K extends keyof ServerConfig>(field: K, value: ServerConfig[K]) {
    setConfig((prev) => prev ? { ...prev, [field]: value } : prev);
    setSuccess(false);
  }

  function setTlsField<K extends keyof ServerConfig["tls"]>(field: K, value: ServerConfig["tls"][K]) {
    setConfig((prev) => prev ? { ...prev, tls: { ...prev.tls, [field]: value } } : prev);
    setSuccess(false);
  }

  function setAcmeField<K extends keyof ServerConfig["tls"]["acme"]>(field: K, value: ServerConfig["tls"]["acme"][K]) {
    setConfig((prev) =>
      prev ? { ...prev, tls: { ...prev.tls, acme: { ...prev.tls.acme, [field]: value } } } : prev
    );
    setSuccess(false);
  }

  function setWsField<K extends keyof ServerConfig["websocket"]>(field: K, value: ServerConfig["websocket"][K]) {
    setConfig((prev) => prev ? { ...prev, websocket: { ...prev.websocket, [field]: value } } : prev);
    setSuccess(false);
  }

  function handleAddDomain() {
    const d = newDomain.trim();
    if (!d) return;
    setAcmeField("domains", [...(config?.tls.acme.domains ?? []), d]);
    setNewDomain("");
  }

  function handleRemoveDomain(i: number) {
    setAcmeField("domains", (config?.tls.acme.domains ?? []).filter((_, idx) => idx !== i));
  }

  async function handleSave() {
    if (!config) return;
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    try {
      const { value: updated } = await saveServerConfig(config);
      setConfig(updated ?? config);
      setSaved(updated ?? config);
      setSuccess(true);
      setRestartRequired(true);
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : "Failed to save server config.");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading server config…" />;
  if (loadError)
    return <ErrorAlert message="Failed to load server config" detail={loadError} onRetry={load} />;
  if (!config) return null;

  const tlsMode = config.tls.mode;
  const wsEnabled = config.websocket.enabled ?? true;

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Server &amp; TLS</h2>
        <p className="mt-1 text-sm text-gray-500">
          HTTP server settings, TLS/HTTPS configuration, and WebSocket settings. Changes require
          an application restart.
        </p>
      </div>

      {/* TLS Settings */}
      <SectionCard title="TLS Settings">
        <FieldRow label="TLS Mode">
          <select
            value={tlsMode}
            onChange={(e) => setTlsField("mode", e.target.value)}
            className={SELECT_CLASS}
            disabled={saving}
          >
            <option value="off">Off</option>
            <option value="static">Static certificate</option>
            <option value="acme">ACME (Let&apos;s Encrypt)</option>
          </select>
        </FieldRow>

        {tlsMode !== "off" && (
          <>
            <FieldRow label="Minimum TLS Version">
              <select
                value={config.tls.min_version}
                onChange={(e) => setTlsField("min_version", e.target.value)}
                className={SELECT_CLASS}
                disabled={saving}
              >
                <option value="">Default (1.2)</option>
                <option value="1.2">TLS 1.2</option>
                <option value="1.3">TLS 1.3</option>
              </select>
            </FieldRow>
            <FieldRow label="HTTP Redirect Port" hint="Port for HTTP→HTTPS redirect (0 = disabled)">
              <input
                type="number"
                min={0}
                value={config.tls.http_redirect_port}
                onChange={(e) => setTlsField("http_redirect_port", Number(e.target.value))}
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
          </>
        )}

        {tlsMode === "static" && (
          <>
            <FieldRow label="Certificate Path">
              <input
                type="text"
                value={config.tls.cert_path}
                onChange={(e) => setTlsField("cert_path", e.target.value)}
                placeholder="/etc/ssl/certs/server.crt"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="Key Path">
              <input
                type="text"
                value={config.tls.key_path}
                onChange={(e) => setTlsField("key_path", e.target.value)}
                placeholder="/etc/ssl/private/server.key"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="CA Path" hint="Optional CA bundle path">
              <input
                type="text"
                value={config.tls.ca_path}
                onChange={(e) => setTlsField("ca_path", e.target.value)}
                placeholder="/etc/ssl/certs/ca-bundle.crt"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
          </>
        )}

        {tlsMode === "acme" && (
          <>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-700">Domains</label>
              <div className="flex flex-wrap gap-2 mb-2">
                {config.tls.acme.domains.map((d, i) => (
                  <span key={i} className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2.5 py-0.5 text-xs font-medium text-blue-700">
                    {d}
                    <button
                      type="button"
                      onClick={() => handleRemoveDomain(i)}
                      disabled={saving}
                      className="ml-0.5 text-blue-400 hover:text-blue-600 disabled:opacity-40"
                    >
                      ×
                    </button>
                  </span>
                ))}
              </div>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={newDomain}
                  onChange={(e) => setNewDomain(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && handleAddDomain()}
                  placeholder="example.com"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
                <button
                  type="button"
                  onClick={handleAddDomain}
                  disabled={saving || !newDomain.trim()}
                  className="shrink-0 rounded-md bg-gray-100 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200 disabled:opacity-40"
                >
                  Add
                </button>
              </div>
            </div>
            <FieldRow label="Email">
              <input
                type="email"
                value={config.tls.acme.email}
                onChange={(e) => setAcmeField("email", e.target.value)}
                placeholder="admin@example.com"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="ACME Challenge">
              <select
                value={config.tls.acme.challenge}
                onChange={(e) => setAcmeField("challenge", e.target.value)}
                className={SELECT_CLASS}
                disabled={saving}
              >
                <option value="http-01">HTTP-01</option>
                <option value="tls-alpn-01">TLS-ALPN-01</option>
                <option value="dns-01">DNS-01</option>
              </select>
            </FieldRow>
            <FieldRow label="Storage Path" hint="Directory for ACME certificate storage">
              <input
                type="text"
                value={config.tls.acme.storage_path}
                onChange={(e) => setAcmeField("storage_path", e.target.value)}
                placeholder="/var/lib/acme"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="Renew Before (days)" hint="Days before expiry to renew (0 = default: 30)">
              <input
                type="number"
                min={0}
                value={config.tls.acme.renew_before_days}
                onChange={(e) => setAcmeField("renew_before_days", Number(e.target.value))}
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <label className="flex cursor-pointer items-center gap-3">
              <div
                className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-within:ring-2 focus-within:ring-blue-500 focus-within:ring-offset-2 ${config.tls.acme.agree_to_tos ? "bg-blue-600" : "bg-gray-200"}`}
              >
                <span
                  className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition-transform ${config.tls.acme.agree_to_tos ? "translate-x-4" : "translate-x-0"}`}
                />
                <input
                  type="checkbox"
                  className="sr-only"
                  checked={config.tls.acme.agree_to_tos}
                  onChange={(e) => setAcmeField("agree_to_tos", e.target.checked)}
                  disabled={saving}
                />
              </div>
              <span className="text-sm text-gray-700">I agree to the Terms of Service</span>
            </label>
          </>
        )}
      </SectionCard>

      {/* WebSocket */}
      <SectionCard title="WebSocket">
        <label className="flex cursor-pointer items-center gap-3">
          <div
            className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-within:ring-2 focus-within:ring-blue-500 focus-within:ring-offset-2 ${wsEnabled ? "bg-blue-600" : "bg-gray-200"}`}
          >
            <span
              className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition-transform ${wsEnabled ? "translate-x-4" : "translate-x-0"}`}
            />
            <input
              type="checkbox"
              className="sr-only"
              checked={wsEnabled}
              onChange={(e) => setWsField("enabled", e.target.checked)}
              disabled={saving}
            />
          </div>
          <span className="text-sm text-gray-700">Enabled</span>
        </label>
        <div className="grid grid-cols-2 gap-4">
          <FieldRow label="Max Connections" hint="0 = unlimited">
            <input
              type="number"
              min={0}
              value={config.websocket.max_connections}
              onChange={(e) => setWsField("max_connections", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </FieldRow>
          <FieldRow label="Send Buffer Size" hint="0 = default">
            <input
              type="number"
              min={0}
              value={config.websocket.send_buffer_size}
              onChange={(e) => setWsField("send_buffer_size", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </FieldRow>
          <FieldRow label="Write Timeout (seconds)" hint="0 = default">
            <input
              type="number"
              min={0}
              value={config.websocket.write_timeout_seconds}
              onChange={(e) => setWsField("write_timeout_seconds", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </FieldRow>
          <FieldRow label="Ping Interval (seconds)" hint="0 = default">
            <input
              type="number"
              min={0}
              value={config.websocket.ping_interval_seconds}
              onChange={(e) => setWsField("ping_interval_seconds", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </FieldRow>
          <FieldRow label="Pong Timeout (seconds)" hint="0 = default (must be > ping interval)">
            <input
              type="number"
              min={0}
              value={config.websocket.pong_timeout_seconds}
              onChange={(e) => setWsField("pong_timeout_seconds", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </FieldRow>
        </div>
      </SectionCard>

      {/* General */}
      <SectionCard title="General">
        <FieldRow label="Graceful Shutdown (seconds)">
          <input
            type="number"
            min={1}
            value={config.graceful_shutdown_seconds}
            onChange={(e) => setField("graceful_shutdown_seconds", Number(e.target.value))}
            className={INPUT_CLASS}
            disabled={saving}
          />
        </FieldRow>
      </SectionCard>

      {saveError && <ErrorAlert message="Failed to save" detail={saveError} />}

      {success && (
        <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800">
          Settings saved successfully.
        </div>
      )}

      <div className="flex justify-end gap-3">
        {restartRequired && (
          <div className="flex items-center gap-1.5 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs font-medium text-amber-800">
            <svg className="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
            </svg>
            Restart required to apply changes
          </div>
        )}
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
