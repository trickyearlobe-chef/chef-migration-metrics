import type { CompatibilityStatus, ComplexityLabel, ComplexityBreakdown } from "../types";

// ---------------------------------------------------------------------------
// StatusBadge — renders a colour-coded pill for compatibility status,
// complexity labels, and boolean ready/blocked/stale states.
//
// Colour mapping follows the spec's confidence indicators:
//   Compatible (TK pass)  → green  (high confidence)
//   CookStyle only        → amber  (medium confidence)
//   Incompatible          → red
//   Untested              → grey
//
// Complexity labels:
//   low      → green
//   medium   → amber
//   high     → red
//   critical → red (darker)
//
// Boolean states:
//   ready    → green
//   blocked  → red
//   stale    → purple
//   fresh    → green
//   active   → green
//   inactive → grey
// ---------------------------------------------------------------------------

type BadgeVariant =
  | CompatibilityStatus
  | ComplexityLabel
  | "ready"
  | "needs_review"
  | "blocked"
  | "stale"
  | "fresh"
  | "warning"
  | "active"
  | "inactive"
  | "healthy"
  | "unhealthy"
  | "unknown"
  | "scan_error"
  | "cs_compatible"
  | "cs_incompatible"
  | "cs_untested"
  | "cs_ready"
  | "cs_needs_review"
  | "cs_blocked"
  | "tk_passed"
  | "tk_failed"
  | "tk_partial"
  | "tk_untested"
  | "disk_sufficient"
  | "disk_insufficient"
  | "disk_unknown";

interface StatusBadgeProps {
  /** The status variant to display. Determines colour and default label. */
  variant: BadgeVariant;
  /** Optional label override. If not provided, a human-readable label is derived from the variant. */
  label?: string;
  /** Optional additional CSS classes. */
  className?: string;
  /** Render as a smaller inline badge (useful in tables). */
  size?: "sm" | "md";
}

const variantStyles: Record<BadgeVariant, string> = {
  // Compatibility statuses (spec § Confidence Indicators)
  compatible: "bg-green-100 text-green-800 ring-green-600/20",
  cookstyle_only: "bg-amber-100 text-amber-800 ring-amber-600/20",
  incompatible: "bg-red-100 text-red-800 ring-red-600/20",
  untested: "bg-gray-100 text-gray-600 ring-gray-500/20",

  // Complexity labels
  low: "bg-green-100 text-green-800 ring-green-600/20",
  medium: "bg-amber-100 text-amber-800 ring-amber-600/20",
  high: "bg-red-100 text-red-800 ring-red-600/20",
  critical: "bg-red-200 text-red-900 ring-red-700/20",

  // Boolean / readiness states
  ready: "bg-green-100 text-green-800 ring-green-600/20",
  needs_review: "bg-amber-100 text-amber-800 ring-amber-600/20",
  blocked: "bg-red-100 text-red-800 ring-red-600/20",
  stale: "bg-purple-100 text-purple-800 ring-purple-600/20",
  fresh: "bg-green-100 text-green-800 ring-green-600/20",
  warning: "bg-amber-100 text-amber-800 ring-amber-600/20",
  active: "bg-green-100 text-green-800 ring-green-600/20",
  inactive: "bg-gray-100 text-gray-600 ring-gray-500/20",

  // Health
  healthy: "bg-green-100 text-green-800 ring-green-600/20",
  unhealthy: "bg-red-100 text-red-800 ring-red-600/20",
  unknown: "bg-gray-100 text-gray-600 ring-gray-500/20",
  scan_error: "bg-orange-100 text-orange-800 ring-orange-600/20",

  // CookStyle signal (low confidence — static analysis)
  cs_compatible: "bg-green-100 text-green-800 ring-green-600/20",
  cs_incompatible: "bg-red-100 text-red-800 ring-red-600/20",
  cs_untested: "bg-gray-100 text-gray-600 ring-gray-500/20",

  // CookStyle rollup status (SoT 4-state: Ready/Needs review/Blocked/Untested)
  cs_ready: "bg-green-100 text-green-800 ring-green-600/20",
  cs_needs_review: "bg-amber-100 text-amber-800 ring-amber-600/20",
  cs_blocked: "bg-red-100 text-red-800 ring-red-600/20",

  // Test Kitchen signal (high confidence for passes)
  tk_passed: "bg-green-100 text-green-800 ring-green-600/20",
  tk_failed: "bg-red-100 text-red-800 ring-red-600/20",
  tk_partial: "bg-orange-100 text-orange-800 ring-orange-600/20",
  tk_untested: "bg-gray-100 text-gray-600 ring-gray-500/20",

  // Disk space signal
  disk_sufficient: "bg-green-100 text-green-800 ring-green-600/20",
  disk_insufficient: "bg-red-100 text-red-800 ring-red-600/20",
  disk_unknown: "bg-gray-100 text-gray-600 ring-gray-500/20",
};

