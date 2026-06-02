// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import type { SortOrder } from "../hooks/useSort";

// ---------------------------------------------------------------------------
// SortableColumnHeader — shared table header cell with sort indicators
//
// Uses SVG chevrons for a consistent visual style across all sortable tables.
// Replaces 7+ local implementations (SortHeader, SortableHeader,
// SortableColHeader, SortIndicator, inline sortIndicator functions).
// ---------------------------------------------------------------------------

interface SortableColumnHeaderProps<T extends string> {
  /** Display label for the column. */
  label: string;
  /** Field name this column sorts by. */
  field: T;
  /** Currently active sort field. */
  currentField: T;
  /** Currently active sort order. */
  currentOrder: SortOrder;
  /** Callback when the header is clicked. */
  onSort: (field: T) => void;
  /** Optional extra className for the <th>. */
  className?: string;
  /** Optional tooltip shown on hover (title attribute). */
  tooltip?: string;
}

export function SortableColumnHeader<T extends string>({
  label,
  field,
  currentField,
  currentOrder,
  onSort,
  className,
  tooltip,
}: SortableColumnHeaderProps<T>) {
  const active = field === currentField;
  return (
    <th
      onClick={() => onSort(field)}
      title={tooltip}
      className={
        "cursor-pointer select-none hover:text-blue-600 " + (className ?? "")
      }
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {active ? (
          <svg
            className="h-3.5 w-3.5 text-blue-600"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d={
                currentOrder === "asc"
                  ? "M4.5 15.75l7.5-7.5 7.5 7.5"
                  : "M19.5 8.25l-7.5 7.5-7.5-7.5"
              }
            />
          </svg>
        ) : (
          <svg
            className="h-3.5 w-3.5 text-gray-300"
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={2}
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9"
            />
          </svg>
        )}
      </span>
    </th>
  );
}
