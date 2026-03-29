import type { CoverageReport } from "../types";

interface PlatformCoverageCardProps {
  coverage: CoverageReport;
  evaluatedAt: string;
}

/**
 * Displays platform coverage analysis for a cookbook:
 * - Summary bar with coverage percentage
 * - Tested and in production (green)
 * - In production but untested / gaps (red/amber)
 * - Tested but not in production (gray/info)
 */
export function PlatformCoverageCard({ coverage, evaluatedAt }: PlatformCoverageCardProps) {
  const {
    coverage_percentage,
    gap_count,
    total_production_nodes,
    covered_node_count,
    tested_and_in_production,
    in_production_not_tested,
    tested_not_in_production,
  } = coverage;

  // Coverage color based on percentage.
  const coverageColor =
    coverage_percentage >= 90
      ? "text-green-700 bg-green-50 border-green-200"
      : coverage_percentage >= 50
        ? "text-amber-700 bg-amber-50 border-amber-200"
        : "text-red-700 bg-red-50 border-red-200";

  const barColor =
    coverage_percentage >= 90
      ? "bg-green-500"
      : coverage_percentage >= 50
        ? "bg-amber-500"
        : "bg-red-500";

  return (
    <div className="card space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-gray-500">
          Platform Coverage
        </h3>
        <span className="text-xs text-gray-400">
          Last evaluated: {new Date(evaluatedAt).toLocaleString()}
        </span>
      </div>

      {/* Summary stats */}
      <div className="flex flex-wrap items-center gap-4">
        <div className={`rounded-lg border px-4 py-2 ${coverageColor}`}>
          <div className="text-2xl font-bold">{coverage_percentage}%</div>
          <div className="text-xs">Coverage</div>
        </div>
        <div className="text-sm text-gray-600 space-y-0.5">
          <div>{total_production_nodes} production node{total_production_nodes !== 1 ? "s" : ""}</div>
          <div>{covered_node_count} covered by tests</div>
          {gap_count > 0 && (
            <div className="text-red-600 font-medium">
              {gap_count} untested platform{gap_count !== 1 ? "s" : ""} in production
            </div>
          )}
        </div>
      </div>

      {/* Coverage bar */}
      <div className="h-2 w-full rounded-full bg-gray-200">
        <div
          className={`h-2 rounded-full transition-all ${barColor}`}
          style={{ width: `${Math.min(coverage_percentage, 100)}%` }}
        />
      </div>

      {/* Tested and in production */}
      {tested_and_in_production.length > 0 && (
        <div>
          <h4 className="text-xs font-medium text-gray-500 mb-1.5">
            ✅ Tested &amp; In Production
          </h4>
          <div className="flex flex-wrap gap-1.5">
            {tested_and_in_production.map((m) => (
              <span
                key={`${m.kitchen_name}-${m.platform}-${m.platform_version}`}
                className="inline-flex items-center gap-1.5 rounded-full border border-green-200 bg-green-50 px-2.5 py-0.5 text-xs text-green-800"
              >
                <span className="font-medium">{m.kitchen_name}</span>
                <span className="text-green-600">→</span>
                <span>{m.platform} {m.platform_version}</span>
                <span className="text-green-600">({m.node_count} node{m.node_count !== 1 ? "s" : ""})</span>
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Gaps: in production but untested */}
      {in_production_not_tested.length > 0 && (
        <div>
          <h4 className="text-xs font-medium text-red-600 mb-1.5">
            ⚠️ In Production — Not Tested
          </h4>
          <div className="flex flex-wrap gap-1.5">
            {in_production_not_tested.map((p) => (
              <span
                key={`${p.platform}-${p.platform_version}`}
                className="inline-flex items-center gap-1.5 rounded-full border border-red-200 bg-red-50 px-2.5 py-0.5 text-xs text-red-800"
              >
                <span className="font-medium">{p.platform} {p.platform_version}</span>
                <span className="text-red-600">
                  ({p.node_count} node{p.node_count !== 1 ? "s" : ""})
                </span>
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Tested but not in production */}
      {tested_not_in_production.length > 0 && (
        <div>
          <h4 className="text-xs font-medium text-gray-500 mb-1.5">
            ℹ️ Tested — Not In Production
          </h4>
          <div className="flex flex-wrap gap-1.5">
            {tested_not_in_production.map((name) => (
              <span
                key={name}
                className="inline-flex items-center rounded-full border border-gray-200 bg-gray-50 px-2.5 py-0.5 text-xs text-gray-600"
              >
                {name}
              </span>
            ))}
          </div>
        </div>
      )}

      {/* No production data */}
      {total_production_nodes === 0 && (
        <p className="text-sm text-gray-400 italic">
          No production nodes found for this cookbook. Coverage analysis requires node data from a collection run.
        </p>
      )}
    </div>
  );
}
