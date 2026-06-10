// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import {
  fetchCredentials,
  fetchConfigOrganisations,
  saveConfigOrganisations,
  type Organisation,
} from "../api";
import { ErrorAlert, InlineSpinner } from "../components/Feedback";
import { chefOrgURLError } from "../lib/chefOrgUrl";

const INPUT_CLASS =
  "block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

const SELECT_CLASS =
  "block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50";

type Step = "welcome" | "credentials" | "organisation" | "done";

function StepIndicator({ current }: { current: Step }) {
  const steps: { id: Step; label: string }[] = [
    { id: "welcome", label: "Welcome" },
    { id: "credentials", label: "Credentials" },
    { id: "organisation", label: "Organisation" },
    { id: "done", label: "Done" },
  ];
  const currentIndex = steps.findIndex((s) => s.id === current);

  return (
    <nav aria-label="Setup progress" className="flex items-center justify-center gap-0">
      {steps.map((step, i) => {
        const isDone = i < currentIndex;
        const isActive = i === currentIndex;
        return (
          <div key={step.id} className="flex items-center">
            <div className="flex flex-col items-center">
              <div
                className={`flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold ${
                  isDone
                    ? "bg-blue-600 text-white"
                    : isActive
                      ? "border-2 border-blue-600 bg-white text-blue-600"
                      : "border-2 border-gray-300 bg-white text-gray-400"
                }`}
              >
                {isDone ? (
                  <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="m4.5 12.75 6 6 9-13.5" />
                  </svg>
                ) : (
                  i + 1
                )}
              </div>
              <span className={`mt-1 text-xs ${isActive ? "font-medium text-blue-600" : "text-gray-400"}`}>
                {step.label}
              </span>
            </div>
            {i < steps.length - 1 && (
              <div className={`mb-4 h-0.5 w-16 ${i < currentIndex ? "bg-blue-600" : "bg-gray-200"}`} />
            )}
          </div>
        );
      })}
    </nav>
  );
}

