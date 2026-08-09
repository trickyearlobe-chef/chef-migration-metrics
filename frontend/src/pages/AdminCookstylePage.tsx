// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useSearchParams } from "react-router-dom";
import { AdminCookstyleSettingsSection } from "./AdminCookstyleSettingsSection";
import { AdminCopClassificationsSection } from "./AdminCopClassificationsSection";
import { AdminCopInventorySection } from "./AdminCopInventorySection";
import { AdminCustomCopsSection } from "./AdminCustomCopsSection";
import { AdminScanScopeSection } from "./AdminScanScopeSection";

// The CookStyle admin hub. Five unrelated jobs had accumulated on one page,
// stacked behind horizontal rules: how scans run, what each finding means,
// which cops we have never classified, cops we wrote ourselves, and which files
// count as cookbook code. Each loads its own data and each is a separate sitting
// of work, so each is a tab.
type CookstyleTab =
  | "settings"
  | "classifications"
  | "inventory"
  | "custom"
  | "scope";

const TABS: { key: CookstyleTab; label: string }[] = [
  { key: "settings", label: "Settings" },
  { key: "classifications", label: "Classifications" },
  { key: "inventory", label: "Cop inventory" },
  { key: "custom", label: "Custom cops" },
  { key: "scope", label: "Scan scope" },
];

function isValidTab(value: string | null): value is CookstyleTab {
  return TABS.some((t) => t.key === value);
}

export function AdminCookstylePage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get("tab");
  const activeTab: CookstyleTab = isValidTab(rawTab) ? rawTab : "settings";

  const switchTab = (tab: CookstyleTab) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      // The default tab leaves no param, so the plain URL is the plain page.
      if (tab === "settings") {
        next.delete("tab");
      } else {
        next.set("tab", tab);
      }
      return next;
    });
  };

  return (
    <div className="space-y-6">
      <div className="max-w-3xl">
        <h2 className="text-xl font-semibold text-gray-900">CookStyle</h2>
        <p className="mt-1 text-sm text-gray-500">
          CookStyle reads cookbook code and reports what it finds. These screens
          control how it runs, and what its findings are taken to mean.
        </p>
      </div>

      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="CookStyle tabs">
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

      {activeTab === "settings" && <AdminCookstyleSettingsSection />}
      {activeTab === "classifications" && <AdminCopClassificationsSection />}
      {activeTab === "inventory" && <AdminCopInventorySection />}
      {activeTab === "custom" && <AdminCustomCopsSection />}
      {activeTab === "scope" && (
        <section>
          <h3 className="mb-3 text-lg font-semibold text-gray-900">
            What counts as cookbook code
          </h3>
          <AdminScanScopeSection />
        </section>
      )}
    </div>
  );
}
