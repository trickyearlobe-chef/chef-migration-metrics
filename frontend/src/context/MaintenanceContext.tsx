// SPDX-License-Identifier: Apache-2.0

import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";

interface MaintenanceState {
  active: boolean;
  message: string;
}

interface MaintenanceContextType {
  maintenance: MaintenanceState;
  setMaintenance: (state: MaintenanceState) => void;
}

const MaintenanceContext = createContext<MaintenanceContextType>({
  maintenance: { active: false, message: "" },
  setMaintenance: () => {},
});

// Global event target for maintenance mode signals from apiFetch
export const maintenanceEvents = new EventTarget();

export function MaintenanceProvider({ children }: { children: ReactNode }) {
  const [maintenance, setMaintenanceState] = useState<MaintenanceState>({
    active: false,
    message: "",
  });

  const setMaintenance = useCallback((state: MaintenanceState) => {
    setMaintenanceState(state);
  }, []);

  // Listen for maintenance events dispatched from apiFetch
  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<MaintenanceState>).detail;
      setMaintenanceState(detail);
    };
    maintenanceEvents.addEventListener("maintenance", handler);
    return () => maintenanceEvents.removeEventListener("maintenance", handler);
  }, []);

  return (
    <MaintenanceContext.Provider value={{ maintenance, setMaintenance }}>
      {children}
      {maintenance.active && <MaintenanceOverlay message={maintenance.message} />}
    </MaintenanceContext.Provider>
  );
}

export function useMaintenance() {
  return useContext(MaintenanceContext);
}

function MaintenanceOverlay({ message }: { message: string }) {
  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-gray-900/80 backdrop-blur-sm">
      <div className="mx-4 max-w-md rounded-xl bg-white p-8 text-center shadow-2xl">
        <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-amber-100">
          <svg
            className="h-8 w-8 text-amber-600 animate-pulse"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
        </div>
        <h2 className="mb-2 text-xl font-semibold text-gray-900">
          System Maintenance
        </h2>
        <p className="mb-4 text-sm text-gray-600">
          {message || "A maintenance operation is in progress. Please wait."}
        </p>
        <p className="text-xs text-gray-400">
          The page will reload automatically when maintenance is complete.
        </p>
        <div className="mt-4 flex justify-center">
          <div className="h-2 w-32 overflow-hidden rounded-full bg-gray-200">
            <div className="h-full w-full animate-indeterminate rounded-full bg-amber-500" />
          </div>
        </div>
      </div>
    </div>
  );
}
