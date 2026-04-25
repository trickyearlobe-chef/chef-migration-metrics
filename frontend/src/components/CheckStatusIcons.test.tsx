// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { CheckStatusIcons } from "./CheckStatusIcons";

describe("CheckStatusIcons", () => {
  it("renders three icons with correct aria labels", () => {
    render(
      <CheckStatusIcons
        diskStatus="sufficient"
        cookstyleStatus="passed"
        kitchenStatus="passed"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    const icons = screen.getAllByRole("img");
    expect(icons).toHaveLength(3);
    expect(icons[0]).toHaveAttribute(
      "aria-label",
      expect.stringContaining("Disk space:"),
    );
    expect(icons[1]).toHaveAttribute(
      "aria-label",
      expect.stringContaining("CookStyle:"),
    );
    expect(icons[2]).toHaveAttribute(
      "aria-label",
      expect.stringContaining("Test Kitchen:"),
    );
  });

  it("applies green colour for passed/sufficient", () => {
    render(
      <CheckStatusIcons
        diskStatus="sufficient"
        cookstyleStatus="passed"
        kitchenStatus="passed"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    const icons = screen.getAllByRole("img");
    for (const icon of icons) {
      expect(icon.className).toContain("text-green-600");
    }
  });

  it("applies red colour for failed/insufficient", () => {
    render(
      <CheckStatusIcons
        diskStatus="insufficient"
        cookstyleStatus="failed"
        kitchenStatus="failed"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    const icons = screen.getAllByRole("img");
    for (const icon of icons) {
      expect(icon.className).toContain("text-red-600");
    }
  });

  it("applies orange colour for scan_error", () => {
    render(
      <CheckStatusIcons
        diskStatus="unknown"
        cookstyleStatus="scan_error"
        kitchenStatus="scan_error"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    const icons = screen.getAllByRole("img");
    expect(icons[1].className).toContain("text-orange-500");
    expect(icons[2].className).toContain("text-orange-500");
  });

  it("applies amber colour for warnings/partial", () => {
    render(
      <CheckStatusIcons
        diskStatus="unknown"
        cookstyleStatus="warnings"
        kitchenStatus="partial"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    const icons = screen.getAllByRole("img");
    expect(icons[1].className).toContain("text-amber-500");
    expect(icons[2].className).toContain("text-amber-500");
  });

  it("applies grey colour for unknown", () => {
    render(
      <CheckStatusIcons
        diskStatus="unknown"
        cookstyleStatus="unknown"
        kitchenStatus="unknown"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    const icons = screen.getAllByRole("img");
    for (const icon of icons) {
      expect(icon.className).toContain("text-gray-400");
    }
  });

  it("shows correct overlay characters", () => {
    const { container } = render(
      <CheckStatusIcons
        diskStatus="sufficient"
        cookstyleStatus="failed"
        kitchenStatus="unknown"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    expect(container.textContent).toContain("✓");
    expect(container.textContent).toContain("✗");
    expect(container.textContent).toContain("?");
  });

  it("shows overlay for scan_error and partial/warnings", () => {
    const { container } = render(
      <CheckStatusIcons
        diskStatus="unknown"
        cookstyleStatus="scan_error"
        kitchenStatus="partial"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    expect(container.textContent).toContain("!");
    expect(container.textContent).toContain("~");
  });

  it("displays detail text in tooltips", () => {
    render(
      <CheckStatusIcons
        diskStatus="sufficient"
        cookstyleStatus="passed"
        kitchenStatus="passed"
        diskDetail="50GB free"
        cookstyleDetail="0 offenses"
        kitchenDetail="3/3 suites passed"
      />,
    );
    const icons = screen.getAllByRole("img");
    expect(icons[0]).toHaveAttribute("title", "50GB free");
    expect(icons[1]).toHaveAttribute("title", "0 offenses");
    expect(icons[2]).toHaveAttribute("title", "3/3 suites passed");
  });

  it("falls back to generic tooltip when detail is null", () => {
    render(
      <CheckStatusIcons
        diskStatus="unknown"
        cookstyleStatus="unknown"
        kitchenStatus="unknown"
        diskDetail={null}
        cookstyleDetail={null}
        kitchenDetail={null}
      />,
    );
    const icons = screen.getAllByRole("img");
    expect(icons[0]).toHaveAttribute("title", "Disk: unknown");
    expect(icons[1]).toHaveAttribute("title", "CookStyle: unknown");
    expect(icons[2]).toHaveAttribute("title", "Test Kitchen: unknown");
  });

  it("sets accessible aria-label with detail", () => {
    render(
      <CheckStatusIcons
        diskStatus="sufficient"
        cookstyleStatus="failed"
        kitchenStatus="passed"
        diskDetail="50GB free"
        cookstyleDetail="12 offenses"
        kitchenDetail="all green"
      />,
    );
    const icons = screen.getAllByRole("img");
    expect(icons[0]).toHaveAttribute(
      "aria-label",
      "Disk space: sufficient — 50GB free",
    );
    expect(icons[1]).toHaveAttribute(
      "aria-label",
      "CookStyle: failed — 12 offenses",
    );
    expect(icons[2]).toHaveAttribute(
      "aria-label",
      "Test Kitchen: passed — all green",
    );
  });
});
