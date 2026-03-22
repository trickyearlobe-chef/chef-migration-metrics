import { useState, useEffect, useCallback, useMemo } from "react";
import { useParams, Link } from "react-router-dom";
import { fetchNodeDisks } from "../api";
import type { NodeDiskDetailResponse, DiskEntry } from "../types";
import { LoadingSpinner, ErrorAlert, EmptyState } from "../components/Feedback";

// ---------------------------------------------------------------------------
// Node disk detail page — shows filesystem / disk data collected from Ohai.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

function formatKB(kb: number): string {
  if (kb <= 0) return "0";
  if (kb >= 1073741824) return `${(kb / 1073741824).toFixed(1)} TB`;
  if (kb >= 1048576) return `${(kb / 1048576).toFixed(1)} GB`;
  if (kb >= 1024) return `${(kb / 1024).toFixed(1)} MB`;
  return `${kb} KB`;
}

function percentBarColor(pct: number): string {
  if (pct >= 90) return "bg-red-500";
  if (pct >= 75) return "bg-amber-500";
  return "bg-green-500";
}

function percentTextColor(pct: number): string {
  if (pct >= 90) return "text-red-700";
  if (pct >= 75) return "text-amber-700";
  return "text-green-700";
}

/** Returns true when the entry looks like a Windows disk (has drive_type set). */
function hasWindowsFields(disks: DiskEntry[]): boolean {
  return disks.some((d) => d.drive_type != null && d.drive_type !== "");
}

/** Returns true when any disk entry has inode data. */
function hasInodeData(disks: DiskEntry[]): boolean {
  return disks.some(
    (d) =>
      d.total_inodes != null &&
      d.total_inodes > 0,
  );
}

/** Returns true when inodes are more than 30% used (i.e. less than 70% free). */
function inodePressure(disk: DiskEntry): boolean {
  if (disk.inodes_percent_used == null) return false;
  return disk.inodes_percent_used > 30;
}

// ---------------------------------------------------------------------------
// Sort types and helpers
// ---------------------------------------------------------------------------

type SortField =
  | "mount"
  | "device"
  | "fs_type"
  | "kb_size"
  | "kb_used"
  | "kb_available"
  | "percent_used"
  | "drive_type"
  | "encryption_status";

type SortOrder = "asc" | "desc";

function compareDiskEntries(a: DiskEntry, b: DiskEntry, field: SortField): number {
  switch (field) {
    case "mount":
      return a.mount.localeCompare(b.mount);
    case "device":
      return a.device.localeCompare(b.device);
    case "fs_type":
      return a.fs_type.localeCompare(b.fs_type);
    case "kb_size":
      return a.kb_size - b.kb_size;
    case "kb_used":
      return a.kb_used - b.kb_used;
    case "kb_available":
      return a.kb_available - b.kb_available;
    case "percent_used":
      return a.percent_used - b.percent_used;
    case "drive_type":
      return (a.drive_type ?? "").localeCompare(b.drive_type ?? "");
    case "encryption_status":
      return (a.encryption_status ?? "").localeCompare(b.encryption_status ?? "");
    default:
      return 0;
  }
}

// ---------------------------------------------------------------------------
// Sortable column header
// ---------------------------------------------------------------------------

function SortableColHeader({
  label,
  field,
  currentField,
  currentOrder,
  onSort,
  className,
}: {
  label: string;
  field: SortField;
  currentField: SortField;
  currentOrder: SortOrder;
  onSort: (field: SortField) => void;
  className?: string;
}) {
  const isActive = field === currentField;
  const arrow = !isActive ? " ↕" : currentOrder === "asc" ? " ↑" : " ↓";
  return (
    <th
      className={
        "cursor-pointer select-none hover:text-gray-700 " + (className ?? "")
      }
      onClick={() => onSort(field)}
    >
      {label}
      <span className="text-xs text-blue-500">{arrow}</span>
    </th>
  );
}

