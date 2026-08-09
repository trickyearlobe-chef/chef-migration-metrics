// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useState, type ReactNode } from "react";
import type { OffenseGroup } from "../types/remediation";
import { EmptyState } from "./Feedback";

// Section order + default-open policy. Blockers and Review (the actionable
// classes) start expanded; Noise starts collapsed to keep the view focused on
// what blocks readiness.
const SECTIONS: {
  key: string;
  label: string;
  defaultOpen: boolean;
  accent: string;
  note?: string;
}[] = [
  { key: "blocker", label: "Blockers", defaultOpen: true, accent: "text-red-700" },
  { key: "review", label: "Review", defaultOpen: true, accent: "text-amber-700" },
  { key: "noise", label: "Noise", defaultOpen: false, accent: "text-gray-500" },
  {
    key: "out_of_scope",
    label: "Outside the cookbook",
    defaultOpen: false,
    accent: "text-slate-500",
    note: "These will break on the new Ruby exactly as predicted, but they live in files a converge never executes — a helper task, a pipeline, a test suite. Real work for whoever owns them; it does not block this cookbook.",
  },
];

// A group sitting entirely in files the converge never executes is shown apart
// from its classification, so the counts here agree with the cookbook's verdict
// without the work disappearing. Anything with even one occurrence in cookbook
// code stays under its classification, because that occurrence is what decides
// the verdict.
//
// Review is the honest default: any group with no/unknown classification is
// folded into the Review worklist rather than a separate bucket.
function sectionKey(group: OffenseGroup): string {
  if (group.count > 0 && group.out_of_scope_count >= group.count) {
    return "out_of_scope";
  }
  return group.classification === "blocker" || group.classification === "noise"
    ? group.classification
    : "review";
}

/**
 * ClassificationSections groups offense groups into collapsible sections by cop
 * classification (Blocker / Review / Noise), each with an offense count.
 * Blockers and Review start expanded. Groups with no/unknown classification
 * fall under Review, the honest default. When `classFilter` is set only the
 * matching section is shown; if nothing matches, a real empty-state is rendered
 * instead of a blank area.
 *
 * Each underlying offense group is rendered via the `renderGroup` callback so
 * each page can keep its own OffenseGroupCard.
 */
export function ClassificationSections({
  groups,
  classFilter,
  renderGroup,
}: {
  groups: OffenseGroup[];
  classFilter: string;
  renderGroup: (group: OffenseGroup) => ReactNode;
}) {
  // Section collapse state, seeded from the default-open policy.
  const [collapsed, setCollapsed] = useState<Set<string>>(
    () => new Set(SECTIONS.filter((s) => !s.defaultOpen).map((s) => s.key)),
  );

  const toggleSection = (key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const visibleSections = SECTIONS.filter(
    (s) => classFilter === "" || classFilter === s.key,
  )
    .map((s) => ({
      ...s,
      groups: groups.filter((g) => sectionKey(g) === s.key),
    }))
    .filter((s) => s.groups.length > 0);

  if (visibleSections.length === 0) {
    const filterLabel =
      SECTIONS.find((s) => s.key === classFilter)?.label ?? classFilter;
    return (
      <EmptyState
        title="No offenses match this filter"
        description={
          classFilter
            ? `No ${filterLabel} offenses for this cookbook and target. Clear the filter to see the rest.`
            : "No offenses to show."
        }
      />
    );
  }

  return (
    <div className="space-y-3">
      {visibleSections.map((section) => {
        const isOpen = !collapsed.has(section.key);
        const offenseCount = section.groups.reduce((sum, g) => sum + g.count, 0);
        return (
          <div
            key={section.key}
            className="overflow-hidden rounded-lg border border-gray-200"
          >
            <button
              type="button"
              className="flex w-full items-center gap-3 bg-gray-50 px-4 py-2.5 text-left hover:bg-gray-100"
              onClick={() => toggleSection(section.key)}
              aria-expanded={isOpen}
            >
              <svg
                className={`h-4 w-4 shrink-0 text-gray-400 transition-transform duration-200 ${isOpen ? "rotate-90" : ""}`}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
                aria-hidden="true"
              >
                <path strokeLinecap="round" strokeLinejoin="round" d="m8.25 4.5 7.5 7.5-7.5 7.5" />
              </svg>
              <span className={`text-sm font-semibold ${section.accent}`}>
                {section.label}
              </span>
              <span className="rounded-full bg-white px-2 py-0.5 text-xs font-medium text-gray-600 ring-1 ring-gray-200">
                {section.groups.length} cop{section.groups.length !== 1 ? "s" : ""},{" "}
                {offenseCount} offense{offenseCount !== 1 ? "s" : ""}
              </span>
            </button>
            {isOpen && (
              <div className="space-y-3 p-3">
                {section.note && (
                  <p className="rounded bg-slate-50 px-3 py-2 text-xs text-slate-600">
                    {section.note}
                  </p>
                )}
                {section.groups.map((group) => renderGroup(group))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
