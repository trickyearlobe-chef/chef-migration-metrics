// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from "react";
import { apiFetch } from "../api/client";

interface FilterTypeAheadProps {
  label: string;
  endpoint: string;
  debounceMs?: number;
  minChars?: number;
  selected: string[];
  onChange: (selected: string[]) => void;
  matchType?: "prefix" | "substring";
}

export function FilterTypeAhead({
  label,
  endpoint,
  debounceMs = 300,
  minChars = 2,
  selected,
  onChange,
  matchType: _matchType = "prefix",
}: FilterTypeAheadProps) {
  const [input, setInput] = useState("");
  const [results, setResults] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [isOpen, setIsOpen] = useState(false);

  const containerRef = useRef<HTMLDivElement>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

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
    if (timerRef.current) clearTimeout(timerRef.current);

    if (input.length < minChars) {
      setResults([]);
      setIsOpen(false);
      return;
    }

    timerRef.current = setTimeout(() => {
      setLoading(true);
      apiFetch<{ data: string[] }>(`${endpoint}?q=${encodeURIComponent(input)}`)
        .then((res) => {
          const filtered = res.data.filter((v) => !selected.includes(v));
          setResults(filtered);
          setIsOpen(filtered.length > 0);
        })
        .catch(() => {
          setResults([]);
        })
        .finally(() => {
          setLoading(false);
        });
    }, debounceMs);

    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [input, debounceMs, minChars, endpoint, selected]);

  function selectItem(value: string) {
    onChange([...selected, value]);
    setInput("");
    setResults([]);
    setIsOpen(false);
  }

  function removeItem(value: string) {
    onChange(selected.filter((v) => v !== value));
  }

  return (
    <div ref={containerRef} className="relative">
      <label className="mb-1 block text-xs font-medium text-gray-500">
        {label}
      </label>
      <div className="relative">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onFocus={() => {
            if (results.length > 0) setIsOpen(true);
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") setIsOpen(false);
          }}
          placeholder={`Search ${label.toLowerCase()}\u2026`}
          className="block w-48 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
        {loading && (
          <div className="pointer-events-none absolute inset-y-0 right-2 flex items-center">
            <svg
              className="h-4 w-4 animate-spin text-gray-400"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"
              />
            </svg>
          </div>
        )}
      </div>

      {isOpen && results.length > 0 && (
        <div className="absolute z-10 mt-1 max-h-60 w-48 overflow-auto rounded-md border border-gray-300 bg-white py-1 shadow-lg">
          {results.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => selectItem(item)}
              className="block w-full px-3 py-1.5 text-left text-sm hover:bg-gray-50"
            >
              {item}
            </button>
          ))}
        </div>
      )}

      {selected.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {selected.map((value) => (
            <span
              key={value}
              className="inline-flex items-center gap-0.5 rounded-full bg-blue-100 px-2 py-0.5 text-xs text-blue-800"
            >
              {value}
              <button
                type="button"
                onClick={() => removeItem(value)}
                className="ml-0.5 text-blue-600 hover:text-blue-900"
                aria-label={`Remove ${value}`}
              >
                &times;
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
