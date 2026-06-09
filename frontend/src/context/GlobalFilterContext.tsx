// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from "react";
import { useSearchParams } from "react-router-dom";
import { fetchFilterTargetChefVersions } from "../api";
import { highestSemver } from "../semver";

// ---------------------------------------------------------------------------
// Global filter context — stores cross-cutting filters (target Chef version
// and staleness tiers) that apply across multiple pages. Values are persisted
// in URL search params so that bookmarking / link-sharing restores state.
//
// URL params: ?target_chef_version=18.5.0&stale_tiers=fresh,warning
// ---------------------------------------------------------------------------

const PARAM_TARGET_VERSION = "target_chef_version";
const PARAM_STALE_TIERS = "stale_tiers";

const VALID_STALE_TIERS = ["fresh", "warning", "critical"] as const;

function parseStaleParam(raw: string): string[] {
  if (!raw) return [];
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter((s) => (VALID_STALE_TIERS as readonly string[]).includes(s));
}

export interface GlobalFilterContextValue {
  /** All available target Chef versions from the backend config. */
  targetVersions: string[];
  /** The currently selected target Chef version. */
  targetChefVersion: string;
  /** Change the selected target Chef version. */
  setTargetChefVersion: (v: string) => void;
  /** Current staleness tier filters (empty = all). */
  staleTiers: string[];
  /** Change the staleness tier filters. */
  setStaleTiers: (tiers: string[]) => void;
  /** True while the version list is being fetched. */
  versionsLoading: boolean;
}

const GlobalFilterContext = createContext<GlobalFilterContextValue | undefined>(
  undefined,
);

export function GlobalFilterProvider({ children }: { children: ReactNode }) {
  const [searchParams, setSearchParams] = useSearchParams();

  // Read initial values from URL params.
  const urlVersion = searchParams.get(PARAM_TARGET_VERSION) ?? "";
  const urlStaleTiers = searchParams.get(PARAM_STALE_TIERS) ?? "";

  const [targetVersions, setTargetVersions] = useState<string[]>([]);
  const [targetChefVersion, setTargetChefVersionState] = useState<string>(
    urlVersion,
  );
  const [staleTiers, setStaleTiersState] = useState<string[]>(
    parseStaleParam(urlStaleTiers),
  );
  const [versionsLoading, setVersionsLoading] = useState(true);

  // Sync URL params whenever filter values change. Preserves existing
  // non-global params so page-specific params are not overwritten.
  const syncParams = useCallback(
    (version: string, tiers: string[]) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);

          // Set or remove target_chef_version.
          if (version) {
            next.set(PARAM_TARGET_VERSION, version);
          } else {
            next.delete(PARAM_TARGET_VERSION);
          }

          // Set or remove stale_tiers (omit when empty = "all").
          if (tiers.length > 0) {
            next.set(PARAM_STALE_TIERS, tiers.join(","));
          } else {
            next.delete(PARAM_STALE_TIERS);
          }

          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const setTargetChefVersion = useCallback(
    (v: string) => {
      setTargetChefVersionState(v);
      syncParams(v, staleTiers);
    },
    [staleTiers, syncParams],
  );

  const setStaleTiers = useCallback(
    (tiers: string[]) => {
      setStaleTiersState(tiers);
      syncParams(targetChefVersion, tiers);
    },
    [targetChefVersion, syncParams],
  );

  // Fetch available target versions on mount.
  useEffect(() => {
    let cancelled = false;
    setVersionsLoading(true);

    fetchFilterTargetChefVersions()
      .then((res) => {
        if (cancelled) return;
        const versions = res.data ?? [];
        setTargetVersions(versions);

        if (versions.length > 0) {
          // If URL had a version that exists in the list, keep it.
          if (urlVersion && versions.includes(urlVersion)) {
            setTargetChefVersionState(urlVersion);
          } else {
            // Auto-select highest version.
            const highest = highestSemver(versions) ?? versions[0];
            setTargetChefVersionState(highest);
            syncParams(highest, staleTiers);
          }
        }
      })
      .catch(() => {
        if (!cancelled) {
          setTargetVersions([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setVersionsLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, []); // intentionally run only on mount

  return (
    <GlobalFilterContext.Provider
      value={{
        targetVersions,
        targetChefVersion,
        setTargetChefVersion,
        staleTiers,
        setStaleTiers,
        versionsLoading,
      }}
    >
      {children}
    </GlobalFilterContext.Provider>
  );
}

/**
 * Hook to access the global filter context.
 * Must be used inside a `<GlobalFilterProvider>`.
 */
export function useGlobalFilters(): GlobalFilterContextValue {
  const ctx = useContext(GlobalFilterContext);
  if (ctx === undefined) {
    throw new Error(
      "useGlobalFilters must be used within a <GlobalFilterProvider>",
    );
  }
  return ctx;
}
