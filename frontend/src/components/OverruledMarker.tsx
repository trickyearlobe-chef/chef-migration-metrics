// SPDX-License-Identifier: Apache-2.0

import type { GitRepoListItem } from "../types";

interface Props {
  verdict: GitRepoListItem["human_verdict"];
  reason?: string;
}

/**
 * Marks a status a person has overruled.
 *
 * Sits inside the existing status cell rather than taking a column of its own:
 * the git repo list is already busy, and this is a qualifier on a status
 * that is already there, not a separate fact.
 *
 * The scan's own badge is left exactly as it was. That is deliberate — the
 * losing verdict stays visible, and what changes is that it no longer gets the
 * last word without saying so.
 *
 * **Known limitation.** A human verdict is not CookStyle-specific: it outranks
 * Test Kitchen too, so hanging it off the CookStyle cell reads as a qualifier
 * on that one scan. It is right for this customer only because their CookStyle
 * coverage is wide and their Test Kitchen coverage is almost non-existent.
 * Accepted for the MVP and recorded in plans/todo-tech-debt.md.
 */
export function OverruledMarker({ verdict, reason }: Props) {
  if (!verdict) return null;

  const overruling = verdict === "not_broken";

  return (
    <span
      title={
        (overruling
          ? "A person recorded that this is not broken, and readiness follows them rather than this scan."
          : "A person recorded that this is broken, and readiness follows them rather than this scan.") +
        (reason ? `\n\nReason: ${reason}` : "")
      }
      data-testid="overruled-marker"
      className={`ml-1 inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium ${
        overruling
          ? "bg-green-100 text-green-800"
          : "bg-red-100 text-red-800"
      }`}
    >
      {overruling ? "person says OK" : "person says broken"}
    </span>
  );
}
