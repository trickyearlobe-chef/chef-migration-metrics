// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { CookstyleResultRow } from "./CookstyleResultRow";
import type { CookstyleResult } from "../types/cookbooks";

const baseResult: CookstyleResult = {
  id: "cs-1",
  cookbook_id: "cb-1",
  target_chef_version: "18.4",
  passed: true,
  offence_count: 0,
  deprecation_count: 0,
  scanned_at: "2025-01-15T10:30:00Z",
  created_at: "2025-01-15T10:30:00Z",
};

function renderRow(
  result: CookstyleResult,
  linkBase?: string,
) {
  return render(
    <MemoryRouter>
      <CookstyleResultRow result={result} linkBase={linkBase} />
    </MemoryRouter>,
  );
}

describe("CookstyleResultRow", () => {
  it("renders a Ready badge (passed fallback) with counts and scanned timestamp", () => {
    renderRow({ ...baseResult, offence_count: 2, deprecation_count: 1 });

    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText(/Offences: 2/)).toBeInTheDocument();
    expect(screen.getByText(/Deprecations: 1/)).toBeInTheDocument();
    expect(screen.getByText(/Scanned:/)).toBeInTheDocument();
  });

  it("renders a Blocked badge (passed=false fallback) with counts", () => {
    renderRow({
      ...baseResult,
      passed: false,
      offence_count: 5,
      deprecation_count: 3,
    });

    expect(screen.getByText("Blocked")).toBeInTheDocument();
    expect(screen.getByText(/Offences: 5/)).toBeInTheDocument();
    expect(screen.getByText(/Deprecations: 3/)).toBeInTheDocument();
  });

  it("prefers the materialised cookstyle_status over the passed boolean", () => {
    // passed=true but status is needs_review — the 4-state value must win.
    renderRow({ ...baseResult, passed: true, cookstyle_status: "needs_review" });

    expect(screen.getByText("Needs review")).toBeInTheDocument();
    expect(screen.queryByText("Ready")).not.toBeInTheDocument();
  });

  it("renders an orange Scan Error badge without offence counts", () => {
    renderRow({
      ...baseResult,
      passed: false,
      offence_count: 0,
      deprecation_count: 0,
      error_message: "cookstyle exited with status 2",
    });

    expect(screen.getByText("Scan Error")).toBeInTheDocument();
    expect(
      screen.getByText("cookstyle exited with status 2"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Offences:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Deprecations:/)).not.toBeInTheDocument();
  });

  it("shows expandable stderr detail on scan error", async () => {
    const user = userEvent.setup();
    renderRow({
      ...baseResult,
      passed: false,
      error_message: "cookstyle crashed",
      process_stderr: "FATAL: NoMethodError undefined method `foo'",
    });

    // stderr is not visible initially
    expect(
      screen.queryByText(/NoMethodError/),
    ).not.toBeInTheDocument();

    // Click the expand button
    const toggle = screen.getByRole("button", { name: /show details/i });
    await user.click(toggle);

    expect(
      screen.getByText(/NoMethodError undefined method/),
    ).toBeInTheDocument();
  });

  it("does not show expand button when error has no stderr", () => {
    renderRow({
      ...baseResult,
      passed: false,
      error_message: "cookstyle crashed",
    });

    expect(
      screen.queryByRole("button", { name: /show details/i }),
    ).not.toBeInTheDocument();
  });

  it("renders a remediation link when linkBase is provided", () => {
    renderRow(baseResult, "/cookbooks/my-cb/1.0.0/remediation");

    const link = screen.getByRole("link", {
      name: /view remediation detail/i,
    });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute(
      "href",
      "/cookbooks/my-cb/1.0.0/remediation?target_chef_version=18.4",
    );
  });

  it("does not render a remediation link when linkBase is omitted", () => {
    renderRow(baseResult);

    expect(
      screen.queryByRole("link", { name: /view remediation detail/i }),
    ).not.toBeInTheDocument();
  });
});
