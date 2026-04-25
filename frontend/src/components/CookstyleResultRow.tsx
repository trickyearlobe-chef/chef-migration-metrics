// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";
import { Link } from "react-router-dom";
import { StatusBadge } from "./StatusBadge";
import type { CookstyleResult } from "../types/cookbooks";

interface CookstyleResultRowProps {
  result: CookstyleResult;
  /** Base URL for the remediation link. The component appends `?target_chef_version=...` */
  linkBase?: string;
}

export function CookstyleResultRow({ result, linkBase }: CookstyleResultRowProps) {
  const [stderrOpen, setStderrOpen] = useState(false);

  const isScanError = !!result.error_message;

  return (
    <div className="flex flex-wrap items-center gap-3 rounded-lg border border-gray-100 p-3">
      <span className="text-xs text-gray-500">
        Target: {result.target_chef_version}
      </span>

      {isScanError ? (
        <StatusBadge variant="scan_error" label="Scan Error" size="sm" />
      ) : result.passed ? (
        <StatusBadge variant="compatible" label="Passed" size="sm" />
      ) : (
        <StatusBadge variant="incompatible" label="Failed" size="sm" />
      )}

      {!isScanError && (
        <span className="text-xs text-gray-500">
          Offences: {result.offence_count} | Deprecations:{" "}
          {result.deprecation_count}
        </span>
      )}

      {isScanError && (
        <span className="text-xs text-orange-700">
          {result.error_message}
        </span>
      )}

      <span className="text-xs text-gray-400">
        Scanned: {new Date(result.scanned_at).toLocaleString()}
      </span>

      {isScanError && result.process_stderr && (
        <button
          type="button"
          onClick={() => setStderrOpen((prev) => !prev)}
          className="text-xs font-medium text-orange-600 hover:text-orange-800 hover:underline"
        >
          {stderrOpen ? "Hide details" : "Show details"}
        </button>
      )}

      {linkBase && result.target_chef_version && (
        <Link
          to={`${linkBase}?target_chef_version=${encodeURIComponent(result.target_chef_version)}`}
          className="ml-auto text-xs font-medium text-blue-600 hover:text-blue-800 hover:underline"
        >
          View Remediation Detail →
        </Link>
      )}

      {isScanError && stderrOpen && result.process_stderr && (
        <pre className="mt-1 w-full max-h-48 overflow-auto rounded bg-gray-900 p-3 font-mono text-xs text-gray-200">
          {result.process_stderr}
        </pre>
      )}
    </div>
  );
}
