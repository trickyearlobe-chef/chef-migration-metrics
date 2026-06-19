// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useSearchParams } from "react-router-dom";
import { AdminSystemStatsPage } from "./AdminSystemStatsPage";
import { AdminPerformancePage } from "./AdminPerformancePage";
import { AdminStatusPage } from "./AdminStatusPage";

type HealthTab = "overview" | "performance" | "status";

const TABS: { key: HealthTab; label: string }[] = [
  { key: "overview", label: "Overview" },
  { key: "performance", label: "Performance" },
  { key: "status", label: "Status" },
];

function isValidTab(value: string | null): value is HealthTab {
  return value === "overview" || value === "performance" || value === "status";
}

export function AdminSystemHealthPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get("tab");
  const activeTab: HealthTab = isValidTab(rawTab) ? rawTab : "overview";

  const switchTab = (tab: HealthTab) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (tab === "overview") {
        next.delete("tab");
      } else {
        next.set("tab", tab);
      }
      return next;
    });
  };

  return (
    <div className="space-y-6">
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="System health tabs">
          {TABS.map((t) => (
            <button
              key={t.key}
              onClick={() => switchTab(t.key)}
              className={
                "whitespace-nowrap border-b-2 px-1 pb-3 text-sm font-medium transition-colors " +
                (activeTab === t.key
                  ? "border-blue-500 text-blue-600"
                  : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700")
              }
            >
              {t.label}
            </button>
          ))}
        </nav>
      </div>

      {activeTab === "overview" && <AdminSystemStatsPage />}
      {activeTab === "performance" && <AdminPerformancePage />}
      {activeTab === "status" && <AdminStatusPage />}
    </div>
  );
}
