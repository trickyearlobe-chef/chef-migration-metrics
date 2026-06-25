// SPDX-License-Identifier: Apache-2.0

import type { CopClassification } from "../types";

// ---------------------------------------------------------------------------
// Classification badge — shared across Cop Analysis, remediation detail pages
// ---------------------------------------------------------------------------

const CLASSIFICATION_STYLES: Record<
  CopClassification,
  { bg: string; text: string; label: string }
> = {
  blocker: { bg: "bg-red-100", text: "text-red-800", label: "Blocker" },
  review: { bg: "bg-amber-100", text: "text-amber-800", label: "Review" },
  noise: { bg: "bg-gray-100", text: "text-gray-500", label: "Noise" },
  unclassified: { bg: "bg-blue-50", text: "text-blue-600", label: "Unclassified" },
};

export function ClassificationBadge({
  classification,
  source,
}: {
  classification: CopClassification;
  source?: string;
}) {
  const style = CLASSIFICATION_STYLES[classification] ?? CLASSIFICATION_STYLES.unclassified;
  const tooltip = source ? `Source: ${source.replace(/_/g, " ")}` : undefined;
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${style.bg} ${style.text}`}
      title={tooltip}
    >
      {style.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Classification filter buttons — reusable filter bar
// ---------------------------------------------------------------------------

export const CLASSIFICATION_FILTERS: { value: string; label: string; colour: string }[] = [
  { value: "", label: "All", colour: "bg-gray-100 text-gray-700" },
  { value: "blocker", label: "Blockers", colour: "bg-red-100 text-red-700" },
  { value: "review", label: "Review", colour: "bg-amber-100 text-amber-700" },
  { value: "noise", label: "Noise", colour: "bg-gray-100 text-gray-500" },
  { value: "unclassified", label: "Unclassified", colour: "bg-blue-50 text-blue-600" },
];

export function ClassificationFilterBar({
  activeFilter,
  onFilterChange,
}: {
  activeFilter: string;
  onFilterChange: (value: string) => void;
}) {
  return (
    <div className="flex gap-1">
      {CLASSIFICATION_FILTERS.map((f) => (
        <button
          key={f.value}
          onClick={() => onFilterChange(f.value)}
          className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
            activeFilter === f.value
              ? f.colour + " ring-2 ring-offset-1 ring-blue-400"
              : "bg-white text-gray-600 border border-gray-200 hover:bg-gray-50"
          }`}
        >
          {f.label}
        </button>
      ))}
    </div>
  );
}