const variantLabels: Record<BadgeVariant, string> = {
  compatible: "Compatible",
  cookstyle_only: "CookStyle Only",
  incompatible: "Incompatible",
  untested: "Untested",
  low: "Low",
  medium: "Medium",
  high: "High",
  critical: "Critical",
  ready: "Ready",
  needs_review: "Needs review",
  blocked: "Blocked",
  stale: "Stale",
  fresh: "Fresh",
  warning: "Warning",
  active: "Active",
  inactive: "Inactive",
  healthy: "Healthy",
  unhealthy: "Unhealthy",
  unknown: "Unknown",
  scan_error: "Scan Error",
  cs_compatible: "CS ✓",
  cs_incompatible: "CS ✗",
  cs_untested: "CS ?",
  cs_ready: "Ready",
  cs_needs_review: "Needs review",
  cs_blocked: "Blocked",
  tk_passed: "TK ✓",
  tk_failed: "TK ✗",
  tk_partial: "TK ~",
  tk_untested: "TK ?",
  disk_sufficient: "Disk ✓",
  disk_insufficient: "Disk ✗",
  disk_unknown: "Disk ?",
};

/** Short descriptor shown as a tooltip on hover for compatibility statuses. */
const variantTooltips: Partial<Record<BadgeVariant, string>> = {
  compatible: "Full integration test (Test Kitchen) passed — high confidence",
  cookstyle_only:
    "Static analysis only (CookStyle) — no integration test. Medium confidence.",
  incompatible: "Known to be incompatible with the target Chef version",
  untested: "No test or scan results available yet",
  stale: "Last check-in exceeds the configured stale threshold",
  warning: "Node has not checked in recently — may be transient",
  critical:
    "Critical remediation complexity — significant manual effort required",
  scan_error:
    "CookStyle crashed before completing the scan — check the error details",
  cs_compatible: "CookStyle (static analysis) passed — low confidence, linting only",
  cs_incompatible: "CookStyle found deprecated API usage for the target Chef version",
  cs_untested: "No CookStyle scan results available",
  cs_ready: "CookStyle: no blockers or review-level cops — ready",
  cs_needs_review: "CookStyle: review-level cops present — needs a human look, not blocking",
  cs_blocked: "CookStyle: a blocker cop is present",
  tk_passed: "Test Kitchen converge + verify passed — high confidence",
  tk_failed: "Test Kitchen failed — may be a real issue or infrastructure noise",
  tk_partial: "Test Kitchen: some platforms passed, some failed",
  tk_untested: "No Test Kitchen results available",
  disk_sufficient: "Sufficient disk space for upgrade",
  disk_insufficient: "Insufficient disk space — upgrade may fail",
  disk_unknown: "Disk space not yet checked",
};

/**
 * Renders a colour-coded status pill / badge.
 *
 * Usage:
 * ```tsx
 * <StatusBadge variant="compatible" />
 * <StatusBadge variant="cookstyle_only" label="CookStyle ⚠" />
 * <StatusBadge variant="stale" size="sm" />
 * <StatusBadge variant="high" label="High (42)" />
 * ```
 */
