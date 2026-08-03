// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import { GlobalFilterProvider } from "../context/GlobalFilterContext";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    fetchGitRepos: vi.fn(),
    fetchSavedFilters: vi.fn(),
    fetchOwners: vi.fn(),
  };
});

const mockUseAuth = vi.fn();
vi.mock("../context/AuthContext", () => ({ useAuth: () => mockUseAuth() }));

import { GitReposPage } from "./GitReposPage";

function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter>
      <GlobalFilterProvider>{children}</GlobalFilterProvider>
    </MemoryRouter>
  );
}

const repos = [
  {
    id: "cron",
    name: "cron",
    git_repo_url: "git@github.com:chef-cookbooks/cron",
    has_test_suite: true,
    clone_status: "ok",
    cookstyle_status: "untested" as const,
    human_verdict: "broken" as const,
    human_verdict_reason: "Its a totally bogus test",
  },
  {
    id: "logrotate",
    name: "logrotate",
    git_repo_url: "git@github.com:chef-cookbooks/logrotate",
    has_test_suite: true,
    clone_status: "ok",
    cookstyle_status: "untested" as const,
  },
];

describe("GitReposPage — the team verdict filter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: repos,
      pagination: { page: 1, per_page: 25, total_items: 2, total_pages: 1 },
    });
    if (vi.isMockFunction(api.fetchSavedFilters)) {
      vi.mocked(api.fetchSavedFilters).mockResolvedValue({ data: [] });
    }
    vi.mocked(api.fetchOwners).mockResolvedValue({
      data: [
        {
          name: "alice.brown",
          display_name: "Alice Brown",
          owner_type: "person",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      pagination: { page: 1, per_page: 50, total_items: 1, total_pages: 1 },
    });
  });

  // Marking a repo somebody has overruled is the point: without it the list
  // goes on showing the scan's verdict as though nobody had disagreed.
  it("marks a repo a person has overruled", async () => {
    render(<GitReposPage />, { wrapper: Wrapper });

    expect(await screen.findByText("cron")).toBeInTheDocument();
    const markers = await screen.findAllByTestId("overruled-marker");
    expect(markers).toHaveLength(1);
    expect(markers[0]).toHaveTextContent(/person says broken/i);
  });

  // The bug this test exists for: selecting the filter updated the chip and
  // the count but never reached the request, so the list stayed unfiltered
  // while appearing to be filtered — the worst of both.
  it("sends the chosen verdict to the API", async () => {
    const user = userEvent.setup();
    render(<GitReposPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchGitRepos).toHaveBeenCalled());
    vi.mocked(api.fetchGitRepos).mockClear();

    await user.click(screen.getByRole("button", { name: /Team verdict/i }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Person says broken/i }),
    );

    await waitFor(() => {
      expect(api.fetchGitRepos).toHaveBeenCalledWith(
        expect.objectContaining({ human_verdict: "broken" }),
      );
    });
  });

  // Both sides selected is asking whether anybody has an opinion at all, not
  // for a pair of verdicts.
  it("asks for any opinion when both sides are selected", async () => {
    const user = userEvent.setup();
    render(<GitReposPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchGitRepos).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /Team verdict/i }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Person says broken/i }),
    );
    await user.click(
      await screen.findByRole("checkbox", { name: /Person says OK/i }),
    );

    await waitFor(() => {
      expect(api.fetchGitRepos).toHaveBeenLastCalledWith(
        expect.objectContaining({ human_verdict: "any" }),
      );
    });
  });

  // Clearing has to reach the request too, or the list stays filtered while
  // claiming not to be.
  it("drops the filter from the request when cleared", async () => {
    const user = userEvent.setup();
    render(<GitReposPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchGitRepos).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /Team verdict/i }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Person says broken/i }),
    );
    await waitFor(() => {
      expect(api.fetchGitRepos).toHaveBeenLastCalledWith(
        expect.objectContaining({ human_verdict: "broken" }),
      );
    });

    await user.click(screen.getByRole("button", { name: /Clear \(/i }));

    await waitFor(() => {
      const last = vi.mocked(api.fetchGitRepos).mock.calls.at(-1)?.[0];
      expect(last?.human_verdict).toBeUndefined();
    });
  });
});

