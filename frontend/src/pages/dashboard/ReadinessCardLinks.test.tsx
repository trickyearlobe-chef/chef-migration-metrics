// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as api from "../../api";
import { ReadinessCard } from "./StatusCards";
import type { ReadinessResponse } from "../../types";

vi.mock("../../api");

const readiness: ReadinessResponse = {
  data: [
    {
      target_chef_version: "19.3.15",
      total_nodes: 10,
      ready_nodes: 6,
      blocked_nodes: 4,
      ready_percent: 60,
    },
  ],
};

describe("ReadinessCard drill-down links", () => {
  beforeEach(() => {
    vi.mocked(api.fetchReadiness).mockResolvedValue(readiness);
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  // The readiness drill-down must NOT carry a target_version param: that maps to
  // NodesPage's invisible deployment filter (the "Clear (2) but 1 chip" bug) and
  // is the wrong scope for readiness (which is keyed off the global target).
  it("links to /nodes?readiness=… with no target_version", async () => {
    render(
      <MemoryRouter>
        <ReadinessCard />
      </MemoryRouter>,
    );
    await waitFor(() =>
      expect(screen.getAllByText(/Ready:/).length).toBeGreaterThan(0),
    );

    const links = screen.getAllByRole("link") as HTMLAnchorElement[];
    const readinessLinks = links.filter((a) =>
      a.getAttribute("href")?.includes("readiness="),
    );
    expect(readinessLinks.length).toBeGreaterThan(0);
    for (const a of readinessLinks) {
      const href = a.getAttribute("href") ?? "";
      expect(href).not.toContain("target_version");
      expect(href).toMatch(/\/nodes\?readiness=(ready|blocked)$/);
    }
  });
});
