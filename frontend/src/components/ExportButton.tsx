import { useState, useEffect, useCallback, useRef } from "react";
import {
  createExport,
  fetchExportStatus,
  downloadExportUrl,
} from "../api";
import type {
  ExportType,
  ExportFormat,
  ExportFilters,
  ExportJobResponse,
} from "../types";

// ---------------------------------------------------------------------------
// ExportButton — a self-contained button that triggers a data export.
//
// For small result sets the backend responds synchronously (HTTP 200) and
// the browser downloads the file immediately. For large result sets the
// backend responds with HTTP 202 and a job ID — the component polls for
// completion and then triggers the download.
// ---------------------------------------------------------------------------

export interface ExportButtonProps {
  /** The type of data to export. */
  exportType: ExportType;

  /** Available formats for this export type. Defaults to ["csv", "json"]. */
  formats?: ExportFormat[];

  /** Target Chef version — required for ready_nodes and blocked_nodes. */
  targetChefVersion?: string;

  /** Active filters to pass through to the export. */
  filters?: ExportFilters;

  /** Optional CSS class name for the wrapper. */
  className?: string;

  /** Optional label override. Defaults to "Export". */
  label?: string;
}

type Phase =
  | "idle"
  | "selecting"   // format picker is open
  | "exporting"   // waiting for sync response or async job creation
  | "polling"     // async job created, polling for completion
  | "done"        // download triggered
  | "error";

const POLL_INTERVAL_MS = 2000;
const MAX_POLLS = 150; // 5 minutes at 2s intervals

const FORMAT_LABELS: Record<ExportFormat, string> = {
  csv: "CSV",
  json: "JSON",
  chef_search_query: "Chef Search Query",
};

export function ExportButton({
  exportType,
  formats = ["csv", "json"],
  targetChefVersion,
  filters = {},
  className = "",
  label = "Export",
}: ExportButtonProps) {
  const [phase, setPhase] = useState<Phase>("idle");
  const [error, setError] = useState<string | null>(null);
  const [_jobId, setJobId] = useState<string | null>(null);
  const [jobStatus, setJobStatus] = useState<string | null>(null);
  const pollCountRef = useRef(0);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // Clean up poll timer on unmount.
  useEffect(() => {
    return () => {
      if (pollTimerRef.current) clearInterval(pollTimerRef.current);
    };
  }, []);

  // Close menu on outside click.
  useEffect(() => {
    if (phase !== "selecting") return;
    const handler = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setPhase("idle");
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [phase]);

  const startPolling = useCallback(
    (id: string) => {
      pollCountRef.current = 0;
      setJobId(id);
      setJobStatus("pending");
      setPhase("polling");

      pollTimerRef.current = setInterval(async () => {
        pollCountRef.current += 1;
        if (pollCountRef.current > MAX_POLLS) {
          if (pollTimerRef.current) clearInterval(pollTimerRef.current);
          setError("Export timed out. Check the export job status later.");
          setPhase("error");
          return;
        }

        try {
          const status: ExportJobResponse = await fetchExportStatus(id);
          setJobStatus(status.status);

          if (status.status === "completed" && status.download_url) {
            if (pollTimerRef.current) clearInterval(pollTimerRef.current);
            // Trigger download.
            const a = document.createElement("a");
            a.href = downloadExportUrl(id);
            a.download = "";
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            setPhase("done");
            // Reset to idle after a brief moment.
            setTimeout(() => {
              setPhase("idle");
              setJobId(null);
              setJobStatus(null);
            }, 3000);
          } else if (status.status === "failed") {
            if (pollTimerRef.current) clearInterval(pollTimerRef.current);
            setError(status.error_message ?? "Export failed.");
            setPhase("error");
          }
        } catch (e) {
          // Transient poll failure — keep trying until MAX_POLLS.
        }
      }, POLL_INTERVAL_MS);
    },
    [],
  );

  const handleExport = useCallback(
    async (format: ExportFormat) => {
      setPhase("exporting");
      setError(null);

      try {
        const result = await createExport({
          export_type: exportType,
          format,
          target_chef_version: targetChefVersion,
          filters,
        });

        if (result === null) {
          // Synchronous — file already downloading.
          setPhase("done");
          setTimeout(() => setPhase("idle"), 3000);
        } else {
          // Asynchronous — start polling.
          startPolling(result.job_id);
        }
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e);
        setError(msg);
        setPhase("error");
      }
    },
    [exportType, targetChefVersion, filters, startPolling],
  );

  const handleButtonClick = () => {
    if (phase === "error") {
      // Reset on error click.
      setPhase("idle");
      setError(null);
      return;
    }
    if (phase !== "idle") return;

    if (formats.length === 1) {
      // Skip format selection if only one format.
      handleExport(formats[0]);
    } else {
      setPhase("selecting");
    }
  };

  // Determine button label and style based on phase.
  let buttonLabel = label;
  let buttonColor = "bg-blue-600 hover:bg-blue-700 text-white";
  let spinner = false;

  switch (phase) {
    case "selecting":
      buttonLabel = "Select format…";
      break;
    case "exporting":
      buttonLabel = "Generating…";
      spinner = true;
      buttonColor = "bg-blue-500 text-white cursor-wait";
      break;
    case "polling":
      buttonLabel = jobStatus === "processing" ? "Processing…" : "Queued…";
      spinner = true;
      buttonColor = "bg-yellow-500 text-white cursor-wait";
      break;
    case "done":
      buttonLabel = "✓ Downloaded";
      buttonColor = "bg-green-600 text-white";
      break;
    case "error":
      buttonLabel = "✕ Failed";
      buttonColor = "bg-red-600 hover:bg-red-700 text-white";
      break;
  }

  return (
    <div className={`relative inline-block ${className}`} ref={menuRef}>
      <button
        onClick={handleButtonClick}
        disabled={phase === "exporting" || phase === "polling"}
        className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium shadow-sm transition-colors ${buttonColor} disabled:opacity-70 disabled:cursor-not-allowed`}
        title={error ?? undefined}
      >
        {spinner && (
          <svg
            className="h-4 w-4 animate-spin"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              className="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="4"
            />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
        )}
        {buttonLabel}
      </button>

      {/* Format picker dropdown */}
      {phase === "selecting" && (
        <div className="absolute right-0 z-10 mt-1 w-48 origin-top-right rounded-md bg-white shadow-lg ring-1 ring-black/5">
          <div className="py-1">
            {formats.map((fmt) => (
              <button
                key={fmt}
                onClick={() => handleExport(fmt)}
                className="block w-full px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100"
              >
                {FORMAT_LABELS[fmt] ?? fmt}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Error tooltip */}
      {phase === "error" && error && (
        <div className="absolute right-0 z-10 mt-1 w-64 rounded-md bg-red-50 p-2 text-xs text-red-700 shadow-lg ring-1 ring-red-200">
          {error}
        </div>
      )}
    </div>
  );
}