// ---------------------------------------------------------------------------
// Inode detail row (expandable)
// ---------------------------------------------------------------------------

function InodeRow({ disk }: { disk: DiskEntry }) {
  if (
    disk.total_inodes == null ||
    disk.total_inodes <= 0
  ) {
    return null;
  }

  const used = disk.inodes_used ?? 0;
  const available = disk.inodes_available ?? 0;
  const total = disk.total_inodes;
  const pct = disk.inodes_percent_used ?? 0;

  return (
    <div className="mt-1.5 rounded border border-gray-100 bg-gray-50 px-3 py-2 text-xs text-gray-600">
      <span className="font-medium text-gray-700">Inodes:</span>{" "}
      {used.toLocaleString()} used / {total.toLocaleString()} total
      {" · "}
      {available.toLocaleString()} available
      <div className="mt-1 flex items-center gap-2">
        <div className="h-1.5 w-24 overflow-hidden rounded-full bg-gray-200">
          <div
            className={`h-full rounded-full ${percentBarColor(pct)}`}
            style={{ width: `${Math.min(pct, 100)}%` }}
          />
        </div>
        <span className={`text-[10px] font-medium ${percentTextColor(pct)}`}>
          {pct}%
        </span>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Disk table row
// ---------------------------------------------------------------------------

function DiskRow({
  disk,
  showWindows,
  showInodes,
}: {
  disk: DiskEntry;
  showWindows: boolean;
  showInodes: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const canExpand = showInodes && disk.total_inodes != null && disk.total_inodes > 0;
  const pct = disk.percent_used;

  return (
    <>
      <tr
        className={canExpand ? "cursor-pointer hover:bg-gray-50" : ""}
        onClick={() => canExpand && setExpanded((prev) => !prev)}
      >
        <td className="font-mono">
          {canExpand && (
            <span className="mr-1 inline-block w-3 text-gray-400">
              {expanded ? "▾" : "▸"}
            </span>
          )}
          {disk.mount}
          {canExpand && inodePressure(disk) && (
            <span className="ml-1.5 text-amber-500" title="Free inodes below 70%">⚠</span>
          )}
        </td>
        <td className="font-mono">{disk.device}</td>
        <td>{disk.fs_type}</td>
        <td>{formatKB(disk.kb_size)}</td>
        <td>{formatKB(disk.kb_used)}</td>
        <td>{formatKB(disk.kb_available)}</td>
        <td>
          <div className="flex items-center gap-2">
            <div className="h-2 w-16 overflow-hidden rounded-full bg-gray-200">
              <div
                className={`h-full rounded-full transition-all ${percentBarColor(pct)}`}
                style={{ width: `${Math.min(pct, 100)}%` }}
              />
            </div>
            <span className={`text-xs font-medium ${percentTextColor(pct)}`}>
              {pct}%
            </span>
          </div>
        </td>
        {showWindows && (
          <>
            <td>{disk.drive_type ?? "—"}</td>
            <td>{disk.encryption_status ?? "—"}</td>
          </>
        )}
      </tr>
      {expanded && canExpand && (
        <tr>
          <td colSpan={showWindows ? 9 : 7} className="px-4 pb-3 pt-0">
            <InodeRow disk={disk} />
          </td>
        </tr>
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Main page component
// ---------------------------------------------------------------------------

export function NodeDiskDetailPage() {
  const { org, name } = useParams<{ org: string; name: string }>();
  const [data, setData] = useState<NodeDiskDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showAll, setShowAll] = useState(false);
  const [sortField, setSortField] = useState<SortField>("mount");
  const [sortOrder, setSortOrder] = useState<SortOrder>("asc");

  const load = useCallback(() => {
    if (!org || !name) return;
    setLoading(true);
    setError(null);
    fetchNodeDisks(org, name, showAll)
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [org, name, showAll]);

  useEffect(() => {
    load();
  }, [load]);

  const handleSort = useCallback(
    (field: SortField) => {
      if (field === sortField) {
        setSortOrder((prev) => (prev === "asc" ? "desc" : "asc"));
      } else {
        setSortField(field);
        setSortOrder("asc");
      }
    },
    [sortField],
  );

  const disks = useMemo(() => {
    if (!data) return [];
    const sorted = [...data.disks].sort((a, b) => {
      const cmp = compareDiskEntries(a, b, sortField);
      return sortOrder === "asc" ? cmp : -cmp;
    });
    return sorted;
  }, [data, sortField, sortOrder]);

  if (loading && !data) return <LoadingSpinner message="Loading filesystem data…" />;
  if (error && !data) return <ErrorAlert message={error} onRetry={load} />;
  if (!data) return null;

  const showWindows = hasWindowsFields(disks);
  const showInodes = hasInodeData(disks);

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <nav className="text-sm text-gray-500">
        <Link to="/nodes" className="hover:text-blue-600 hover:underline">
          Nodes
        </Link>
        <span className="mx-1">/</span>
        <Link
          to={`/nodes/${encodeURIComponent(org ?? "")}/${encodeURIComponent(name ?? "")}`}
          className="hover:text-blue-600 hover:underline"
        >
          {data.node_name}
        </Link>
        <span className="mx-1">/</span>
        <span className="text-gray-800">Disk Detail</span>
      </nav>

      {/* Header */}
      <div>
        <h2 className="text-xl font-bold text-gray-800">
          {data.node_name} — Filesystem
        </h2>
        {data.platform && (
          <p className="mt-1 text-sm text-gray-500">
            Platform: <span className="font-medium text-gray-700">{data.platform}</span>
          </p>
        )}
      </div>

      {/* Toolbar: virtual FS toggle */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <label className="inline-flex items-center gap-2 text-sm text-gray-600">
          <input
            type="checkbox"
            checked={showAll}
            onChange={(e) => setShowAll(e.target.checked)}
            className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
          />
          Show virtual / pseudo filesystems
        </label>

        {disks.length > 0 && (
          <span className="text-xs text-gray-400">
            {disks.length} filesystem{disks.length !== 1 ? "s" : ""}
          </span>
        )}
      </div>

      {/* Loading overlay for refresh */}
      {loading && data && <LoadingSpinner message="Refreshing…" />}

      {/* Error on refresh */}
      {error && data && <ErrorAlert message={error} onRetry={load} />}

      {/* Table */}
      {!loading && disks.length === 0 ? (
        <EmptyState
          title="No filesystems found"
          description={
            showAll
              ? "No filesystem data is available for this node."
              : "No real filesystems found. Try enabling virtual / pseudo filesystems."
          }
        />
      ) : (
        !loading && (
          <div className="card">
            <div className="card-header flex items-center justify-between">
              <span>Filesystems</span>
              {showInodes && (
                <span className="text-xs font-normal text-gray-400">
                  Click a row to expand inode details
                </span>
              )}
            </div>
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <SortableColHeader label="Mount Point" field="mount" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                    <SortableColHeader label="Device" field="device" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                    <SortableColHeader label="FS Type" field="fs_type" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                    <SortableColHeader label="Size" field="kb_size" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                    <SortableColHeader label="Used" field="kb_used" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                    <SortableColHeader label="Available" field="kb_available" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                    <SortableColHeader label="% Used" field="percent_used" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                    {showWindows && (
                      <>
                        <SortableColHeader label="Drive Type" field="drive_type" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                        <SortableColHeader label="Encryption" field="encryption_status" currentField={sortField} currentOrder={sortOrder} onSort={handleSort} />
                      </>
                    )}
                  </tr>
                </thead>
                <tbody>
                  {disks.map((d) => (
                    <DiskRow
                      key={`${d.device}-${d.mount}`}
                      disk={d}
                      showWindows={showWindows}
                      showInodes={showInodes}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )
      )}


    </div>
  );
}
