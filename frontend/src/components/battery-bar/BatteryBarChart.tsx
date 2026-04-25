// SPDX-License-Identifier: Apache-2.0

import { useState, useCallback } from "react";
import type { GroupedMajorVersion } from "./grouping";
import { getSegmentColour } from "./colours";

export interface BatteryBarChartProps {
  groups: GroupedMajorVersion[];
  totalCount: number;
  labelPrefix: string;
  childLinkBuilder: (version: string) => string;
}

export function BatteryBarChart({
  groups,
  totalCount,
  labelPrefix,
  childLinkBuilder,
}: BatteryBarChartProps) {
  const [expandedMajor, setExpandedMajor] = useState<number | null>(null);

  const toggleExpand = useCallback(
    (major: number) => {
      setExpandedMajor((prev) => (prev === major ? null : major));
    },
    [],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent, major: number) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        toggleExpand(major);
      }
    },
    [toggleExpand],
  );

  const handleChildKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        setExpandedMajor(null);
        // Return focus to the parent row.
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
        const isExpanded = expandedMajor === group.majorVersion;
        const barWidth = totalCount > 0
          ? Math.max((group.totalCount / totalCount) * 100, 2)
          : 0;
        const showPctInBar = group.totalPercentage >= 8;
        const labelId = `battery-label-${group.majorVersion}`;

        return (
          <div key={group.majorVersion}>
            {/* Battery bar row */}
            <div
              className="battery-bar-row"
              role="button"
              tabIndex={0}
              aria-expanded={isExpanded}
              onClick={() => toggleExpand(group.majorVersion)}
              onKeyDown={(e) => handleKeyDown(e, group.majorVersion)}
            >
              <span className="bar-chart-label" id={labelId}>
                <span className="mr-1 text-xs text-gray-400">
                  {isExpanded ? "▾" : "▸"}
                </span>
                {labelPrefix} {group.majorVersion}
              </span>
              <div className="bar-chart-track">
                <div
                  className="battery-bar-track"
                  style={{ width: `${barWidth}%` }}
                  role="img"
                  aria-label={`${labelPrefix} ${group.majorVersion}: ${group.versions.map((v) => `${v.version} (${v.count})`).join(", ")}`}
                >
                  {group.versions.map((v, vIdx) => {
                    const segWidth =
                      group.totalCount > 0
                        ? (v.count / group.totalCount) * 100
                        : 0;
                    return (
                      <div
                        key={v.version}
                        className="battery-bar-segment"
                        style={{
                          width: `${Math.max(segWidth, 1)}%`,
                          backgroundColor: getSegmentColour(
                            groupIndex,
                            vIdx,
                            group.versions.length,
                          ),
                        }}
                        title={`${v.version} — ${v.count.toLocaleString()} nodes (${v.percent.toFixed(1)}%)`}
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
                {group.versions.map((v, vIdx) => {
                  const childPct =
                    totalCount > 0 ? (v.count / totalCount) * 100 : 0;
                  return (
                    <a
                      key={v.version}
                      href={childLinkBuilder(v.version)}
                      className="battery-bar-child"
                      onKeyDown={handleChildKeyDown}
                      onClick={(e) => {
                        // Use client-side navigation if available
                        e.preventDefault();
                        window.location.href = childLinkBuilder(v.version);
                      }}
                    >
                      <span className="bar-chart-label pl-6 text-xs">
                        {v.version}
                      </span>
                      <div className="bar-chart-track">
                        <div
                          className="bar-chart-fill"
                          style={{
                            width: `${Math.max(childPct, 2)}%`,
                            backgroundColor: getSegmentColour(
                              groupIndex,
                              vIdx,
                              group.versions.length,
                            ),
                          }}
                        >
                          {childPct >= 8 && (
                            <span>{childPct.toFixed(1)}%</span>
                          )}
                        </div>
                      </div>
                      <span className="bar-chart-value text-xs">
                        {v.count.toLocaleString()}
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
