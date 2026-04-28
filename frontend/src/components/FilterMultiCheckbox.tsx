// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from "react";

interface FilterMultiCheckboxProps {
  label: string;
  options: { value: string; label: string; count?: number }[];
  selected: string[];
  onChange: (selected: string[]) => void;
  compact?: boolean;
}

export function FilterMultiCheckbox({
  label,
  options,
  selected,
  onChange,
  compact = false,
}: FilterMultiCheckboxProps) {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleOutsideClick(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setIsOpen(false);
      }
    }
    document.addEventListener("mousedown", handleOutsideClick);
    return () => document.removeEventListener("mousedown", handleOutsideClick);
  }, []);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") setIsOpen(false);
    }
    if (isOpen) {
      document.addEventListener("keydown", handleKeyDown);
      return () => document.removeEventListener("keydown", handleKeyDown);
    }
  }, [isOpen]);

  function toggle(value: string) {
    if (selected.includes(value)) {
      onChange(selected.filter((v) => v !== value));
    } else {
      onChange([...selected, value]);
    }
  }

  function remove(value: string) {
    onChange(selected.filter((v) => v !== value));
  }

  const buttonLabel =
    selected.length > 0 ? `${label} (${selected.length})` : label;

  const wrapperClass = compact
    ? "relative flex items-center gap-1.5"
    : "relative";

  const labelClass = compact
    ? "text-xs font-medium text-gray-500"
    : "mb-1 block text-xs font-medium text-gray-500";

  const buttonClass = compact
    ? "block w-28 rounded-md border border-gray-300 bg-white px-1.5 py-1 text-left text-xs shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
    : "block w-40 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-left text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500";

  return (
    <div ref={containerRef} className={wrapperClass}>
      <label className={labelClass}>{label}</label>
      <button
        type="button"
        onClick={() => setIsOpen((o) => !o)}
        className={buttonClass}
      >
        {buttonLabel}
      </button>

      {isOpen && (
        <div className="absolute left-0 top-full z-10 mt-1 max-h-60 w-56 overflow-auto rounded-md border border-gray-300 bg-white py-1 shadow-lg">
          {options.map((opt) => (
            <label
              key={opt.value}
              className="flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm hover:bg-gray-50"
            >
              <input
                type="checkbox"
                checked={selected.includes(opt.value)}
                onChange={() => toggle(opt.value)}
                className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span>
                {opt.label}
                {opt.count !== undefined && (
                  <span className="ml-1 text-gray-400">({opt.count})</span>
                )}
              </span>
            </label>
          ))}
          {options.length === 0 && (
            <div className="px-3 py-2 text-sm text-gray-400">No options</div>
          )}
        </div>
      )}

      {!compact && selected.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {selected.map((value) => {
            const opt = options.find((o) => o.value === value);
            return (
              <span
                key={value}
                className="inline-flex items-center gap-0.5 rounded-full bg-blue-100 px-2 py-0.5 text-xs text-blue-800"
              >
                {opt?.label ?? value}
                <button
                  type="button"
                  onClick={() => remove(value)}
                  className="ml-0.5 text-blue-600 hover:text-blue-900"
                  aria-label={`Remove ${opt?.label ?? value}`}
                >
                  ×
                </button>
              </span>
            );
          })}
        </div>
      )}
    </div>
  );
}
