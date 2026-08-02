// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return { ...actual, fetchGitRepos: vi.fn(), fetchCookbooks: vi.fn() };
});

import { SubjectPicker } from "./SubjectPicker";

const repo = {
  id: "1",
  name: "cron",
  git_repo_url: "git@github.com:chef-cookbooks/cron",
  has_test_suite: true,
  clone_status: "ok",
};

const cookbookRow = (name: string, version: string) => ({
  id: `${name}-${version}`,
  name,
  version,
  is_active: true,
  is_stale_cookbook: false,
  download_status: "ok",
});

describe("SubjectPicker", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 8, total_items: 0, total_pages: 0 },
    });
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [],
      pagination: { page: 1, per_page: 32, total_items: 0, total_pages: 0 },
    });
  });

  // A cookbook can be real, deployed and broken with no repo collected for it.
  // Those are more likely to be unowned and untested, not less.
  it("offers a cookbook that has no repo", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [cookbookRow("legacy-thing", "1.0.0")],
      pagination: { page: 1, per_page: 32, total_items: 1, total_pages: 1 },
    });
    const onChange = vi.fn();
    render(<SubjectPicker value="" onChange={onChange} />);

    await user.type(screen.getByRole("combobox"), "legacy");
    await user.click(await screen.findByRole("button", { name: /legacy-thing/i }));

    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ name: "legacy-thing", type: "cookbook" }),
      "legacy-thing",
    );
    expect(await screen.findByTestId("subject-confirmed")).toHaveTextContent(
      /no repo collected/i,
    );
  });

  // Where a repo exists it is the subject, because that is where the fix is
  // made — the cookbook of the same name must not be offered as a rival.
  it("offers the repo rather than the cookbook when both exist", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: [repo],
      pagination: { page: 1, per_page: 8, total_items: 1, total_pages: 1 },
    });
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [cookbookRow("cron", "1.0.0"), cookbookRow("cron", "2.0.0")],
      pagination: { page: 1, per_page: 32, total_items: 2, total_pages: 1 },
    });
    render(<SubjectPicker value="" onChange={vi.fn()} />);

    await user.type(screen.getByRole("combobox"), "cron");

    const options = await screen.findAllByRole("button", { name: /cron/i });
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent(/repo/i);
  });

  // The register is never keyed on a version, so several versions of one
  // cookbook are one choice, not a list of near-identical ones.
  it("collapses a cookbook's versions to a single choice", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchCookbooks).mockResolvedValue({
      data: [
        cookbookRow("legacy-thing", "1.0.0"),
        cookbookRow("legacy-thing", "2.0.0"),
        cookbookRow("legacy-thing", "3.1.4"),
      ],
      pagination: { page: 1, per_page: 32, total_items: 3, total_pages: 1 },
    });
    render(<SubjectPicker value="" onChange={vi.fn()} />);

    await user.type(screen.getByRole("combobox"), "legacy");

    const options = await screen.findAllByRole("button", { name: /legacy-thing/i });
    expect(options).toHaveLength(1);
  });

  // Nothing collected matches, and the reason says why rather than showing an
  // empty box somebody might read as "still loading".
  it("says why nothing matched", async () => {
    const user = userEvent.setup();
    render(<SubjectPicker value="" onChange={vi.fn()} />);

    await user.type(screen.getByRole("combobox"), "nonesuch");

    expect(
      await screen.findByText(/Only repos and cookbooks CMM has collected/i),
    ).toBeInTheDocument();
  });

  // A name typed and never chosen is reported unconfirmed, which is what stops
  // the form recording a verdict that would change nobody's readiness.
  it("reports a typed name as unconfirmed", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<SubjectPicker value="" onChange={onChange} />);

    await user.type(screen.getByRole("combobox"), "made-up");

    expect(onChange).toHaveBeenLastCalledWith(null, "made-up");
  });
});
