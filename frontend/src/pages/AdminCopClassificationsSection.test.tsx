// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import * as api from "../api";
import type { CopAggregationResponse } from "../types";

vi.mock("../api");

import { AdminCopClassificationsSection } from "./AdminCopClassificationsSection";

const RESPONSE: CopAggregationResponse = {
  summary: {
    blocker_cops: 1,
    blocker_cookbooks: 4,
    review_cops: 1,
    review_cookbooks: 2,
    noise_cops: 1,
    unclassified_cops: 1,
  },
  data: [
    {
      cop_name: "Chef/Deprecations/ResourceWithoutUnifiedTrue",
      description: "Required for Chef 18 unified mode",
      category: "Chef/Deprecations",
      severity: "warning",
      classification: "review",
      classification_source: "curated_default",
      cookbooks_affected: 2,
      total_offences: 5,
      auto_correctable_pct: 0,
      unblocks: 0,
      is_custom: false,
    },
    {
      cop_name: "Lint/DeprecatedClassMethods",
      description: "File.exists? removed in Ruby 3",
      category: "Lint",
      severity: "warning",
      classification: "blocker",
      classification_source: "operator_override",
      cookbooks_affected: 4,
      total_offences: 9,
      auto_correctable_pct: 100,
      unblocks: 3,
      is_custom: false,
    },
  ],
  pagination: { page: 1, per_page: 200, total_items: 2, total_pages: 1 },
};

describe("AdminCopClassificationsSection", () => {
  beforeEach(() => {
    vi.mocked(api.fetchTargetVersions).mockResolvedValue(["18.5.0", "19.3.15"]);
    vi.mocked(api.fetchCookstyleCops).mockResolvedValue(RESPONSE);
    vi.mocked(api.setCopClassification).mockResolvedValue({ status: "ok" });
    vi.mocked(api.deleteCopClassification).mockResolvedValue({ status: "ok" });
  });

  it("renders the heading and loads cops for the first target version", async () => {
    render(<AdminCopClassificationsSection />);
    await waitFor(() =>
      expect(screen.getByText("Lint/DeprecatedClassMethods")).toBeInTheDocument(),
    );
    expect(api.fetchCookstyleCops).toHaveBeenCalledWith(
      expect.objectContaining({ target_chef_version: "18.5.0" }),
    );
  });

  it("renders a target-version selector with all versions", async () => {
    render(<AdminCopClassificationsSection />);
    const select = await screen.findByRole("combobox", { name: /target chef version/i });
    expect(select).toHaveValue("18.5.0");
    fireEvent.change(select, { target: { value: "19.3.15" } });
    await waitFor(() =>
      expect(api.fetchCookstyleCops).toHaveBeenLastCalledWith(
        expect.objectContaining({ target_chef_version: "19.3.15" }),
      ),
    );
  });

  it("filters the list by search text", async () => {
    render(<AdminCopClassificationsSection />);
    await waitFor(() =>
      expect(screen.getByText("Lint/DeprecatedClassMethods")).toBeInTheDocument(),
    );
    fireEvent.change(screen.getByPlaceholderText(/search cops/i), {
      target: { value: "ResourceWithout" },
    });
    expect(
      screen.getByText("Chef/Deprecations/ResourceWithoutUnifiedTrue"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Lint/DeprecatedClassMethods")).not.toBeInTheDocument();
  });

  it("shows a reset button for operator overrides and calls delete", async () => {
    render(<AdminCopClassificationsSection />);
    await waitFor(() =>
      expect(screen.getByText("Lint/DeprecatedClassMethods")).toBeInTheDocument(),
    );
    const resetBtn = screen.getByRole("button", { name: /reset Lint\/DeprecatedClassMethods/i });
    fireEvent.click(resetBtn);
    await waitFor(() =>
      expect(api.deleteCopClassification).toHaveBeenCalledWith(
        "Lint/DeprecatedClassMethods",
        "18.5.0",
      ),
    );
  });

  it("opens the override editor and saves a new classification", async () => {
    render(<AdminCopClassificationsSection />);
    await waitFor(() =>
      expect(
        screen.getByText("Chef/Deprecations/ResourceWithoutUnifiedTrue"),
      ).toBeInTheDocument(),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: /override Chef\/Deprecations\/ResourceWithoutUnifiedTrue/i,
      }),
    );
    fireEvent.change(screen.getByRole("combobox", { name: /classification/i }), {
      target: { value: "blocker" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(api.setCopClassification).toHaveBeenCalledWith(
        "Chef/Deprecations/ResourceWithoutUnifiedTrue",
        expect.objectContaining({
          target_chef_version: "18.5.0",
          classification: "blocker",
        }),
      ),
    );
  });

  it("requests all known cops by default (no triggered_only)", async () => {
    render(<AdminCopClassificationsSection />);
    await waitFor(() =>
      expect(screen.getByText("Lint/DeprecatedClassMethods")).toBeInTheDocument(),
    );
    expect(api.fetchCookstyleCops).toHaveBeenLastCalledWith(
      expect.not.objectContaining({ triggered_only: true }),
    );
  });

  it("narrows to triggered cops when the checkbox is ticked", async () => {
    render(<AdminCopClassificationsSection />);
    await waitFor(() =>
      expect(screen.getByText("Lint/DeprecatedClassMethods")).toBeInTheDocument(),
    );
    fireEvent.click(screen.getByLabelText(/only cops that have triggered/i));
    await waitFor(() =>
      expect(api.fetchCookstyleCops).toHaveBeenLastCalledWith(
        expect.objectContaining({ triggered_only: true }),
      ),
    );
  });

  it("filters by classification via the filter bar", async () => {
    render(<AdminCopClassificationsSection />);
    await waitFor(() =>
      expect(screen.getByText("Lint/DeprecatedClassMethods")).toBeInTheDocument(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Blockers" }));
    await waitFor(() =>
      expect(api.fetchCookstyleCops).toHaveBeenLastCalledWith(
        expect.objectContaining({ classification: "blocker" }),
      ),
    );
  });
});
