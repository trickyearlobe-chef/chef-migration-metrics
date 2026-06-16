import { useState, useEffect, useCallback } from "react";
import { fetchCredentials, testCredential } from "../../api";
import type { Credential, TestCredentialResponse } from "../../types";
import { formatDate } from "./constants";
import { CredentialTypeBadge } from "./CredentialTypeBadge";
import { CreateCredentialModal } from "./CreateCredentialModal";
import { RotateCredentialModal } from "./RotateCredentialModal";
import { DeleteCredentialModal } from "./DeleteCredentialModal";
import { TestResultBanner } from "./TestResultBanner";

export function AdminCredentialsPage() {
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [showCreate, setShowCreate] = useState(false);
  const [rotateTarget, setRotateTarget] = useState<Credential | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Credential | null>(null);
  const [testResult, setTestResult] = useState<TestCredentialResponse | null>(
    null,
  );
  const [testing, setTesting] = useState<string | null>(null);

  const loadCredentials = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    fetchCredentials({ per_page: 500 })
      .then((res) => {
        if (!cancelled) {
          setCredentials(res.data ?? []);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          const message =
            err instanceof Error ? err.message : "Failed to load credentials";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const cancel = loadCredentials();
    return cancel;
  }, [loadCredentials]);

  async function handleTest(name: string) {
    setTesting(name);
    setTestResult(null);
    try {
      const result = await testCredential(name);
      setTestResult(result);
    } catch (err: unknown) {
      setTestResult({
        valid: false,
        error: err instanceof Error ? err.message : "Test request failed",
      });
    } finally {
      setTesting(null);
    }
  }

  return (
    <div>
      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-800">
            Credential Management
          </h2>
          <p className="text-sm text-gray-500">
            Manage encrypted credentials such as Chef API keys.
          </p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
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
          Add Credential
        </button>
      </div>

      {/* Test result banner */}
      <TestResultBanner
        result={testResult}
        onDismiss={() => setTestResult(null)}
      />

      {/* Error banner */}
      {error && (
        <div className="mb-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </div>
      )}

      {/* Loading */}
      {loading && (
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
            Loading credentials…
          </span>
        </div>
      )}

      {/* Credentials table */}
      {!loading && credentials.length > 0 && (
        <div className="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Name
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Type
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Created By
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Last Rotated
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Created
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {credentials.map((c) => (
                <tr key={c.name} className="hover:bg-gray-50">
                  <td className="whitespace-nowrap px-4 py-3 text-sm font-medium text-gray-900">
                    {c.name}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3">
                    <CredentialTypeBadge type={c.credential_type} />
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                    {c.created_by}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                    {formatDate(c.last_rotated_at)}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                    {formatDate(c.created_at)}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      {/* Test */}
                      <button
                        onClick={() => handleTest(c.name)}
                        disabled={testing === c.name}
                        title="Test credential"
                        className="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-blue-600 disabled:opacity-50"
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
                            d="M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
                          />
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            d="M15.91 11.672a.375.375 0 0 1 0 .656l-5.603 3.113a.375.375 0 0 1-.557-.328V8.887c0-.286.307-.466.557-.327l5.603 3.112Z"
                          />
                        </svg>
                      </button>
                      {/* Rotate */}
                      <button
                        onClick={() => setRotateTarget(c)}
                        title="Rotate credential"
                        className="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-amber-600"
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
                            d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182M21.015 4.357v4.992"
                          />
                        </svg>
                      </button>
                      {/* Delete */}
                      <button
                        onClick={() => setDeleteTarget(c)}
                        title="Delete credential"
                        className="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-red-600"
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
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Empty state */}
      {!loading && credentials.length === 0 && !error && (
        <div className="rounded-lg border border-gray-200 bg-white py-12 text-center">
          <p className="text-sm text-gray-500">No credentials found.</p>
        </div>
      )}

      {/* Modals */}
      <CreateCredentialModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        onCreated={loadCredentials}
      />
      <RotateCredentialModal
        open={!!rotateTarget}
        onClose={() => setRotateTarget(null)}
        onRotated={loadCredentials}
        target={rotateTarget}
      />
      <DeleteCredentialModal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onDeleted={loadCredentials}
        target={deleteTarget}
      />
    </div>
  );
}
