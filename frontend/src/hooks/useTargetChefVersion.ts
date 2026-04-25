// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useGlobalFilters } from "../context/GlobalFilterContext";

export interface UseTargetChefVersionOptions {
  /**
   * Optional initial version seeded from URL search params or other source.
   * Ignored — the GlobalFilterContext now owns version selection and URL
   * persistence. Kept for API compatibility so callers don't need changes.
   */
  initialVersion?: string;
}

export interface UseTargetChefVersionReturn {
  /** All available target Chef versions from the backend config. */
  targetVersions: string[];
  /** The currently selected target version. */
  selectedVersion: string;
  /** Setter to change the selected version (e.g. from a <select>). */
  setSelectedVersion: (version: string) => void;
  /** True while the initial version list is being fetched. */
  versionsLoading: boolean;
}

/**
 * Thin wrapper around GlobalFilterContext that preserves the original hook
 * API. All pages using this hook now share a single global target version
 * that persists across navigation via URL params.
 */
export function useTargetChefVersion(
  _options: UseTargetChefVersionOptions = {},
): UseTargetChefVersionReturn {
  const {
    targetVersions,
    targetChefVersion,
    setTargetChefVersion,
    versionsLoading,
  } = useGlobalFilters();

  return {
    targetVersions,
    selectedVersion: targetChefVersion,
    setSelectedVersion: setTargetChefVersion,
    versionsLoading,
  };
}
