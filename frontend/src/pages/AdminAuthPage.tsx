import { useCallback, useEffect, useState } from "react";
import {
  fetchAuthConfig,
  fetchSAMLCertificate,
  fetchSAMLEndpoints,
  fetchSAMLMetadata,
  samlMetadataUrl,
  generateSAMLKeypair,
  saveAuthConfig,
  type AuthConfig,
  type AuthProvider,
  type SAMLCertificateResponse,
  type SAMLEndpoints,
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

function RoleMappingEditor({
  mapping,
  disabled,
  onChange,
}: {
  mapping: Record<string, string>;
  disabled: boolean;
  onChange: (m: Record<string, string>) => void;
}) {
  const entries = Object.entries(mapping);

  function handleAdd() {
    onChange({ ...mapping, "": "viewer" });
  }

  function handleRemove(key: string) {
    const next = { ...mapping };
    delete next[key];
    onChange(next);
  }

  function handleKeyChange(oldKey: string, newKey: string) {
    const next: Record<string, string> = {};
    for (const [k, v] of Object.entries(mapping)) {
      next[k === oldKey ? newKey : k] = v;
    }
    onChange(next);
  }

  function handleValueChange(key: string, value: string) {
    onChange({ ...mapping, [key]: value });
  }

  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-gray-700">Role Mapping</label>
      <p className="mb-2 text-xs text-gray-400">
        Map IdP group names to app roles. Group name must match exactly what the IdP sends (often email address for Google).
      </p>
      <div className="space-y-2">
        {entries.map(([key, value], i) => (
          <div key={i} className="flex items-center gap-2">
            <input
              type="text"
              value={key}
              onChange={(e) => handleKeyChange(key, e.target.value)}
              placeholder="group-name@domain.com"
              className="flex-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50"
              disabled={disabled}
            />
            <span className="text-xs text-gray-400">→</span>
            <select
              value={value}
              onChange={(e) => handleValueChange(key, e.target.value)}
              className="rounded-md border border-gray-300 px-2 py-1.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50 bg-white"
              disabled={disabled}
            >
              <option value="viewer">viewer</option>
              <option value="operator">operator</option>
              <option value="admin">admin</option>
            </select>
            <button
              type="button"
              onClick={() => handleRemove(key)}
              disabled={disabled}
              className="rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-500 disabled:opacity-40"
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={handleAdd}
        disabled={disabled}
        className="mt-2 flex items-center gap-1 text-xs font-medium text-blue-600 hover:text-blue-700 disabled:opacity-40"
      >
        <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
        </svg>
        Add Mapping
      </button>
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
  onChange: (index: number, field: keyof AuthProvider, value: string | number | boolean | Record<string, string> | undefined) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="flex items-center justify-between border-b border-gray-100 px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-gray-900">
            {provider.type === "local" ? "Local" : "SAML"} Provider
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
            <option value="saml">SAML</option>
          </select>
        </FieldRow>

        {provider.type === "local" && (
          <p className="text-sm text-gray-500">Local username/password authentication.</p>
        )}

        {provider.type === "saml" && (
          <div className="space-y-4">
            <FieldRow label="IDP Metadata URL" hint="URL to the IdP SAML metadata XML (use this OR path below)">
              <input
                type="url"
                value={provider.idp_metadata_url ?? ""}
                onChange={(e) => onChange(index, "idp_metadata_url", e.target.value)}
                placeholder="https://idp.example.com/metadata.xml"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="IDP Metadata Path" hint="Local file path to IdP metadata XML (e.g. for Google Workspace)">
              <input
                type="text"
                value={provider.idp_metadata_path ?? ""}
                onChange={(e) => onChange(index, "idp_metadata_path", e.target.value)}
                placeholder="./GoogleIDPMetadata.xml"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <FieldRow label="SP Entity ID" hint="Service Provider entity ID (e.g. https://app.example.com/saml)">
              <input
                type="text"
                value={provider.sp_entity_id ?? ""}
                onChange={(e) => onChange(index, "sp_entity_id", e.target.value)}
                placeholder="https://app.example.com/saml"
                className={INPUT_CLASS}
                disabled={saving}
              />
            </FieldRow>
            <div className="grid grid-cols-2 gap-4">
              <FieldRow label="SP Certificate Credential" hint="Credential store name for SP cert">
                <input
                  type="text"
                  value={provider.sp_certificate_credential ?? ""}
                  onChange={(e) => onChange(index, "sp_certificate_credential", e.target.value)}
                  placeholder="saml-sp-cert"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
              <FieldRow label="SP Private Key Credential" hint="Credential store name for SP key">
                <input
                  type="text"
                  value={provider.sp_private_key_credential ?? ""}
                  onChange={(e) => onChange(index, "sp_private_key_credential", e.target.value)}
                  placeholder="saml-sp-key"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <FieldRow label="Username Attribute" hint="SAML attribute for username">
                <input
                  type="text"
                  value={provider.username_attr ?? ""}
                  onChange={(e) => onChange(index, "username_attr", e.target.value)}
                  placeholder="email"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
              <FieldRow label="Email Attribute" hint="SAML attribute for email address">
                <input
                  type="text"
                  value={provider.email_attr ?? ""}
                  onChange={(e) => onChange(index, "email_attr", e.target.value)}
                  placeholder="email"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
              <FieldRow label="Display Name Attribute">
                <input
                  type="text"
                  value={provider.display_name_attr ?? ""}
                  onChange={(e) => onChange(index, "display_name_attr", e.target.value)}
                  placeholder="displayName"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
              <FieldRow label="Groups Attribute">
                <input
                  type="text"
                  value={provider.groups_attr ?? ""}
                  onChange={(e) => onChange(index, "groups_attr", e.target.value)}
                  placeholder="groups"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
              <FieldRow label="Role Attribute" hint="Direct role attribute (admin/operator/viewer). Overrides group mapping.">
                <input
                  type="text"
                  value={provider.role_attr ?? ""}
                  onChange={(e) => onChange(index, "role_attr", e.target.value)}
                  placeholder="role"
                  className={INPUT_CLASS}
                  disabled={saving}
                />
              </FieldRow>
            </div>
            <RoleMappingEditor
              mapping={provider.role_mapping ?? {}}
              disabled={saving}
              onChange={(m) => onChange(index, "role_mapping", m)}
            />

            <div className="space-y-2 border-t border-gray-100 pt-4">
              <label className="flex items-start gap-2 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={provider.sign_requests ?? false}
                  onChange={(e) => onChange(index, "sign_requests", e.target.checked)}
                  disabled={saving}
                  className="mt-0.5 h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50"
                />
                <span>
                  Sign AuthnRequests
                  <span className="block text-xs text-gray-400">
                    Sign outgoing SSO requests with the SP key (RSA-SHA256) and advertise
                    <code className="mx-1">AuthnRequestsSigned</code>in metadata. The IdP
                    validates against the SP signing certificate.
                  </span>
                </span>
              </label>
              <label className="flex items-start gap-2 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={provider.allow_idp_initiated ?? false}
                  onChange={(e) => onChange(index, "allow_idp_initiated", e.target.checked)}
                  disabled={saving}
                  className="mt-0.5 h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50"
                />
                <span>
                  Allow IdP-initiated SSO
                  <span className="block text-xs text-gray-400">
                    Accept unsolicited assertions (no matching AuthnRequest). Leave off
                    unless your IdP requires it — it weakens replay protection.
                  </span>
                </span>
              </label>
            </div>
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

  // SP Certificate state.
  const [spCert, setSpCert] = useState<SAMLCertificateResponse | null>(null);
  const [generatingCert, setGeneratingCert] = useState(false);
  const [certError, setCertError] = useState<string | null>(null);
  const [certCopied, setCertCopied] = useState(false);

  // SP metadata export state.
  const [exportingMetadata, setExportingMetadata] = useState(false);
  const [metadataError, setMetadataError] = useState<string | null>(null);
  const [metadataUrlCopied, setMetadataUrlCopied] = useState(false);

  // SP endpoint URLs (backend-computed) + which field was last copied.
  const [samlEndpoints, setSamlEndpoints] = useState<SAMLEndpoints | null>(null);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    Promise.all([fetchAuthConfig(), fetchSAMLCertificate(), fetchSAMLEndpoints()])
      .then(([data, cert, endpoints]) => {
        if (cancelled) return;
        setConfig(data);
        setSaved(data);
        setSpCert(cert);
        setSamlEndpoints(endpoints);
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

  function handleProviderChange(index: number, field: keyof AuthProvider, value: string | number | boolean | Record<string, string> | undefined) {
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
    // Apply defaults for SAML credential names before sending.
    const toSave = {
      ...config,
      providers: config.providers.map((p) =>
        p.type === "saml"
          ? {
              ...p,
              sp_certificate_credential: p.sp_certificate_credential || "saml-sp-cert",
              sp_private_key_credential: p.sp_private_key_credential || "saml-sp-key",
            }
          : p,
      ),
    };
    try {
      const { value: updated } = await saveAuthConfig(toSave);
      setConfig(updated ?? toSave);
      setSaved(updated ?? toSave);
      setSuccess(true);
      setRestartRequired(true);
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : "Failed to save auth config.");
    } finally {
      setSaving(false);
    }
  }

  async function handleGenerateCert() {
    setGeneratingCert(true);
    setCertError(null);
    setCertCopied(false);
    try {
      const cert = await generateSAMLKeypair();
      setSpCert(cert);
    } catch (err: unknown) {
      setCertError(err instanceof Error ? err.message : "Failed to generate keypair.");
    } finally {
      setGeneratingCert(false);
    }
  }

  async function handleExportMetadata() {
    setExportingMetadata(true);
    setMetadataError(null);
    try {
      // Download the live SP metadata exactly as the endpoint emits it —
      // standard SAML 2.0 EntityDescriptor XML the IdP can ingest directly.
      const xml = await fetchSAMLMetadata();
      const blob = new Blob([xml], { type: "application/samlmetadata+xml" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "sp-metadata.xml";
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err: unknown) {
      setMetadataError(err instanceof Error ? err.message : "Failed to export SP metadata.");
    } finally {
      setExportingMetadata(false);
    }
  }

  function handleCopyMetadataUrl() {
    navigator.clipboard.writeText(samlMetadataUrl()).then(() => {
      setMetadataUrlCopied(true);
      setTimeout(() => setMetadataUrlCopied(false), 2000);
    });
  }

  function handleCopyField(key: string, value: string) {
    navigator.clipboard.writeText(value).then(() => {
      setCopiedField(key);
      setTimeout(() => setCopiedField((k) => (k === key ? null : k)), 2000);
    });
  }

  function handleCopyCert() {
    if (!spCert) return;
    navigator.clipboard.writeText(spCert.certificate_pem).then(() => {
      setCertCopied(true);
      setTimeout(() => setCertCopied(false), 2000);
    });
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

      {/* SP Certificate — shown when any SAML provider exists */}
      {config.providers.some((p) => p.type === "saml") && (
        <SectionCard title="SAML SP Certificate">
          <p className="text-sm text-gray-500">
            Generate a signing certificate for the Service Provider. Copy the certificate PEM and
            paste it into your Identity Provider&apos;s SAML app configuration.
          </p>

          {spCert && (
            <div className="space-y-2">
              <div className="flex items-center gap-3 text-xs text-gray-500">
                <span>Fingerprint: <code className="font-mono">{spCert.fingerprint_sha256.slice(0, 16)}…</code></span>
                <span>Expires: {new Date(spCert.not_after).toLocaleDateString()}</span>
                {spCert.subject && <span>CN: {spCert.subject}</span>}
              </div>
              <div className="relative">
                <textarea
                  readOnly
                  value={spCert.certificate_pem}
                  rows={6}
                  className="block w-full rounded-md border border-gray-300 bg-gray-50 px-3 py-2 font-mono text-xs text-gray-700 focus:outline-none"
                  onClick={(e) => (e.target as HTMLTextAreaElement).select()}
                />
                <button
                  type="button"
                  onClick={handleCopyCert}
                  className="absolute right-2 top-2 rounded bg-white px-2 py-1 text-xs font-medium text-gray-600 shadow-sm border border-gray-200 hover:bg-gray-50"
                >
                  {certCopied ? "Copied!" : "Copy"}
                </button>
              </div>
            </div>
          )}

          {certError && <ErrorAlert message="Certificate generation failed" detail={certError} />}

          <button
            type="button"
            onClick={handleGenerateCert}
            disabled={generatingCert}
            className="inline-flex items-center gap-2 rounded-md bg-emerald-600 px-3 py-2 text-sm font-medium text-white shadow-sm hover:bg-emerald-700 disabled:opacity-50"
          >
            {generatingCert && <InlineSpinner />}
            {spCert ? "Regenerate SP Certificate" : "Generate SP Certificate"}
          </button>

          {spCert && (
            <p className="text-xs text-amber-600">
              ⚠️ Regenerating will invalidate the current certificate. You will need to update
              the certificate in your Identity Provider.
            </p>
          )}

          <div className="space-y-3 border-t border-gray-100 pt-4">
            <p className="text-sm text-gray-500">
              Provide this Service Provider&apos;s metadata to your Identity Provider. Some
              IdPs (ADFS, Shibboleth, Keycloak, PingFederate) fetch it from a URL and
              refresh automatically; others (Google, Okta) need the XML file uploaded.
            </p>

            <FieldRow
              label="SP Metadata URL"
              hint="Public endpoint — paste into IdPs that fetch metadata by URL."
            >
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  readOnly
                  value={samlMetadataUrl()}
                  onClick={(e) => (e.target as HTMLInputElement).select()}
                  className="block w-full rounded-md border border-gray-300 bg-gray-50 px-3 py-2 font-mono text-xs text-gray-700 focus:outline-none"
                />
                <button
                  type="button"
                  onClick={handleCopyMetadataUrl}
                  className="shrink-0 rounded-md border border-gray-200 bg-white px-2 py-2 text-xs font-medium text-gray-600 shadow-sm hover:bg-gray-50"
                >
                  {metadataUrlCopied ? "Copied!" : "Copy URL"}
                </button>
              </div>
            </FieldRow>

            {samlEndpoints && (
              <div className="space-y-3">
                <p className="text-sm text-gray-500">
                  IdPs configured by hand (Google, Okta) need these pasted in directly.
                  The <span className="font-medium">callback (ACS) URL</span> is where the
                  IdP must POST its SAML response — these are the live values, not a guess.
                </p>
                {[
                  {
                    key: "acs",
                    label: "Callback (ACS) URL",
                    hint: "The IdP's reply / assertion-consumer URL — POST target for the SAML response.",
                    value: samlEndpoints.acs_url,
                  },
                  {
                    key: "slo",
                    label: "Single Logout (SLO) URL",
                    hint: "Where the IdP sends LogoutRequests.",
                    value: samlEndpoints.slo_url,
                  },
                  {
                    key: "entity",
                    label: "SP Entity ID",
                    hint: "This service provider's entity ID (audience).",
                    value: samlEndpoints.entity_id,
                  },
                ].map((f) => (
                  <FieldRow key={f.key} label={f.label} hint={f.hint}>
                    <div className="flex items-center gap-2">
                      <input
                        type="text"
                        readOnly
                        value={f.value}
                        onClick={(e) => (e.target as HTMLInputElement).select()}
                        className="block w-full rounded-md border border-gray-300 bg-gray-50 px-3 py-2 font-mono text-xs text-gray-700 focus:outline-none"
                      />
                      <button
                        type="button"
                        onClick={() => handleCopyField(f.key, f.value)}
                        className="shrink-0 rounded-md border border-gray-200 bg-white px-2 py-2 text-xs font-medium text-gray-600 shadow-sm hover:bg-gray-50"
                      >
                        {copiedField === f.key ? "Copied!" : "Copy"}
                      </button>
                    </div>
                  </FieldRow>
                ))}
              </div>
            )}

            {metadataError && (
              <ErrorAlert message="Failed to export SP metadata" detail={metadataError} />
            )}
            <button
              type="button"
              onClick={handleExportMetadata}
              disabled={exportingMetadata}
              className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 disabled:opacity-50"
            >
              {exportingMetadata && <InlineSpinner />}
              Export SP Metadata (XML)
            </button>
          </div>
        </SectionCard>
      )}

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
