import { useCallback, useEffect, useState } from "react";
import { fetchAuthConfig, saveAuthConfig, type AuthConfig, type AuthProvider } from "../api";
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

function ProviderCard({
  provider,
  index,
  saving,
  onChange,
  onRemove,
}: {
  provider: AuthProvider;
  index: number;
  saving: boolean;
  onChange: (index: number, field: keyof AuthProvider, value: string | number | undefined) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="flex items-center justify-between border-b border-gray-100 px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-gray-900">
            {provider.type === "local" ? "Local" : provider.type === "ldap" ? "LDAP" : "SAML"} Provider
          </span>
          <span className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
            {provider.type}
          </span>
        </div>
        <button
          type="button"
          onClick={() => onRemove(index)}
          disabled={saving}
          title="Remove"
          className="shrink-0 rounded p-1.5 text-gray-400 hover:bg-red-50 hover:text-red-500 disabled:opacity-40"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <div className="space-y-4 p-4">
        <FieldRow label="Provider Type">
          <select
            value={provider.type}
            onChange={(e) => onChange(index, "type", e.target.value)}
            className={SELECT_CLASS}
            disabled={saving}
          >
            <option value="local">Local</option>
            <option value="ldap">LDAP</option>
            <option value="saml">SAML</option>
          </select>
        </FieldRow>

        {provider.type === "local" && (
          <p className="text-sm text-gray-500">Local username/password authentication.</p>
        )}

        {provider.type === "ldap" && (
          <div className="grid grid-cols-2 gap-4">
            <FieldRow label="Host">
              <input
                type="text"
                value={provider.host ?? ""}
                onChange={(e) => onChange(index, "host", e.target.value)}
                placeholder="ldap.example.com"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="Port">
              <input
                type="number"
                min={1}
                max={65535}
                value={provider.port ?? 389}
                onChange={(e) => onChange(index, "port", Number(e.target.value))}
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <div className="col-span-2">
              <FieldRow label="Base DN" hint="e.g. dc=example,dc=com">
                <input
                  type="text"
                  value={provider.base_dn ?? ""}
                  onChange={(e) => onChange(index, "base_dn", e.target.value)}
                  placeholder="dc=example,dc=com"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
            </div>
            <div className="col-span-2">
              <FieldRow label="Bind DN" hint="Leave empty for anonymous bind">
                <input
                  type="text"
                  value={provider.bind_dn ?? ""}
                  onChange={(e) => onChange(index, "bind_dn", e.target.value)}
                  placeholder="cn=admin,dc=example,dc=com"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
            </div>
            <FieldRow label="Bind Password Env" hint="Environment variable containing the bind password">
              <input
                type="text"
                value={provider.bind_password_env ?? ""}
                onChange={(e) => onChange(index, "bind_password_env", e.target.value)}
                placeholder="LDAP_BIND_PASSWORD"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="Bind Password Credential" hint="Or reference a stored credential">
              <input
                type="text"
                value={provider.bind_password_credential ?? ""}
                onChange={(e) => onChange(index, "bind_password_credential", e.target.value)}
                placeholder="ldap-bind-password"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
          </div>
        )}

        {provider.type === "saml" && (
          <div className="space-y-4">
            <FieldRow label="IDP Metadata URL" hint="URL to the IdP SAML metadata XML">
              <input
                type="url"
                value={provider.idp_metadata_url ?? ""}
                onChange={(e) => onChange(index, "idp_metadata_url", e.target.value)}
                placeholder="https://idp.example.com/metadata.xml"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="SP Entity ID" hint="Service Provider entity ID">
              <input
                type="text"
                value={provider.sp_entity_id ?? ""}
                onChange={(e) => onChange(index, "sp_entity_id", e.target.value)}
                placeholder="https://app.example.com"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
          </div>
        )}
      </div>
    </div>
  );
}

export function AdminAuthPage() {
  const [config, setConfig] = useState<AuthConfig | null>(null);
  const [saved, setSaved] = useState<AuthConfig | null>(null);
  const [restartRequired, setRestartRequired] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    fetchAuthConfig()
      .then((data) => {
        if (cancelled) return;
        setConfig(data);
        setSaved(data);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(err instanceof Error ? err.message : "Failed to load auth config.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => load(), [load]);

  const isDirty = JSON.stringify(config) !== JSON.stringify(saved);

  function setTopField<K extends keyof AuthConfig>(field: K, value: AuthConfig[K]) {
    setConfig((prev) => prev ? { ...prev, [field]: value } : prev);
    setSuccess(false);
  }

  function handleProviderChange(index: number, field: keyof AuthProvider, value: string | number | undefined) {
    setConfig((prev) => {
      if (!prev) return prev;
      const updated = prev.providers.map((p, i) => i === index ? { ...p, [field]: value } : p);
      return { ...prev, providers: updated };
    });
    setSuccess(false);
  }

  function handleProviderRemove(index: number) {
    setConfig((prev) => {
      if (!prev) return prev;
      return { ...prev, providers: prev.providers.filter((_, i) => i !== index) };
    });
    setSuccess(false);
  }

  function handleProviderAdd() {
    setConfig((prev) => {
      if (!prev) return prev;
      return { ...prev, providers: [...prev.providers, { type: "local" }] };
    });
    setSuccess(false);
  }

  async function handleSave() {
    if (!config) return;
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    try {
      const { value: updated } = await saveAuthConfig(config);
      setConfig(updated ?? config);
      setSaved(updated ?? config);
      setSuccess(true);
      setRestartRequired(true);
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : "Failed to save auth config.");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading auth config…" />;
  if (loadError)
    return <ErrorAlert message="Failed to load auth config" detail={loadError} onRetry={load} />;
  if (!config) return null;

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Authentication</h2>
        <p className="mt-1 text-sm text-gray-500">
          Authentication providers and session policy. Changes require an application restart.
        </p>
      </div>

      {/* Auth Providers */}
      <SectionCard title="Auth Providers">
        {config.providers.length === 0 ? (
          <p className="text-center text-sm text-gray-400 py-4">
            No providers configured. Add one below.
          </p>
        ) : (
          <div className="space-y-4">
            {config.providers.map((provider, i) => (
              <ProviderCard
                key={i}
                provider={provider}
                index={i}
                saving={saving}
                onChange={handleProviderChange}
                onRemove={handleProviderRemove}
              />
            ))}
          </div>
        )}
        <button
          type="button"
          onClick={handleProviderAdd}
          disabled={saving}
          className="flex items-center gap-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 disabled:opacity-40"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          Add Provider
        </button>
      </SectionCard>

      {/* Policy */}
      <SectionCard title="Policy">
        <div className="grid grid-cols-2 gap-4">
          <div className="col-span-2">
            <FieldRow label="Session Expiry" hint="Duration string e.g. 24h, 7d (empty = default 24h)">
              <input
                type="text"
                value={config.session_expiry}
                onChange={(e) => setTopField("session_expiry", e.target.value)}
                placeholder="24h"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
          </div>
          <FieldRow label="Minimum Password Length" hint="0 = use system default (8)">
            <input
              type="number"
              min={0}
              value={config.min_password_length}
              onChange={(e) => setTopField("min_password_length", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </FieldRow>
          <FieldRow label="Lockout Attempts" hint="Number of failed attempts before lockout (0 = disabled)">
            <input
              type="number"
              min={0}
              value={config.lockout_attempts}
              onChange={(e) => setTopField("lockout_attempts", Number(e.target.value))}
              className={INPUT_CLASS}
              disabled={saving}
            />
          </FieldRow>
        </div>
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
