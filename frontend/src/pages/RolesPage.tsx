import { useState, useEffect, useCallback } from "react";
import { DEFAULT_PAGE_SIZE } from "../constants";
import { Link, useSearchParams } from "react-router-dom";
import { useOrg } from "../context/OrgContext";
import { useSort } from "../hooks/useSort";
import { useTargetChefVersion } from "../hooks/useTargetChefVersion";
import { SortableColumnHeader } from "../components/SortableColumnHeader";
import { FilterInput } from "../components/FilterInputs";
import { FilterMultiCheckbox } from "../components/FilterMultiCheckbox";
import { fetchRoles } from "../api";
import type {
  RoleListItem,
  RoleSummary,
  Pagination as PaginationType,
} from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";
import { Pagination } from "../components/Pagination";
import { CompatibilityBadge } from "../components/StatusBadge";
import type { RoleFilterQuery } from "../api/roles";

function SummaryBar({
  summary,
  onFilterClick,
}: {
  summary: RoleSummary | null;
  onFilterClick: (status: string) => void;
}) {
  if (!summary || summary.total_roles === 0) return null;

  const pct = (n: number) =>
    summary.total_roles > 0
      ? ((n / summary.total_roles) * 100).toFixed(1)
      : "0";

  return (
    <div className="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="flex h-3">
        {summary.compatible_roles > 0 && (
          <button
            type="button"
            className="bg-green-500 transition-all hover:brightness-110 cursor-pointer"
            style={{ width: `${pct(summary.compatible_roles)}%` }}
            title={`Compatible: ${summary.compatible_roles} — click to filter`}
            onClick={() => onFilterClick("compatible")}
          />
        )}
        {summary.untested_roles > 0 && (
          <button
            type="button"
            className="bg-gray-400 transition-all hover:brightness-110 cursor-pointer"
            style={{ width: `${pct(summary.untested_roles)}%` }}
            title={`Untested: ${summary.untested_roles} — click to filter`}
            onClick={() => onFilterClick("untested")}
          />
        )}
        {summary.incompatible_roles > 0 && (
          <button
            type="button"
            className="bg-red-500 transition-all hover:brightness-110 cursor-pointer"
            style={{ width: `${pct(summary.incompatible_roles)}%` }}
            title={`Incompatible: ${summary.incompatible_roles} — click to filter`}
            onClick={() => onFilterClick("incompatible")}
          />
        )}
      </div>
      <div className="flex items-center justify-between px-4 py-2 text-xs text-gray-600">
        <button
          type="button"
          onClick={() => onFilterClick("compatible")}
          className="cursor-pointer hover:underline"
        >
          <span className="mr-1 inline-block h-2 w-2 rounded-full bg-green-500" />
          Compatible: {summary.compatible_roles}
        </button>
        <button
          type="button"
          onClick={() => onFilterClick("untested")}
          className="cursor-pointer hover:underline"
        >
          <span className="mr-1 inline-block h-2 w-2 rounded-full bg-gray-400" />
          Untested: {summary.untested_roles}
        </button>
        <button
          type="button"
          onClick={() => onFilterClick("incompatible")}
          className="cursor-pointer hover:underline"
        >
          <span className="mr-1 inline-block h-2 w-2 rounded-full bg-red-500" />
          Incompatible: {summary.incompatible_roles}
        </button>
        <span className="font-medium text-gray-800">
          Total: {summary.total_roles}
        </span>
      </div>
    </div>
  );
}

