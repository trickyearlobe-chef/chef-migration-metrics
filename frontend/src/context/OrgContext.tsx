import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from "react";
import { fetchOrganisations } from "../api";
import type { Organisation } from "../types";

// ---------------------------------------------------------------------------
// Organisation context — stores the list of orgs fetched from the API and
// the currently-selected org name. The selected org is passed as
// `?organisation=` to all downstream API calls so every view is scoped.
//
// "All organisations" is represented by selectedOrg being an empty string.
// ---------------------------------------------------------------------------

interface OrgContextValue {
  /** All organisations returned by GET /api/v1/organisations. */
  organisations: Organisation[];
  /** Whether the org list is currently loading. */
  loading: boolean;
  /** Error message if the org list fetch failed. */
  error: string | null;
  /** Currently selected organisation name, or "" for all. */
  selectedOrg: string;
  /** Change the selected organisation. Pass "" for "all". */
  setSelectedOrg: (name: string) => void;
  /** Re-fetch the organisation list from the API. */
  refresh: () => void;
}

const OrgContext = createContext<OrgContextValue | undefined>(undefined);

export function OrgProvider({ children }: { children: ReactNode }) {
  const [organisations, setOrganisations] = useState<Organisation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedOrg, setSelectedOrg] = useState<string>("");

  const refresh = useCallback(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    fetchOrganisations()
      .then((res) => {
        if (!cancelled) {
          setOrganisations(res.data ?? []);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          const message =
            err instanceof Error ? err.message : "Failed to load organisations";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  // Fetch on mount.
  useEffect(() => {
    const cancel = refresh();
    return cancel;
  }, [refresh]);

  return (
    <OrgContext.Provider
      value={{
        organisations,
        loading,
        error,
        selectedOrg,
        setSelectedOrg,
        refresh,
      }}
    >
      {children}
    </OrgContext.Provider>
  );
}

/**
 * Hook to access the organisation context.
 * Must be used inside an `<OrgProvider>`.
 */
export function useOrg(): OrgContextValue {
  const ctx = useContext(OrgContext);
  if (ctx === undefined) {
    throw new Error("useOrg must be used within an <OrgProvider>");
  }
  return ctx;
}
