// SPDX-License-Identifier: Apache-2.0

import type { ExportRequest, ExportJobResponse } from "../types";
import { apiFetch, buildUrl, ApiError, BASE } from "./client";

export async function createExport(
  body: ExportRequest,
): Promise<ExportJobResponse | null> {
  const url = buildUrl("/exports");
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (res.status === 200) {
    const disposition = res.headers.get("Content-Disposition") ?? "";
    const filenameMatch = disposition.match(/filename="?([^"]+)"?/);
    const filename =
      filenameMatch?.[1] ??
      `export.${body.format === "json" ? "json" : body.format === "chef_search_query" ? "txt" : "csv"}`;
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

  let code = res.status;
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