export function RolesPage() {
  const { selectedOrg } = useOrg();
  const [roles, setRoles] = useState<RoleListItem[]>([]);
  const [summary, setSummary] = useState<RoleSummary | null>(null);
  const [pagination, setPagination] = useState<PaginationType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [searchParams, setSearchParams] = useSearchParams();

  const [nameFilter, setNameFilter] = useState(searchParams.get("name") || "");
  const [compatibility, setCompatibility] = useState<string[]>(
    searchParams.get("compatibility_status")?.split(",").filter(Boolean) ?? [],
  );
  const [page, setPage] = useState(1);
  const perPage = DEFAULT_PAGE_SIZE;

  const { sortField, sortOrder, handleSort } = useSort({
    defaultField: "name",
    defaultOrder: "asc",
    descendingFields: ["node_count", "incompatible_cookbook_count"],
  });

  const { selectedVersion: selectedTargetVersion } = useTargetChefVersion({
    initialVersion: searchParams.get("target_chef_version") || undefined,
  });

  useEffect(() => {
    if (
      searchParams.has("name") ||
      searchParams.has("compatibility_status") ||
      searchParams.has("target_chef_version")
    ) {
      setSearchParams({}, { replace: true });
    }
  }, []);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);

    const filters: RoleFilterQuery = {
      page,
      per_page: perPage,
    };
    if (selectedOrg) filters.organisation = selectedOrg;
    if (nameFilter) filters.name = nameFilter;
    if (compatibility.length > 0)
      filters.compatibility_status = compatibility.join(",");
    if (selectedTargetVersion)
      filters.target_chef_version = selectedTargetVersion;
    if (sortField) filters.sort = sortField;
    if (sortOrder) filters.order = sortOrder;

    fetchRoles(filters)
      .then((res) => {
        setRoles(res.data ?? []);
        setSummary(res.summary ?? null);
        setPagination(res.pagination);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [
    selectedOrg,
    nameFilter,
    compatibility,
    selectedTargetVersion,
    page,
    sortField,
    sortOrder,
  ]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    setPage(1);
  }, [
    selectedOrg,
    nameFilter,
    compatibility,
    selectedTargetVersion,
    sortField,
    sortOrder,
  ]);

  const activeFilterCount =
    (nameFilter ? 1 : 0) + (compatibility.length > 0 ? 1 : 0);

  const clearFilters = () => {
    setNameFilter("");
    setCompatibility([]);
  };

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-bold text-gray-800">Roles</h2>

      <SummaryBar
        summary={summary}
        onFilterClick={(status) => setCompatibility([status])}
      />

      <div className="flex flex-wrap items-end gap-3">
        <FilterInput
          label="Name"
          value={nameFilter}
          onChange={setNameFilter}
          placeholder="Filter by name"
        />
        <FilterMultiCheckbox
          label="Compatibility"
          options={[
            { value: "compatible", label: "Compatible" },
            { value: "incompatible", label: "Incompatible" },
            { value: "untested", label: "Untested" },
          ]}
          selected={compatibility}
          onChange={setCompatibility}
        />
        {activeFilterCount > 0 && (
          <button
            onClick={clearFilters}
            className="mb-0.5 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-900"
            title="Clear all filters"
          >
            Clear ({activeFilterCount})
          </button>
        )}
      </div>

      {loading && <LoadingSpinner message="Loading roles…" />}
      {error && <ErrorAlert message={error} onRetry={load} />}
      {!loading && !error && (
        <>
          {roles.length === 0 ? (
            <EmptyState
              title="No roles found"
              description="Adjust filters or wait for data collection."
            />
          ) : (
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <SortableColumnHeader
                      label="Name"
                      field="name"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                    <th>Organisation(s)</th>
                    <SortableColumnHeader
                      label="Nodes"
                      field="node_count"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                      className="text-right"
                    />
                    <th className="text-right">Cookbooks</th>
                    <SortableColumnHeader
                      label="Compatibility"
                      field="incompatible_cookbook_count"
                      currentField={sortField}
                      currentOrder={sortOrder}
                      onSort={handleSort}
                    />
                  </tr>
                </thead>
                <tbody>
                  {roles.map((role) => (
                    <tr key={role.role_name}>
                      <td>
                        <Link
                          to={`/roles/${encodeURIComponent(role.role_name)}`}
                          className="font-medium text-blue-600 hover:text-blue-800 hover:underline"
                        >
                          {role.role_name}
                        </Link>
                      </td>
                      <td>
                        <span className="text-sm text-gray-600">
                          {role.organisations?.join(", ") || "—"}
                        </span>
                      </td>
                      <td className="text-right">
                        <span className="text-sm text-gray-700">
                          {role.node_count.toLocaleString()}
                        </span>
                      </td>
                      <td className="text-right">
                        <span className="text-sm text-gray-700">
                          {role.total_cookbook_count}
                        </span>
                      </td>
                      <td>
                        <CompatibilityBadge
                          status={role.compatibility_status ?? "untested"}
                          size="sm"
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {pagination && (
            <Pagination pagination={pagination} onPageChange={setPage} />
          )}
        </>
      )}
    </div>
  );
}
