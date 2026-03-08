import type { CompatibilityStatus, ComplexityLabel } from "../types";

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
  | "blocked"
  | "stale"
  | "fresh"
  | "active"
  | "inactive"
  | "healthy"
  | "unhealthy"
  | "unknown";

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
  blocked: "bg-red-100 text-red-800 ring-red-600/20",
  stale: "bg-purple-100 text-purple-800 ring-purple-600/20",
  fresh: "bg-green-100 text-green-800 ring-green-600/20",
  active: "bg-green-100 text-green-800 ring-green-600/20",
  inactive: "bg-gray-100 text-gray-600 ring-gray-500/20",

  // Health
  healthy: "bg-green-100 text-green-800 ring-green-600/20",
  unhealthy: "bg-red-100 text-red-800 ring-red-600/20",
  unknown: "bg-gray-100 text-gray-600 ring-gray-500/20",
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
  blocked: "Blocked",
  stale: "Stale",
  fresh: "Fresh",
  active: "Active",
  inactive: "Inactive",
  healthy: "Healthy",
  unhealthy: "Unhealthy",
  unknown: "Unknown",
};

/** Short descriptor shown as a tooltip on hover for compatibility statuses. */
const variantTooltips: Partial<Record<BadgeVariant, string>> = {
  compatible:
    "Full integration test (Test Kitchen) passed — high confidence",
  cookstyle_only:
    "Static analysis only (CookStyle) — no integration test. Medium confidence.",
  incompatible: "Known to be incompatible with the target Chef version",
  untested: "No test or scan results available yet",
  stale: "Last check-in exceeds the configured stale threshold",
  critical: "Critical remediation complexity — significant manual effort required",
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
      {/* Dot indicator for compatibility statuses to make the distinction
          between green (TK pass) and amber (CookStyle) unmissable per spec */}
      {(variant === "compatible" ||
        variant === "cookstyle_only" ||
        variant === "incompatible" ||
        variant === "untested") && (
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
      return "bg-green-500";
    case "cookstyle_only":
      return "bg-amber-500";
    case "incompatible":
      return "bg-red-500";
    case "untested":
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
  confidence,
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
      label = confidence === "high" ? "Compatible" : "Compatible";
      break;
    case "cookstyle_only":
      variant = "cookstyle_only";
      label = "CookStyle Only";
      break;
    case "incompatible":
      variant = "incompatible";
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
      : variantLabels[variant] ?? complexityLabel;

  return <StatusBadge variant={variant} label={label} size={size} />;
}

/** Renders a stale/fresh indicator for nodes. */
export function StaleBadge({
  isStale,
  ageHours,
  size = "md",
}: {
  isStale: boolean;
  ageHours?: number;
  size?: "sm" | "md";
}) {
  if (!isStale) {
    return <StatusBadge variant="fresh" size={size} />;
  }

  let label = "Stale";
  if (ageHours != null) {
    if (ageHours < 48) {
      label = `Stale (${Math.round(ageHours)}h ago)`;
    } else {
      const days = Math.round(ageHours / 24);
      label = `Stale (${days}d ago)`;
    }
  }

  return <StatusBadge variant="stale" label={label} size={size} />;
}
