// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import type { CookstyleFailurePreset } from "../types/config";
import {
  COOKSTYLE_SEVERITIES,
  COOKSTYLE_NAMESPACES,
  COOKSTYLE_NAMESPACE_LABELS,
  COOKSTYLE_PRESETS,
} from "../types/config";

interface Props {
  preset: CookstyleFailurePreset;
  rules: Record<string, string[]>;
  onChange: (preset: CookstyleFailurePreset, rules: Record<string, string[]>) => void;
  disabled: boolean;
}

export function CookstyleFailureRulesGrid({ preset, rules, onChange, disabled }: Props) {
  function handlePresetChange(newPreset: string) {
    if (newPreset === "custom") {
      onChange("custom", rules);
      return;
    }
    const p = newPreset as Exclude<CookstyleFailurePreset, "custom">;
    onChange(p, COOKSTYLE_PRESETS[p]);
  }

  function handleCheckboxChange(namespace: string, severity: string, checked: boolean) {
    const current = rules[namespace] ?? [];
    const updated = checked
      ? [...current, severity]
      : current.filter((s) => s !== severity);
    const newRules = { ...rules, [namespace]: updated };
    onChange("custom", newRules);
  }

  function isChecked(namespace: string, severity: string): boolean {
    return (rules[namespace] ?? []).includes(severity);
  }

  return (
    <div className="space-y-3">
      <div>
        <label htmlFor="cookstyle-preset" className="block text-sm font-medium text-gray-700">
          Failure Rules Preset
        </label>
        <select
          id="cookstyle-preset"
          aria-label="Failure Rules Preset"
          value={preset}
          onChange={(e) => handlePresetChange(e.target.value)}
          disabled={disabled}
          className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-50"
        >
          <option value="strict">Strict</option>
          <option value="default">Default</option>
          <option value="relaxed">Relaxed</option>
          <option value="custom">Custom</option>
        </select>
        <p className="mt-1 text-xs text-gray-500">
          Controls which CookStyle offense severities cause a cookbook to fail. Manual changes switch
          to &ldquo;Custom&rdquo;.
        </p>
      </div>

      <div className="overflow-x-auto">
        <table className="min-w-full text-sm">
          <thead>
            <tr>
              <th className="px-3 py-2 text-left font-medium text-gray-700">Namespace</th>
              {COOKSTYLE_SEVERITIES.map((sev) => (
                <th key={sev} className="px-3 py-2 text-center font-medium text-gray-700">
                  {sev}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {COOKSTYLE_NAMESPACES.map((ns) => (
              <tr key={ns} className="border-t border-gray-100">
                <td className="px-3 py-2 font-medium text-gray-600">
                  {COOKSTYLE_NAMESPACE_LABELS[ns]}
                </td>
                {COOKSTYLE_SEVERITIES.map((sev) => (
                  <td key={sev} className="px-3 py-2 text-center">
                    <input
                      type="checkbox"
                      aria-label={`${COOKSTYLE_NAMESPACE_LABELS[ns]} ${sev}`}
                      checked={isChecked(ns, sev)}
                      onChange={(e) => handleCheckboxChange(ns, sev, e.target.checked)}
                      disabled={disabled}
                      className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50"
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
