// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";
import { fetchTLSStatus } from "../api";
import type { TLSStatus } from "../types";

/**
 * Prominent global banner shown when the server fell back to a degraded TLS
 * listener at startup — an untrusted self-signed certificate, or plain HTTP as a
 * last resort. Polls GET /api/v1/server/tls-status — a public,
 * DB-independent endpoint — so the warning renders on every page. Renders nothing
 * when TLS is healthy or the poll fails.
 */
export function TLSDegradedBanner({ intervalMs = 30_000 }: { intervalMs?: number }) {
  const [status, setStatus] = useState<TLSStatus | null>(null);

  useEffect(() => {
    let active = true;
    const tick = async () => {
      try {
        const s = await fetchTLSStatus();
        if (active) setStatus(s);
      } catch {
        // Ignore poll errors — the next tick retries. Never hide the banner on
        // a transient failure once it is showing.
      }
    };
    tick();
    const id = setInterval(tick, intervalMs);
    return () => {
      active = false;
      clearInterval(id);
    };
  }, [intervalMs]);

  if (!status?.degraded) return null;

  // Plain HTTP (cleartext) is the last-resort fallback and is the more dangerous
  // state; self-signed keeps traffic encrypted but untrusted.
  const headline =
    status.kind === "plain"
      ? "TLS degraded — running INSECURE over plain HTTP."
      : "TLS degraded — serving an untrusted self-signed certificate.";

  return (
    <div
      role="alert"
      className="flex items-start gap-2 border-b border-red-700 bg-red-600 px-4 py-2.5 text-sm text-white"
    >
      <svg
        className="mt-0.5 h-5 w-5 shrink-0"
        fill="none"
        viewBox="0 0 24 24"
        strokeWidth={2}
        stroke="currentColor"
        aria-hidden="true"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
        />
      </svg>
      <div className="min-w-0">
        <span className="font-semibold">{headline}</span>{" "}
        <span>Fix the certificate and restart.</span>
        {status.reason && (
          <span className="mt-0.5 block break-words font-mono text-xs text-red-100">
            {status.reason}
          </span>
        )}
      </div>
    </div>
  );
}