export function StatusBadge({
  variant,
  label,
  className = "",
  size = "md",
}: StatusBadgeProps) {
  const style = variantStyles[variant] ?? variantStyles.unknown;
  const displayLabel = label ?? variantLabels[variant] ?? variant;
  const tooltip = variantTooltips[variant];

  const sizeClasses =
    size === "sm"
      ? "px-1.5 py-0.5 text-[10px] leading-tight"
      : "px-2.5 py-0.5 text-xs";

  return (
    <span
      className={[
        "inline-flex items-center gap-1 rounded-full font-medium ring-1 ring-inset",
        sizeClasses,
        style,
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      title={tooltip}
    >
      {/* Dot indicator for compatibility and CS/TK signal badges */}
      {(variant === "compatible" ||
        variant === "cookstyle_only" ||
        variant === "incompatible" ||
        variant === "untested" ||
        variant === "cs_compatible" ||
        variant === "cs_incompatible" ||
        variant === "cs_untested" ||
        variant === "cs_ready" ||
        variant === "cs_needs_review" ||
        variant === "cs_blocked" ||
        variant === "tk_passed" ||
        variant === "tk_failed" ||
        variant === "tk_partial" ||
        variant === "tk_untested" ||
        variant === "disk_sufficient" ||
        variant === "disk_insufficient" ||
        variant === "disk_unknown") && (
        <span
          className={`inline-block h-1.5 w-1.5 shrink-0 rounded-full ${dotColor(variant)}`}
          aria-hidden="true"
        />
      )}
      {displayLabel}
    </span>
  );
}

/** Small filled dot colour for the leading indicator. */
function dotColor(variant: BadgeVariant): string {
  switch (variant) {
    case "compatible":
    case "cs_compatible":
    case "cs_ready":
    case "tk_passed":
    case "disk_sufficient":
      return "bg-green-500";
    case "cookstyle_only":
    case "cs_needs_review":
      return "bg-amber-500";
    case "incompatible":
    case "cs_incompatible":
    case "cs_blocked":
    case "tk_failed":
    case "disk_insufficient":
      return "bg-red-500";
    case "tk_partial":
      return "bg-orange-500";
    case "untested":
    case "cs_untested":
    case "tk_untested":
    case "disk_unknown":
      return "bg-gray-400";
    default:
      return "bg-gray-400";
  }
}

// ---------------------------------------------------------------------------
// Convenience wrappers for common use-cases
// ---------------------------------------------------------------------------

/** Renders the appropriate compatibility badge for a given status string. */
export function CompatibilityBadge({
  status,
  confidence: _confidence,
  size = "md",
}: {
  status: string;
  confidence?: string | null;
  size?: "sm" | "md";
}) {
  // Normalise the status to our variant type.
  let variant: BadgeVariant;
  let label: string | undefined;

  switch (status) {
    case "compatible":
      variant = "compatible";
      label = "Compatible";
      break;
    case "cookstyle_only":
      variant = "cookstyle_only";
      label = "CookStyle Only";
      break;
    case "incompatible":
      variant = "incompatible";
      break;
    case "scan_error":
      variant = "scan_error";
      break;
    default:
      variant = "untested";
      break;
  }

  return <StatusBadge variant={variant} label={label} size={size} />;
}

/** Renders a complexity label badge. */
export function ComplexityBadge({
  complexityLabel,
  score,
  size = "md",
}: {
  complexityLabel: string;
  score?: number;
  size?: "sm" | "md";
}) {
  const variant = (
    ["low", "medium", "high", "critical"].includes(complexityLabel)
      ? complexityLabel
      : "unknown"
  ) as BadgeVariant;

  const label =
    score != null
      ? `${variantLabels[variant] ?? complexityLabel} (${score})`
      : (variantLabels[variant] ?? complexityLabel);

  return <StatusBadge variant={variant} label={label} size={size} />;
}

// ---------------------------------------------------------------------------
// Complexity breakdown labels for display
// ---------------------------------------------------------------------------

const breakdownLabels: Record<string, string> = {
  error_fatal: "Fatal",
  deprecation: "Deprecations",
  correctness: "Correctness",
  manual_fix: "Manual Fix",
  modernize: "Modernize",
  tk_fail: "TK",
};

/**
 * Renders the complexity score as a formula showing contributing components.
 * Only non-zero components are displayed. Example:
 *   Correctness 9×3 + Manual Fix 9×4 = Critical (63)
 */
export function ComplexityBreakdownDisplay({
  breakdown,
  complexityLabel,
  score,
}: {
  breakdown: ComplexityBreakdown;
  complexityLabel: string;
  score: number;
}) {
  const entries = Object.entries(breakdown)
    .filter(([, item]) => item.subtotal > 0)
    .map(([key, item]) => {
      const label = breakdownLabels[key] ?? key;
      if (key === "tk_fail") {
        return `${label} ${item.status ?? "fail"} +${item.subtotal}`;
      }
      return `${label} ${item.count}×${item.weight}`;
    });

  const variant = (
    ["low", "medium", "high", "critical"].includes(complexityLabel)
      ? complexityLabel
      : "unknown"
  ) as BadgeVariant;

  const labelText = variantLabels[variant] ?? complexityLabel;

  if (entries.length === 0) {
    return <StatusBadge variant={variant} label={`${labelText} (${score})`} />;
  }

  return (
    <span className="inline-flex items-center gap-1.5 text-sm">
      <span className="text-gray-500 font-mono text-xs">
        {entries.join(" + ")}
      </span>
      <span className="text-gray-400">=</span>
      <StatusBadge variant={variant} label={`${labelText} (${score})`} />
    </span>
  );
}
export function StaleBadge({
  isStale,
  stalenesTier,
  ageHours,
  size = "md",
}: {
  isStale: boolean;
  stalenesTier?: "fresh" | "warning" | "critical";
  ageHours?: number;
  size?: "sm" | "md";
}) {
  // Prefer tier if available, fall back to boolean for backward compat.
  const tier = stalenesTier ?? (isStale ? "critical" : "fresh");

  if (tier === "fresh") {
    return <StatusBadge variant="fresh" size={size} />;
  }

  // Format age label per spec:
  // < 1 hour → Nm, 1-47 hours → Nh, >= 48 hours → Nd
  let ageLabel = "";
  if (ageHours != null) {
    if (ageHours < 1) {
      ageLabel = ` (${Math.max(1, Math.round(ageHours * 60))}m)`;
    } else if (ageHours < 48) {
      ageLabel = ` (${Math.round(ageHours)}h)`;
    } else {
      ageLabel = ` (${Math.round(ageHours / 24)}d)`;
    }
  }

  if (tier === "warning") {
    return (
      <StatusBadge variant="warning" label={`Missing${ageLabel}`} size={size} />
    );
  }

  // critical
  return <StatusBadge variant="stale" label={`Gone${ageLabel}`} size={size} />;
}

/** Renders a CookStyle signal badge from a compatibility status string. */
export function CookStyleBadge({
  status,
  size = "md",
}: {
  status: string;
  size?: "sm" | "md";
}) {
  let variant: BadgeVariant;
  switch (status) {
    case "compatible":
    case "cookstyle_only":
      variant = "cs_compatible";
      break;
    case "incompatible":
      variant = "cs_incompatible";
      break;
    default:
      variant = "cs_untested";
      break;
  }
  return <StatusBadge variant={variant} size={size} />;
}

/**
 * Renders the CookStyle rollup status badge from the 4-state SoT value
 * (ready / needs_review / blocked / untested). This is the CookStyle signal
 * only — it is never merged with the Test Kitchen signal (use TKBadge for that).
 */
export function CookStyleStatusBadge({
  status,
  size = "md",
}: {
  status?: string | null;
  size?: "sm" | "md";
}) {
  let variant: BadgeVariant;
  let label: string;
  switch (status) {
    case "ready":
      variant = "cs_ready";
      label = "Ready";
      break;
    case "needs_review":
      variant = "cs_needs_review";
      label = "Needs review";
      break;
    case "blocked":
      variant = "cs_blocked";
      label = "Blocked";
      break;
    default:
      variant = "cs_untested";
      label = "Untested";
      break;
  }
  return <StatusBadge variant={variant} label={label} size={size} />;
}

/** Renders a Test Kitchen signal badge from a TK status string. */
export function TKBadge({
  status,
  size = "md",
}: {
  status: string;
  size?: "sm" | "md";
}) {
  let variant: BadgeVariant;
  switch (status) {
    case "passed":
      variant = "tk_passed";
      break;
    case "failed":
      variant = "tk_failed";
      break;
    case "partial":
      variant = "tk_partial";
      break;
    default:
      variant = "tk_untested";
      break;
  }
  return <StatusBadge variant={variant} size={size} />;
}

/** Renders a Disk space signal badge from a disk status string. */
export function DiskBadge({
  status,
  size = "md",
}: {
  status: string;
  size?: "sm" | "md";
}) {
  let variant: BadgeVariant;
  switch (status) {
    case "sufficient":
      variant = "disk_sufficient";
      break;
    case "insufficient":
      variant = "disk_insufficient";
      break;
    default:
      variant = "disk_unknown";
      break;
  }
  return <StatusBadge variant={variant} size={size} />;
}

/** Renders a deployment state badge (Current only / Staged / Activated). */
export function DeploymentStateBadge({
  state,
  size = "md",
}: {
  state?: string | null;
  size?: "sm" | "md";
}) {
  let variant: BadgeVariant;
  switch (state) {
    case "Staged":
      variant = "warning";
      break;
    case "Activated":
      variant = "active";
      break;
    default:
      variant = "inactive";
      break;
  }
  const label = state === "Staged" || state === "Activated" ? state : "Current only";
  return <StatusBadge variant={variant} label={label} size={size} />;
}

/** Renders a speculative converge status badge (success / fail / —). */
export function ConvergeBadge({
  status,
  size = "md",
}: {
  status?: string | null;
  size?: "sm" | "md";
}) {
  if (!status) {
    return <span className="text-xs text-gray-400">—</span>;
  }
  let variant: BadgeVariant;
  let label: string;
  switch (status) {
    case "success":
      variant = "ready";
      label = "Success";
      break;
    case "failed":
      variant = "blocked";
      label = "Failed";
      break;
    default:
      variant = "unknown";
      label = status;
      break;
  }
  return <StatusBadge variant={variant} label={label} size={size} />;
}
