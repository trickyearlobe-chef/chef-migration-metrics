import { Link, useSearchParams } from "react-router-dom";
import { ErrorAlert } from "../components/Feedback";
import { useAuth } from "../context/AuthContext";
import { OwnershipMappedImport } from "./OwnershipMappedImport";
import { ScheduledImports } from "../components/ScheduledImports";
import { ImportRejections } from "../components/ImportRejections";

// ---------------------------------------------------------------------------
// Ownership Import page — bulk import ownership assignments. Requires the
// admin role.
//
// One way in: read the source, map its columns, preview, then commit. A second
// tab used to take a file already in CMM's column order, which no source has
// ever supplied — every export arrives in somebody else's shape, so choosing
// the columns is the job rather than a fallback for awkward cases.
// ---------------------------------------------------------------------------

type ImportTab = "mapped" | "scheduled" | "rejections";

const TABS: { key: ImportTab; label: string }[] = [
  { key: "mapped", label: "File or database" },
  // Saved database imports: run one now, see what the schedules are doing,
  // throw away what an import brought in, and take the reports for whoever
  // maintains the source. Its own tab because a schedule nobody can see the
  // state of is a schedule nobody can trust.
  { key: "scheduled", label: "Saved imports" },
  // The rows an import could not use. Named for what it holds rather than for
  // the mechanism, because the person who needs it is looking for their data.
  { key: "rejections", label: "Rows not imported" },
];

export function OwnershipImportPage() {
  const { user } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();

  const rawTab = searchParams.get("tab");
  const activeTab: ImportTab = TABS.some((t) => t.key === rawTab)
    ? (rawTab as ImportTab)
    : "mapped";

  function switchTab(tab: ImportTab) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (tab === "mapped") {
        next.delete("tab");
      } else {
        next.set("tab", tab);
      }
      return next;
    });
  }

  // Role gate — importing owners is an administrator function. It used to
  // admit operators; that was narrowed on 2026-08-06 at the product owner's
  // instruction, and the route guard and the API were narrowed with it.
  const allowed = user?.role === "admin";

  if (!allowed) {
    return (
      <div className="space-y-6">
        <nav className="text-sm text-gray-500">
          <Link to="/ownership" className="hover:text-blue-600 hover:underline">Ownership</Link>
          <span className="mx-1">/</span>
          <span className="text-gray-800">Import</span>
        </nav>
        <ErrorAlert message="Access denied. The admin role is required to import ownership data." />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <Link to="/ownership" className="hover:text-blue-600 hover:underline">Ownership</Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">Import</span>
      </nav>

      {/* Header */}
      <div>
        <h2 className="text-lg font-semibold text-gray-800">
          Import Ownership Data
        </h2>
        <p className="text-sm text-gray-500">
          Bring existing ownership data into CMM from a file, or from a database.
        </p>
      </div>

      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Import tabs">
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

      {activeTab === "mapped" && <OwnershipMappedImport />}
      {activeTab === "scheduled" && <ScheduledImports />}
      {activeTab === "rejections" && <ImportRejections />}
    </div>
  );
}
