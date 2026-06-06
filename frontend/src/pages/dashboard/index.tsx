import { useSearchParams } from "react-router-dom";
import { useOrg } from "../../context/OrgContext";
import {
  VersionDistributionCard,
  PlatformDistributionCard,
  ReadinessCard,
  CookbookCompatibilityCard,
  GitRepoCompatibilityCard,
  TestKitchenCompatibilityCard,
} from "./StatusCards";
import {
  VersionDistributionTrendCard,
  ReadinessTrendCard,
  ComplexityTrendCard,
  StaleTrendCard,
  DeploymentTrendCard,
} from "./TrendCards";
import { DeploymentStatusCard } from "./DeploymentCards";

// ---------------------------------------------------------------------------
// Dashboard page — three tabs:
//   "Current Status" — point-in-time summary cards
//   "Trends"         — historical trend charts
//   "Deployment"     — per-version deployment progress
//
// The active tab is persisted in the URL via ?tab=status|trends|deployment so
// that bookmarks and shared links preserve the view.
// ---------------------------------------------------------------------------

type DashboardTab = "status" | "trends" | "deployment";

const TABS: { key: DashboardTab; label: string }[] = [
  { key: "status", label: "Current Status" },
  { key: "trends", label: "Trends" },
  { key: "deployment", label: "Deployment" },
];

function isValidTab(value: string | null): value is DashboardTab {
  return value === "status" || value === "trends" || value === "deployment";
}

export function DashboardPage() {
  const { selectedOrg } = useOrg();
  const org = selectedOrg || undefined;

  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get("tab");
  const activeTab: DashboardTab = isValidTab(rawTab) ? rawTab : "status";

  const switchTab = (tab: DashboardTab) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (tab === "status") {
        next.delete("tab");
      } else {
        next.set("tab", tab);
      }
      return next;
    });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-gray-800">Dashboard</h2>
      </div>

      {/* ---- Tab bar ---- */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Dashboard tabs">
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

      {/* ---- Current Status tab ---- */}
      {activeTab === "status" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <VersionDistributionCard organisation={org} />
          <PlatformDistributionCard organisation={org} />
          <ReadinessCard organisation={org} />
          <CookbookCompatibilityCard organisation={org} />
          <GitRepoCompatibilityCard organisation={org} />
          <TestKitchenCompatibilityCard organisation={org} />
        </div>
      )}

      {/* ---- Trends tab ---- */}
      {activeTab === "trends" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <VersionDistributionTrendCard organisation={org} />
          <ReadinessTrendCard organisation={org} />
          <ComplexityTrendCard organisation={org} />
          <StaleTrendCard organisation={org} />
        </div>
      )}

      {/* ---- Deployment tab ---- */}
      {activeTab === "deployment" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <DeploymentStatusCard organisation={org} />
          <DeploymentTrendCard organisation={org} />
        </div>
      )}
    </div>
  );
}
