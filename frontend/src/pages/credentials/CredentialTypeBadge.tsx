import { BADGE_STYLES, BADGE_LABELS } from "./constants";

export function CredentialTypeBadge({ type }: { type: string }) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${BADGE_STYLES[type] ?? BADGE_STYLES.generic}`}
    >
      {BADGE_LABELS[type] ?? type}
    </span>
  );
}
