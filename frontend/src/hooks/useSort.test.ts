// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useSort } from "./useSort";

describe("useSort", () => {
  it("returns the default field and order", () => {
    const { result } = renderHook(() =>
      useSort({ defaultField: "name" as const }),
    );
    expect(result.current.sortField).toBe("name");
    expect(result.current.sortOrder).toBe("asc");
  });

  it("respects a custom default order", () => {
    const { result } = renderHook(() =>
      useSort({ defaultField: "date" as const, defaultOrder: "desc" }),
    );
    expect(result.current.sortField).toBe("date");
    expect(result.current.sortOrder).toBe("desc");
  });

  it("toggles order when the same field is clicked", () => {
    const { result } = renderHook(() =>
      useSort({ defaultField: "name" as const }),
    );
    expect(result.current.sortOrder).toBe("asc");

    act(() => result.current.handleSort("name"));
    expect(result.current.sortOrder).toBe("desc");

    act(() => result.current.handleSort("name"));
    expect(result.current.sortOrder).toBe("asc");
  });

  it("switches to ascending when a new non-descending field is selected", () => {
    const { result } = renderHook(() =>
      useSort({
        defaultField: "name" as const,
        defaultOrder: "desc",
      }),
    );

    act(() => result.current.handleSort("email"));
    expect(result.current.sortField).toBe("email");
    expect(result.current.sortOrder).toBe("asc");
  });

  it("switches to descending when a descendingFields entry is selected", () => {
    const { result } = renderHook(() =>
      useSort({
        defaultField: "name" as const,
        descendingFields: ["created_at", "count"],
      }),
    );
    expect(result.current.sortOrder).toBe("asc");

    act(() => result.current.handleSort("created_at"));
    expect(result.current.sortField).toBe("created_at");
    expect(result.current.sortOrder).toBe("desc");
  });

  it("still toggles a descending field on repeated click", () => {
    const { result } = renderHook(() =>
      useSort({
        defaultField: "name" as const,
        descendingFields: ["count"],
      }),
    );

    act(() => result.current.handleSort("count"));
    expect(result.current.sortOrder).toBe("desc");

    act(() => result.current.handleSort("count"));
    expect(result.current.sortOrder).toBe("asc");

    act(() => result.current.handleSort("count"));
    expect(result.current.sortOrder).toBe("desc");
  });

  it("defaults to ascending when descendingFields is omitted", () => {
    const { result } = renderHook(() =>
      useSort({ defaultField: "a" as const }),
    );

    act(() => result.current.handleSort("b"));
    expect(result.current.sortField).toBe("b");
    expect(result.current.sortOrder).toBe("asc");
  });

  it("handles switching between multiple fields correctly", () => {
    const { result } = renderHook(() =>
      useSort({
        defaultField: "name" as const,
        descendingFields: ["date"],
      }),
    );

    // Toggle current field to desc
    act(() => result.current.handleSort("name"));
    expect(result.current.sortOrder).toBe("desc");

    // Switch to a descending-default field
    act(() => result.current.handleSort("date"));
    expect(result.current.sortField).toBe("date");
    expect(result.current.sortOrder).toBe("desc");

    // Switch to a regular field
    act(() => result.current.handleSort("email"));
    expect(result.current.sortField).toBe("email");
    expect(result.current.sortOrder).toBe("asc");

    // Go back to date — should default to desc again
    act(() => result.current.handleSort("date"));
    expect(result.current.sortField).toBe("date");
    expect(result.current.sortOrder).toBe("desc");
  });
});
