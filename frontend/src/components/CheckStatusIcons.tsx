// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from "react";

type DiskStatus = "sufficient" | "insufficient" | "unknown";
type CookstyleStatus =
  | "passed"
  | "failed"
  | "warnings"
  | "scan_error"
  | "unknown";
type KitchenStatus =
  | "passed"
  | "failed"
  | "partial"
  | "scan_error"
  | "unknown";
type CheckStatus = DiskStatus | CookstyleStatus | KitchenStatus;

interface CheckStatusIconsProps {
  diskStatus: DiskStatus;
  cookstyleStatus: CookstyleStatus;
  kitchenStatus: KitchenStatus;
  diskDetail: string | null;
  cookstyleDetail: string | null;
  kitchenDetail: string | null;
}

interface CheckStatusIconProps {
  status: CheckStatus;
  icon: ReactNode;
  tooltip: string;
  ariaLabel: string;
}

function statusColor(status: CheckStatus): string {
  switch (status) {
    case "sufficient":
    case "passed":
      return "text-green-600";
    case "insufficient":
    case "failed":
      return "text-red-600";
    case "warnings":
    case "partial":
      return "text-amber-500";
    case "scan_error":
      return "text-orange-500";
    case "unknown":
    default:
      return "text-gray-400";
  }
}

function statusOverlay(status: CheckStatus): string {
  switch (status) {
    case "sufficient":
    case "passed":
      return "✓";
    case "insufficient":
    case "failed":
      return "✗";
    case "warnings":
    case "partial":
      return "~";
    case "scan_error":
      return "!";
    case "unknown":
    default:
      return "?";
  }
}

function DiskIcon() {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
    >
      <rect x="2" y="3" width="12" height="10" rx="1" />
      <line x1="2" y1="10" x2="14" y2="10" />
      <circle cx="11" cy="12" r="0.75" fill="currentColor" />
    </svg>
  );
}

function CodeIcon() {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
    >
      <polyline points="5,4 1,8 5,12" />
      <polyline points="11,4 15,8 11,12" />
      <line x1="9" y1="3" x2="7" y2="13" />
    </svg>
  );
}

function FlaskIcon() {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
    >
      <path d="M6 2 L6 6 L2 14 L14 14 L10 6 L10 2" />
      <line x1="5" y1="2" x2="11" y2="2" />
      <line x1="4" y1="11" x2="12" y2="11" strokeDasharray="1 1.5" />
    </svg>
  );
}

function CheckStatusIcon({ status, icon, tooltip, ariaLabel }: CheckStatusIconProps) {
  return (
    <span
      className={`relative inline-flex items-center justify-center ${statusColor(status)}`}
      title={tooltip}
      role="img"
      aria-label={ariaLabel}
    >
      {icon}
      <span className="absolute -right-0.5 -top-0.5 text-[8px] font-bold leading-none">
        {statusOverlay(status)}
      </span>
    </span>
  );
}

export function CheckStatusIcons({
  diskStatus,
  cookstyleStatus,
  kitchenStatus,
  diskDetail,
  cookstyleDetail,
  kitchenDetail,
}: CheckStatusIconsProps) {
  return (
    <div className="inline-flex items-center gap-1">
      <CheckStatusIcon
        status={diskStatus}
        icon={<DiskIcon />}
        tooltip={diskDetail ?? "Disk: unknown"}
        ariaLabel={`Disk space: ${diskStatus}${diskDetail ? ` — ${diskDetail}` : ""}`}
      />
      <CheckStatusIcon
        status={cookstyleStatus}
        icon={<CodeIcon />}
        tooltip={cookstyleDetail ?? "CookStyle: unknown"}
        ariaLabel={`CookStyle: ${cookstyleStatus}${cookstyleDetail ? ` — ${cookstyleDetail}` : ""}`}
      />
      <CheckStatusIcon
        status={kitchenStatus}
        icon={<FlaskIcon />}
        tooltip={kitchenDetail ?? "Test Kitchen: unknown"}
        ariaLabel={`Test Kitchen: ${kitchenStatus}${kitchenDetail ? ` — ${kitchenDetail}` : ""}`}
      />
    </div>
  );
}

export type {
  DiskStatus,
  CookstyleStatus,
  KitchenStatus,
  CheckStatus,
  CheckStatusIconsProps,
};