export function AdminSetupWizardPage() {
  const [step, setStep] = useState<Step>("welcome");
  const [credentials, setCredentials] = useState<string[]>([]);
  const [org, setOrg] = useState<Organisation>({
    name: "",
    chef_server_url: "",
    org_name: "",
    client_name: "pivotal",
    client_key_path: "",
    client_key_credential: "",
    ssl_verify: null,
  });
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const loadCredentials = useCallback(() => {
    fetchCredentials()
      .then((res) => setCredentials(res.data.map((c) => c.name)))
      .catch(() => {
        /* credentials are optional at this step */
      });
  }, []);

  useEffect(() => {
    if (step === "organisation") loadCredentials();
  }, [step, loadCredentials]);

  function handleOrgChange<K extends keyof Organisation>(field: K, value: Organisation[K]) {
    setOrg((prev) => ({ ...prev, [field]: value }));
    setSaveError(null);
  }

  async function handleSaveOrg() {
    setSaving(true);
    setSaveError(null);
    try {
      await saveConfigOrganisations([org]);
      setStep("done");
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : "Failed to save organisation.");
    } finally {
      setSaving(false);
    }
  }

  const orgUrlError = org.chef_server_url.trim() !== "" ? chefOrgURLError(org.chef_server_url) : null;
  const orgValid =
    org.name.trim() !== "" &&
    chefOrgURLError(org.chef_server_url) === null &&
    org.client_name.trim() !== "" &&
    (org.client_key_credential.trim() !== "" || org.client_key_path.trim() !== "");

  return (
    <div className="flex min-h-full flex-col items-center justify-start pt-8">
      <div className="w-full max-w-xl space-y-8">
        {/* Header */}
        <div className="text-center">
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-blue-100">
            <svg className="h-8 w-8 text-blue-600" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
            </svg>
          </div>
          <h1 className="mt-3 text-2xl font-bold text-gray-900">Initial Setup</h1>
          <p className="mt-1 text-sm text-gray-500">Configure Chef Migration Metrics for the first time.</p>
        </div>

        <StepIndicator current={step} />

        {/* Step content */}
        <div className="rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
          {step === "welcome" && (
            <div className="space-y-4">
              <h2 className="text-lg font-semibold text-gray-900">Welcome</h2>
              <p className="text-sm text-gray-600">
                No organisations have been configured yet. This wizard will guide you through adding
                your first Chef Infra Server organisation.
              </p>
              <div className="rounded-lg bg-blue-50 p-4 text-sm text-blue-800">
                <p className="font-medium">Before you begin:</p>
                <ul className="mt-2 list-inside list-disc space-y-1">
                  <li>Have your Chef Server URL ready (e.g. <code className="rounded bg-blue-100 px-1">https://chef.example.com</code>)</li>
                  <li>Have your Chef API client key (.pem file)</li>
                  <li>Know your organisation name and client name</li>
                </ul>
              </div>
              <div className="flex justify-end">
                <button
                  type="button"
                  onClick={() => setStep("credentials")}
                  className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700"
                >
                  Get Started
                  <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5 21 12m0 0-7.5 7.5M21 12H3" />
                  </svg>
                </button>
              </div>
            </div>
          )}

          {step === "credentials" && (
            <div className="space-y-4">
              <h2 className="text-lg font-semibold text-gray-900">Store a Chef API Key</h2>
              <p className="text-sm text-gray-600">
                Your Chef API client key (.pem file) needs to be stored as a credential before it can
                be referenced by an organisation.
              </p>
              <div className="rounded-lg bg-amber-50 border border-amber-200 p-4 text-sm text-amber-800">
                Go to <strong>Admin → Credentials</strong> and create a credential of type{" "}
                <code className="rounded bg-amber-100 px-1">chef_client_key</code>, then return here
                and continue.
              </div>
              <p className="text-xs text-gray-500">
                Alternatively, if you prefer to reference a key file on disk, you can skip this step
                and use a file path in the next step.
              </p>
              <div className="flex items-center justify-between">
                <button
                  type="button"
                  onClick={() => setStep("welcome")}
                  className="text-sm text-gray-500 hover:text-gray-700"
                >
                  ← Back
                </button>
                <div className="flex gap-3">
                  <button
                    type="button"
                    onClick={() => window.open("/admin/credentials", "_blank")}
                    className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
                  >
                    Open Credentials
                    <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 6H5.25A2.25 2.25 0 0 0 3 8.25v10.5A2.25 2.25 0 0 0 5.25 21h10.5A2.25 2.25 0 0 0 18 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25" />
                    </svg>
                  </button>
                  <button
                    type="button"
                    onClick={() => setStep("organisation")}
                    className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700"
                  >
                    Continue
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5 21 12m0 0-7.5 7.5M21 12H3" />
                    </svg>
                  </button>
                </div>
              </div>
            </div>
          )}

          {step === "organisation" && (
            <div className="space-y-4">
              <h2 className="text-lg font-semibold text-gray-900">Add Your First Organisation</h2>
              <p className="text-sm text-gray-600">
                Configure the first Chef Infra Server organisation to collect data from.
              </p>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700">Organisation Label <span className="text-red-500">*</span></label>
                  <input type="text" value={org.name} onChange={(e) => handleOrgChange("name", e.target.value)}
                    placeholder="e.g. production" className={INPUT_CLASS} disabled={saving} />
                  <p className="mt-1 text-xs text-gray-500">A short identifier used within the app.</p>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">Chef Server URL <span className="text-red-500">*</span></label>
                  <input type="url" value={org.chef_server_url} onChange={(e) => handleOrgChange("chef_server_url", e.target.value)}
                    placeholder="https://chef.example.com/organizations/myorg" className={INPUT_CLASS}
                    aria-invalid={orgUrlError !== null} disabled={saving} />
                  {orgUrlError ? (
                    <p className="mt-1 text-xs text-red-600">{orgUrlError}</p>
                  ) : (
                    <p className="mt-1 text-xs text-gray-500">Full organisation URL — the org name is taken from the <code>/organizations/&lt;org&gt;</code> path.</p>
                  )}
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">Client Name <span className="text-red-500">*</span></label>
                  <input type="text" value={org.client_name} onChange={(e) => handleOrgChange("client_name", e.target.value)}
                    placeholder="pivotal" className={INPUT_CLASS} disabled={saving} />
                </div>
                <div className="col-span-2">
                  <label className="block text-sm font-medium text-gray-700">Chef API Key Credential <span className="text-red-500">*</span></label>
                  {credentials.length > 0 ? (
                    <select value={org.client_key_credential} onChange={(e) => handleOrgChange("client_key_credential", e.target.value)}
                      className={SELECT_CLASS} disabled={saving}>
                      <option value="">— select credential —</option>
                      {credentials.map((c) => (
                        <option key={c} value={c}>{c}</option>
                      ))}
                    </select>
                  ) : (
                    <div className="space-y-2">
                      <input type="text" value={org.client_key_path}
                        onChange={(e) => handleOrgChange("client_key_path", e.target.value)}
                        placeholder="/etc/chef/client.pem" className={INPUT_CLASS} disabled={saving} />
                      <p className="text-xs text-gray-500">No credentials found. Enter the path to the .pem file on disk, or go back to create a credential first.</p>
                    </div>
                  )}
                </div>
                <div className="col-span-2">
                  <label className="flex cursor-pointer items-center gap-3">
                    <div
                      className={`relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors ${
                        (org.ssl_verify ?? true) ? "bg-blue-600" : "bg-gray-200"
                      }`}
                    >
                      <span className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${(org.ssl_verify ?? true) ? "translate-x-4" : "translate-x-0"}`} />
                      <input type="checkbox" className="sr-only"
                        checked={org.ssl_verify ?? true}
                        onChange={(e) => handleOrgChange("ssl_verify", e.target.checked)}
                        disabled={saving} />
                    </div>
                    <span className="text-sm text-gray-700">Verify SSL certificate</span>
                  </label>
                </div>
              </div>

              {saveError && <ErrorAlert message="Failed to save organisation" detail={saveError} />}

              <div className="flex items-center justify-between">
                <button type="button" onClick={() => setStep("credentials")}
                  className="text-sm text-gray-500 hover:text-gray-700">
                  ← Back
                </button>
                <button type="button" onClick={handleSaveOrg} disabled={saving || !orgValid}
                  className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50">
                  {saving && <InlineSpinner />}
                  {saving ? "Saving…" : "Save Organisation"}
                </button>
              </div>
            </div>
          )}

          {step === "done" && (
            <div className="space-y-4 text-center">
              <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-green-100">
                <svg className="h-7 w-7 text-green-600" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="m4.5 12.75 6 6 9-13.5" />
                </svg>
              </div>
              <h2 className="text-lg font-semibold text-gray-900">Setup Complete!</h2>
              <p className="text-sm text-gray-600">
                Your first organisation has been saved. The background collector will start on its next
                scheduled run.
              </p>
              <p className="text-sm text-gray-600">
                Use <strong>Admin → Settings</strong> to configure collection schedules, credentials, and
                more.
              </p>
              <button
                type="button"
                onClick={() => window.location.assign("/")}
                className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-5 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700"
              >
                Go to Dashboard
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// SetupModeGuard — redirects admins to /admin/setup when no orgs configured.
// Renders children immediately for non-admin users or while checking.
// ---------------------------------------------------------------------------

export function useSetupRequired(): { setupRequired: boolean; checking: boolean } {
  const [setupRequired, setSetupRequired] = useState(false);
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    let cancelled = false;
    fetchConfigOrganisations()
      .then((orgs) => {
        if (!cancelled) setSetupRequired(orgs.length === 0);
      })
      .catch(() => {
        // If we can't fetch (e.g. not admin, or network error), don't block.
      })
      .finally(() => {
        if (!cancelled) setChecking(false);
      });
    return () => { cancelled = true; };
  }, []);

  return { setupRequired, checking };
}
