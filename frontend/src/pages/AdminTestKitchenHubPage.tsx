// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useSearchParams } from "react-router-dom";
import { AdminTestKitchenPage } from "./AdminTestKitchenPage";
import { AdminKitchenAnalysisPage } from "./AdminKitchenAnalysisPage";
import KitchenBatchesPage from "./KitchenBatchesPage";
import KitchenQueuePage from "./KitchenQueuePage";
import { AdminConcurrencyPage } from "./AdminConcurrencyPage";
import { AdminAnalysisToolsPage } from "./AdminAnalysisToolsPage";
import { AdminCookstylePage } from "./AdminCookstylePage";

type KitchenTab = "config" | "cookstyle" | "analysis" | "batches" | "queue" | "settings";

const TABS: { key: KitchenTab; label: string }[] = [
  { key: "config", label: "Hypervisor" },
  { key: "cookstyle", label: "CookStyle" },
  { key: "analysis", label: "Analysis" },
  { key: "batches", label: "Batches" },
  { key: "queue", label: "Queue" },
  { key: "settings", label: "Settings" },
];

function isValidTab(value: string | null): value is KitchenTab {
  return ["config", "cookstyle", "analysis", "batches", "queue", "settings"].includes(
    value ?? "",
  );
}

export function AdminTestKitchenHubPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get("tab");
  const activeTab: KitchenTab = isValidTab(rawTab) ? rawTab : "config";

  const switchTab = (tab: KitchenTab) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (tab === "config") {
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
        <nav className="-mb-px flex gap-6" aria-label="Test Kitchen tabs">
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

      {activeTab === "config" && <AdminTestKitchenPage />}
      {activeTab === "cookstyle" && <AdminCookstylePage />}
      {activeTab === "analysis" && <AdminKitchenAnalysisPage />}
      {activeTab === "batches" && <KitchenBatchesPage />}
      {activeTab === "queue" && <KitchenQueuePage />}
      {activeTab === "settings" && (
        <div className="space-y-8">
          <AdminConcurrencyPage />
          <AdminAnalysisToolsPage />
        </div>
      )}
    </div>
  );
}
