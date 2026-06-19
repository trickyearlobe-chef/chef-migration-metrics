// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from "react";
import { fetchAdminStatus } from "../api";
import type { AdminStatus } from "../types";
import { ErrorAlert, LoadingSpinner } from "../components/Feedback";
import { StatusBadge } from "../components/StatusBadge";
import { formatDate } from "./credentials/constants";

// overallVariant maps the top-level health to a badge colour.
function overallVariant(status: string): "healthy" | "warning" {
  return status === "healthy" ? "healthy" : "warning";
}

// runStatusVariant maps a collection-run / datastore status string to a badge
// colour. Unknown strings fall back to the neutral "unknown" pill.
function runStatusVariant(
  status: string,
): "healthy" | "unhealthy" | "warning" | "unknown" {
  switch (status) {
    case "completed":
    case "connected":
      return "healthy";
    case "failed":
    case "error":
      return "unhealthy";
    case "running":
      return "warning";
    default:
      return "unknown";
  }
}

function SectionCard({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-100 px-4 py-3">
        <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
      </div>
      <div className="space-y-3 p-4">{children}</div>
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between gap-4 text-sm">
      <span className="text-gray-500">{label}</span>
      <span className="font-medium text-gray-900">{children}</span>
    </div>
  );
}

export function AdminStatusPage() {
  const [data, setData] = useState<AdminStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetchAdminStatus()
      .then((resp) => {
        if (!cancelled) setData(resp);
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(err instanceof Error ? err.message : "Failed to load status.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => load(), [load]);

  if (loading) return <LoadingSpinner message="Loading status…" />;
  if (error)
    return (
      <ErrorAlert message="Failed to load status" detail={error} onRetry={load} />
    );
  if (!data) return null;

  const cs = data.credential_storage;
  const credTypes = Object.entries(cs.credential_types);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">
            Operational Status
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Datastore, credential storage, and collection health. Version{" "}
            <span className="font-mono">{data.version}</span>.
          </p>
        </div>
        <StatusBadge variant={overallVariant(data.status)} label={data.status} />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <SectionCard title="Datastore">
          <Field label="Connection">
            <StatusBadge
              variant={runStatusVariant(data.datastore.status)}
              label={data.datastore.status}
              size="sm"
            />
          </Field>
          <Field label="Pending migrations">
            {data.datastore.pending_migrations}
          </Field>
        </SectionCard>

        <SectionCard title="Credential storage">
          <Field label="Encryption key">
            <StatusBadge
              variant={cs.encryption_key_configured ? "healthy" : "unknown"}
              label={cs.encryption_key_configured ? "configured" : "not configured"}
              size="sm"
            />
          </Field>
          <Field label="Total credentials">{cs.total_credentials}</Field>
          <Field label="Orphaned">{cs.orphaned_credentials}</Field>
          {credTypes.length > 0 && (
            <div className="border-t border-gray-100 pt-2">
              <p className="mb-1 text-xs font-medium text-gray-500">By type</p>
              {credTypes.map(([type, count]) => (
                <Field key={type} label={type}>
                  {count}
                </Field>
              ))}
            </div>
          )}
        </SectionCard>

        <SectionCard title="Collection">
          <Field label="Next run">{formatDate(data.collection.next_run_at)}</Field>
          <Field label="Last run">{formatDate(data.collection.last_run_at)}</Field>
          <Field label="Last run status">
            {data.collection.last_run_status ? (
              <StatusBadge
                variant={runStatusVariant(data.collection.last_run_status)}
                label={data.collection.last_run_status}
                size="sm"
              />
            ) : (
              "—"
            )}
          </Field>
        </SectionCard>
      </div>

      <SectionCard title={`Organisations (${data.organisations.length})`}>
        {data.organisations.length === 0 ? (
          <p className="text-sm text-gray-500">No organisations configured.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="border-b border-gray-100 text-left text-xs font-medium text-gray-500">
                  <th className="py-2 pr-4">Name</th>
                  <th className="py-2 pr-4">Credential source</th>
                  <th className="py-2 pr-4">Status</th>
                  <th className="py-2 pr-4">Nodes</th>
                  <th className="py-2">Last collected</th>
                </tr>
              </thead>
              <tbody>
                {data.organisations.map((org) => (
                  <tr key={org.name} className="border-b border-gray-50">
                    <td className="py-2 pr-4 font-medium text-gray-900">
                      {org.name}
                    </td>
                    <td className="py-2 pr-4 text-gray-700">
                      {org.credential_source}
                    </td>
                    <td className="py-2 pr-4">
                      <StatusBadge
                        variant={runStatusVariant(org.status)}
                        label={org.status}
                        size="sm"
                      />
                    </td>
                    <td className="py-2 pr-4 text-gray-700">
                      {org.node_count.toLocaleString()}
                    </td>
                    <td className="py-2 text-gray-700">
                      {formatDate(org.last_collected_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </SectionCard>
    </div>
  );
}
