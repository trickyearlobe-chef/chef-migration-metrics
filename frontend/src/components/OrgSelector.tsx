import { useOrg } from "../context/OrgContext";

/**
 * Dropdown selector for choosing which Chef organisation to scope all views
 * to. Populated from GET /api/v1/organisations via the OrgContext.
 *
 * Selecting "All Organisations" sets the selected org to "" which means
 * downstream API calls omit the `?organisation=` query parameter.
 */
export function OrgSelector() {
  const { organisations, loading, error, selectedOrg, setSelectedOrg } =
    useOrg();

  if (error) {
    return (
      <div className="flex items-center gap-2 text-sm text-red-600">
        <svg
          className="h-4 w-4 shrink-0"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={1.5}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 9v3.75m9-.75a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 3.75h.008v.008H12v-.008Z"
          />
        </svg>
        <span className="truncate" title={error}>
          Failed to load orgs
        </span>
      </div>
    );
  }

  return (
    <div className="relative">
      <label
        htmlFor="org-selector"
        className="sr-only"
      >
        Organisation
      </label>
      <select
        id="org-selector"
        value={selectedOrg}
        onChange={(e) => setSelectedOrg(e.target.value)}
        disabled={loading}
        className={[
          "block w-full appearance-none rounded-md border border-gray-300",
          "bg-white py-1.5 pl-3 pr-8 text-sm text-gray-700 shadow-sm",
          "transition-colors",
          "hover:border-gray-400",
          "focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500",
          "disabled:cursor-wait disabled:opacity-60",
        ].join(" ")}
      >
        <option value="">
          {loading ? "Loading…" : "All Organisations"}
        </option>
        {organisations.map((org) => (
          <option key={org.name} value={org.name}>
            {org.name}
            {org.node_count > 0 ? ` (${org.node_count} nodes)` : ""}
          </option>
        ))}
      </select>

      {/* Custom dropdown chevron */}
      <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-2">
        <svg
          className="h-4 w-4 text-gray-400"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={2}
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="m19.5 8.25-7.5 7.5-7.5-7.5"
          />
        </svg>
      </div>
    </div>
  );
}
