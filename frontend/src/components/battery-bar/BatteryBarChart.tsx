// SPDX-License-Identifier: Apache-2.0

import { useState, useCallback } from "react";
import type { BarGroup } from "./grouping";
import { getSegmentColour } from "./colours";

export interface BatteryBarChartProps {
  groups: BarGroup[];
  totalCount: number;
  /** @deprecated Use groupByMajorVersion(dist, total, prefix) instead. */
  labelPrefix?: string;
  childLinkBuilder: (filterValue: string) => string;
}

export function BatteryBarChart({
  groups,
  totalCount,
  childLinkBuilder,
}: BatteryBarChartProps) {
  const [expandedKey, setExpandedKey] = useState<string | null>(null);

  const toggleExpand = useCallback(
    (key: string) => {
      setExpandedKey((prev) => (prev === key ? null : key));
    },
    [],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent, key: string) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        toggleExpand(key);
      }
    },
    [toggleExpand],
  );

  const handleChildKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        setExpandedKey(null);
        const target = e.currentTarget as HTMLElement;
        const parent = target.closest(".battery-bar-children")
          ?.previousElementSibling as HTMLElement | null;
        parent?.focus();
      }
    },
    [],
  );

  if (groups.length === 0) return null;

  return (
    <div className="space-y-0.5">
      {groups.map((group, groupIndex) => {
        const isExpanded = expandedKey === group.key;
        const barWidth = totalCount > 0
          ? Math.max((group.totalCount / totalCount) * 100, 2)
          : 0;
        const showPctInBar = group.totalPercentage >= 8;
        const labelId = `battery-label-${group.key}`;

        return (
          <div key={group.key}>
            {/* Battery bar row */}
            <div
              className="battery-bar-row"
              role="button"
              tabIndex={0}
              aria-expanded={isExpanded}
              onClick={() => toggleExpand(group.key)}
              onKeyDown={(e) => handleKeyDown(e, group.key)}
            >
              <span className="bar-chart-label" id={labelId}>
                <span className="mr-1 text-xs text-gray-400">
                  {isExpanded ? "▾" : "▸"}
                </span>
                {group.label}
              </span>
              <div className="bar-chart-track">
                <div
                  className="battery-bar-track"
                  style={{ width: `${barWidth}%` }}
                  role="img"
                  aria-label={`${group.label}: ${group.entries.map((e) => `${e.label} (${e.count})`).join(", ")}`}
                >
                  {group.entries.map((entry, vIdx) => {
                    const segWidth =
                      group.totalCount > 0
                        ? (entry.count / group.totalCount) * 100
                        : 0;
                    return (
                      <div
                        key={entry.filterValue}
                        className="battery-bar-segment"
                        style={{
                          width: `${Math.max(segWidth, 1)}%`,
                          backgroundColor: getSegmentColour(
                            groupIndex,
                            vIdx,
                            group.entries.length,
                          ),
                        }}
                        title={`${entry.label} — ${entry.count.toLocaleString()} nodes (${entry.percent.toFixed(1)}%)`}
                      />
                    );
                  })}
                  {showPctInBar && (
                    <span className="battery-bar-pct">
                      {group.totalPercentage.toFixed(1)}%
                    </span>
                  )}
                </div>
              </div>
              <span className="bar-chart-value">
                {group.totalCount.toLocaleString()}
              </span>
            </div>

            {/* Expanded child rows */}
            {isExpanded && (
              <div
                className="battery-bar-children"
                role="region"
                aria-labelledby={labelId}
              >
                {group.entries.map((entry, vIdx) => {
                  const childPct =
                    totalCount > 0 ? (entry.count / totalCount) * 100 : 0;
                  return (
                    <a
                      key={entry.filterValue}
                      href={childLinkBuilder(entry.filterValue)}
                      className="battery-bar-child"
                      onKeyDown={handleChildKeyDown}
                      onClick={(e) => {
                        e.preventDefault();
                        window.location.href = childLinkBuilder(entry.filterValue);
                      }}
                    >
                      <span className="bar-chart-label pl-6 text-xs">
                        {entry.label}
                      </span>
                      <div className="bar-chart-track">
                        <div
                          className="bar-chart-fill"
                          style={{
                            width: `${Math.max(childPct, 2)}%`,
                            backgroundColor: getSegmentColour(
                              groupIndex,
                              vIdx,
                              group.entries.length,
                            ),
                          }}
                        >
                          {childPct >= 8 && (
                            <span>{childPct.toFixed(1)}%</span>
                          )}
                        </div>
                      </div>
                      <span className="bar-chart-value text-xs">
                        {entry.count.toLocaleString()}
                      </span>
                    </a>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