// ---------------------------------------------------------------------------
// The ownership filter. The backend has answered "what's mine" and "what has
// nobody" since 2026-08-02; until now no screen could ask either question.
// ---------------------------------------------------------------------------

describe("GitReposPage — the ownership filter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchGitRepos).mockResolvedValue({
      data: repos,
      pagination: { page: 1, per_page: 25, total_items: 2, total_pages: 1 },
    });
    if (vi.isMockFunction(api.fetchSavedFilters)) {
      vi.mocked(api.fetchSavedFilters).mockResolvedValue({ data: [] });
    }
    vi.mocked(api.fetchOwners).mockResolvedValue({
      data: [
        {
          name: "alice.brown",
          display_name: "Alice Brown",
          owner_type: "person",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      pagination: { page: 1, per_page: 50, total_items: 1, total_pages: 1 },
    });
  });

  it("asks the API for one person's repos", async () => {
    const user = userEvent.setup();
    render(<GitReposPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchGitRepos).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );

    await waitFor(() => {
      expect(api.fetchGitRepos).toHaveBeenLastCalledWith(
        expect.objectContaining({ owner: "alice.brown" }),
      );
    });
  });

  // The question the measurement was waiting on: the repos nobody has been
  // made responsible for.
  it("asks the API for the repos with nobody", async () => {
    const user = userEvent.setup();
    render(<GitReposPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchGitRepos).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));

    await waitFor(() => {
      const last = vi.mocked(api.fetchGitRepos).mock.calls.at(-1)?.[0];
      expect(last?.unowned).toBe("true");
      expect(last?.owner).toBeUndefined();
    });
  });

  // The API returns 400 when both are sent. Asking for one has to withdraw the
  // other, so the pair can never leave the page together.
  it("never sends an owner and the no-owner question together", async () => {
    const user = userEvent.setup();
    render(<GitReposPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchGitRepos).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));

    await waitFor(() => {
      const last = vi.mocked(api.fetchGitRepos).mock.calls.at(-1)?.[0];
      expect(last?.unowned).toBe("true");
      expect(last?.owner).toBeUndefined();
    });

    // Every request, not just the last one.
    for (const [call] of vi.mocked(api.fetchGitRepos).mock.calls) {
      expect(call?.owner && call?.unowned).toBeFalsy();
    }
  });

  // Seen in the running app with seven owners selected: chips hanging off the
  // control widened it and shoved CookStyle, TK and Clone to the right. They
  // belong under the bar, where they can wrap without moving anything.
  it("shows the chosen owner as a chip below the filter bar, not inside it", async () => {
    const user = userEvent.setup();
    render(<GitReposPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchGitRepos).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(
      await screen.findByRole("checkbox", { name: /Alice Brown/ }),
    );

    const chip = await screen.findByRole("button", {
      name: "Remove alice.brown",
    });
    expect(screen.getByTestId("filter-bar")).not.toContainElement(chip);

    // And removing one person only removes that person's filter.
    await user.click(chip);
    await waitFor(() => {
      expect(vi.mocked(api.fetchGitRepos).mock.calls.at(-1)?.[0]?.owner).toBe(
        undefined,
      );
    });
  });

  it("drops the ownership filter from the request when cleared", async () => {
    const user = userEvent.setup();
    render(<GitReposPage />, { wrapper: Wrapper });

    await waitFor(() => expect(api.fetchGitRepos).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: /^Owner/ }));
    await user.click(await screen.findByRole("checkbox", { name: /No owner/i }));
    await waitFor(() => {
      expect(vi.mocked(api.fetchGitRepos).mock.calls.at(-1)?.[0]?.unowned).toBe(
        "true",
      );
    });

    await user.click(screen.getByRole("button", { name: /Clear \(/i }));

    await waitFor(() => {
      const last = vi.mocked(api.fetchGitRepos).mock.calls.at(-1)?.[0];
      expect(last?.unowned).toBeUndefined();
      expect(last?.owner).toBeUndefined();
    });
  });
});
