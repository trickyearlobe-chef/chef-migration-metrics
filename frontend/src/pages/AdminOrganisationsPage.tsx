import { useCallback, useEffect, useState } from "react";
import {
  fetchConfigOrganisations,
  saveConfigOrganisations,
  fetchCredentials,
  type Organisation,
} from "../api";
import { ErrorAlert, InlineSpinner, LoadingSpinner } from "../components/Feedback";
import { chefOrgURLError } from "../lib/chefOrgUrl";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";
const SELECT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50 bg-white";

function OrgCard({
  org,
  index,
  credentials,
  saving,
  onChange,
  onRemove,
}: {
  org: Organisation;
  index: number;
  credentials: string[];
  saving: boolean;
  onChange: (index: number, field: keyof Organisation, value: string | boolean | null) => void;
  onRemove: (index: number) => void;
}) {
  const sslValue = org.ssl_verify ?? true;
  // Only surface the format error once something has been typed, so empty new
  // rows don't show a red error before the operator starts.
  const urlError = org.chef_server_url.trim() !== "" ? chefOrgURLError(org.chef_server_url) : null;

  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="flex items-center justify-between border-b border-gray-100 px-4 py-3">
        <h3 className="text-sm font-medium text-gray-900">
          {org.name || "New Organisation"}
        </h3>
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
      <div className="grid grid-cols-2 gap-4 p-4">
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">Name</label>
          <input
            type="text"
            value={org.name}
            onChange={(e) => onChange(index, "name", e.target.value)}
            placeholder="production"
            className={INPUT_CLASS}
            disabled={saving}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">Chef Server URL</label>
          <input
            type="url"
            value={org.chef_server_url}
            onChange={(e) => onChange(index, "chef_server_url", e.target.value)}
            placeholder="https://chef.example.com/organizations/myorg"
            className={INPUT_CLASS}
            aria-invalid={urlError !== null}
            disabled={saving}
          />
          {urlError ? (
            <p className="mt-1 text-xs text-red-600">{urlError}</p>
          ) : (
            <p className="mt-1 text-xs text-gray-500">
              Full organisation URL — the org name is taken from the <code>/organizations/&lt;org&gt;</code> path.
            </p>
          )}
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">Client Name</label>
          <input
            type="text"
            value={org.client_name}
            onChange={(e) => onChange(index, "client_name", e.target.value)}
            placeholder="migration-user"
            className={INPUT_CLASS}
            disabled={saving}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">Credential</label>
          <select
            value={org.client_key_credential}
            onChange={(e) => onChange(index, "client_key_credential", e.target.value)}
            className={SELECT_CLASS}
            disabled={saving}
          >
            <option value="">— none —</option>
            {credentials.map((name) => (
              <option key={name} value={name}>{name}</option>
            ))}
          </select>
        </div>
        <div className="flex items-center pt-5">
          <label className="flex cursor-pointer items-center gap-3">
            <div
              className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-within:ring-2 focus-within:ring-blue-500 focus-within:ring-offset-2 ${sslValue ? "bg-blue-600" : "bg-gray-200"}`}
            >
              <span
                className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition-transform ${sslValue ? "translate-x-4" : "translate-x-0"}`}
              />
              <input
                type="checkbox"
                className="sr-only"
                checked={sslValue}
                onChange={(e) => onChange(index, "ssl_verify", e.target.checked)}
                disabled={saving}
              />
            </div>
            <span className="text-sm text-gray-700">SSL Verify</span>
          </label>
        </div>
      </div>
    </div>
  );
}

export function AdminOrganisationsPage() {
  const [orgs, setOrgs] = useState<Organisation[]>([]);
  const [saved, setSaved] = useState<Organisation[]>([]);
  const [credentials, setCredentials] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    Promise.all([fetchConfigOrganisations(), fetchCredentials()])
      .then(([orgsData, credsData]) => {
        if (cancelled) return;
        setOrgs(orgsData ?? []);
        setSaved(orgsData ?? []);
        setCredentials((credsData.data ?? []).map((c) => c.name));
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(err instanceof Error ? err.message : "Failed to load organisations.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => load(), [load]);

  const isDirty = JSON.stringify(orgs) !== JSON.stringify(saved);
  // Block save until every org has a valid full org URL (the UI validates the
  // shape before the backend ever sees it).
  const allOrgsValid = orgs.every((o) => chefOrgURLError(o.chef_server_url) === null);

  function handleOrgChange(index: number, field: keyof Organisation, value: string | boolean | null) {
    setOrgs((prev) => prev.map((o, i) => i === index ? { ...o, [field]: value } : o));
    setSuccess(false);
  }

  function handleRemove(index: number) {
    setOrgs((prev) => prev.filter((_, i) => i !== index));
    setSuccess(false);
  }

  function handleAdd() {
    setOrgs((prev) => [
      ...prev,
      { name: "", chef_server_url: "", org_name: "", client_name: "", client_key_path: "", client_key_credential: "", ssl_verify: null },
    ]);
    setSuccess(false);
  }

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    setSuccess(false);
    try {
      const { value: updated } = await saveConfigOrganisations(orgs);
      setOrgs(updated ?? orgs);
      setSaved(updated ?? orgs);
      setSuccess(true);
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : "Failed to save organisations.");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <LoadingSpinner message="Loading organisations…" />;
  if (loadError)
    return <ErrorAlert message="Failed to load organisations" detail={loadError} onRetry={load} />;

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">Organisations</h2>
        <p className="mt-1 text-sm text-gray-500">
          Chef Infra Server organisations to collect data from. Each organisation requires a
          credential (PEM key) managed on the Credentials page.
        </p>
      </div>

      {orgs.length === 0 ? (
        <div className="rounded-lg border border-gray-200 bg-white px-4 py-10 text-center text-sm text-gray-400 shadow-sm">
          No organisations configured. Add one below.
        </div>
      ) : (
        <div className="space-y-4">
          {orgs.map((org, i) => (
            <OrgCard
              key={i}
              org={org}
              index={i}
              credentials={credentials}
              saving={saving}
              onChange={handleOrgChange}
              onRemove={handleRemove}
            />
          ))}
        </div>
      )}

      <button
        type="button"
        onClick={handleAdd}
        disabled={saving}
        className="flex items-center gap-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 disabled:opacity-40"
      >
        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
        </svg>
        Add Organisation
      </button>

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
          disabled={saving || !isDirty || !allOrgsValid}
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
        >
          {saving && <InlineSpinner />}
          {saving ? "Saving…" : "Save"}
        </button>
      </div>
    </div>
  );
}
