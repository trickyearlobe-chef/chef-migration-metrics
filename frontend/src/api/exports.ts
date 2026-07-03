// SPDX-License-Identifier: Apache-2.0

import type {
  ExportType,
  ExportFormat,
  ExportParams,
  ExportJobResponse,
} from "../types";
import { apiFetch, buildUrl, ApiError, BASE } from "./client";

// createExport posts an export request. The list view's own query params are
// sent verbatim (minus pagination) alongside export_type and format, so the
// export reproduces the current filtered list. A 200 streams the file inline
// (returns null after triggering the download); a 202 returns the async job.
export async function createExport(
  exportType: ExportType,
  format: ExportFormat,
  params: ExportParams,
): Promise<ExportJobResponse | null> {
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === "") continue;
    // Pagination is meaningless for a full-list export.
    if (key === "page" || key === "per_page") continue;
    qs.set(key, String(value));
  }
  qs.set("export_type", exportType);
  qs.set("format", format);

  const url = buildUrl(`/exports?${qs.toString()}`);
  const res = await fetch(url, {
    method: "POST",
    headers: { Accept: "application/json" },
  });

  if (res.status === 200) {
    const disposition = res.headers.get("Content-Disposition") ?? "";
    const filenameMatch = disposition.match(/filename="?([^"]+)"?/);
    const filename =
      filenameMatch?.[1] ??
      `export.${format === "json" ? "json" : format === "chef_search_query" ? "txt" : "csv"}`;
    const blob = await res.blob();
    const blobUrl = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = blobUrl;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(blobUrl);
    return null;
  }

  if (res.status === 202) {
    return res.json() as Promise<ExportJobResponse>;
  }

  const code = res.status;
  let message = res.statusText || `HTTP ${res.status}`;
  try {
    const errBody = await res.text();
    try {
      const parsed = JSON.parse(errBody);
      message = parsed.message || parsed.error || message;
    } catch {
      /* ignore */
    }
    throw new ApiError(code, message, errBody);
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError(code, message, "");
  }
}

export function fetchExportStatus(jobId: string): Promise<ExportJobResponse> {
  return apiFetch<ExportJobResponse>(
    buildUrl(`/exports/${encodeURIComponent(jobId)}`),
  );
}

export function downloadExportUrl(jobId: string): string {
  return `${BASE}/exports/${encodeURIComponent(jobId)}/download`;
}
