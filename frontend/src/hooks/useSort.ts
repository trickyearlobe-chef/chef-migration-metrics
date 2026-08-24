// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useState, useCallback } from "react";

export type SortOrder = "asc" | "desc";

export interface UseSortOptions<T extends string> {
  /** Initial sort field. */
  defaultField: T;
  /** Initial sort order. */
  defaultOrder?: SortOrder;
  /**
   * Fields that should default to descending when first selected.
   * All other fields default to ascending. This matches the common UX
   * pattern where numeric/date columns show "biggest first" and text
   * columns show "A–Z first".
   */
  descendingFields?: T[];
}

export interface UseSortReturn<T extends string> {
  sortField: T;
  sortOrder: SortOrder;
  /** Call with a field name to toggle or change the active sort. */
  handleSort: (field: T) => void;
}

/**
 * Generic hook for column-header sorting state.
 *
 * Handles the toggle-on-same-field / pick-default-on-new-field logic shared by
 * the page components.
 */
export function useSort<T extends string>({
  defaultField,
  defaultOrder = "asc",
  descendingFields = [],
}: UseSortOptions<T>): UseSortReturn<T> {
  const [sortField, setSortField] = useState<T>(defaultField);
  const [sortOrder, setSortOrder] = useState<SortOrder>(defaultOrder);

  const handleSort = useCallback(
    (field: T) => {
      if (field === sortField) {
        setSortOrder((prev) => (prev === "asc" ? "desc" : "asc"));
      } else {
        setSortField(field);
        setSortOrder(
          (descendingFields as string[]).includes(field) ? "desc" : "asc",
        );
      }
    },
    [sortField, descendingFields],
  );

  return { sortField, sortOrder, handleSort };
}
