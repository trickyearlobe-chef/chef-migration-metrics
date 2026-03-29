// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useState, useEffect } from "react";
import { fetchFilterTargetChefVersions } from "../api";
import { highestSemver } from "../semver";

export interface UseTargetChefVersionOptions {
  /**
   * Optional initial version seeded from URL search params or other source.
   * When provided and present in the fetched list, it takes priority over
   * the auto-selected highest version.
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
 * Hook that loads target Chef versions from the backend, auto-selects the
 * highest semver version, and exposes selection state.
 *
 * Replaces the identical load-versions-and-pick-highest pattern previously
 * duplicated in NodesPage, CookbooksPage, GitReposPage, RemediationPage,
 * CookbookRemediationPage, and GitRepoRemediationPage.
 */
export function useTargetChefVersion(
  options: UseTargetChefVersionOptions = {},
): UseTargetChefVersionReturn {
  const { initialVersion } = options;

  const [targetVersions, setTargetVersions] = useState<string[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<string>(
    initialVersion ?? "",
  );
  const [versionsLoading, setVersionsLoading] = useState(true);

  useEffect(() => {
    setVersionsLoading(true);
    fetchFilterTargetChefVersions()
      .then((res) => {
        const versions = res.data ?? [];
        setTargetVersions(versions);
        if (versions.length > 0 && !selectedVersion) {
          setSelectedVersion(highestSemver(versions) ?? versions[0]);
        } else if (
          initialVersion &&
          versions.length > 0 &&
          !versions.includes(initialVersion)
        ) {
          // initialVersion not in the list — fall back to highest.
          setSelectedVersion(highestSemver(versions) ?? versions[0]);
        }
      })
      .catch(() => {
        setTargetVersions([]);
      })
      .finally(() => {
        setVersionsLoading(false);
      });
  }, []); // intentionally run only on mount

  return { targetVersions, selectedVersion, setSelectedVersion, versionsLoading };
}
