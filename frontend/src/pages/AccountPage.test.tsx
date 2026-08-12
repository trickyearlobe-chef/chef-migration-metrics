// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { AccountPage } from "./AccountPage";

vi.mock("../api");

const existing = {
  id: "tok-1",
  name: "editor on my laptop",
  can_write: false,
  created_at: "2026-08-01T09:00:00Z",
  last_used_at: "2026-08-11T14:30:00Z",
};

describe("AccountPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchMyApiTokens).mockResolvedValue({
      tokens: [existing],
    } as never);
    vi.mocked(api.createMyApiToken).mockResolvedValue({
      token: {
        id: "tok-2",
        name: "new one",
        can_write: false,
        created_at: "2026-08-12T10:00:00Z",
      },
      secret: "shown once, never again",
      notice: "Copy this now.",
    } as never);
    vi.mocked(api.destroyMyApiToken).mockResolvedValue(undefined as never);
  });

  it("shows the credentials in my name, and when each was last used", async () => {
    render(<AccountPage />);
    await waitFor(() =>
      expect(screen.getByText("editor on my laptop")).toBeInTheDocument(),
    );
    // Somebody deciding whether to destroy one needs to know it is still in
    // use; a name on its own does not tell them.
    expect(screen.getByText(/last used/i)).toBeInTheDocument();
  });

  it("says a credential is read-only when it is", async () => {
    render(<AccountPage />);
    await waitFor(() => screen.getByText("editor on my laptop"));
    expect(screen.getByText(/read only/i)).toBeInTheDocument();
  });

  it("will not create one without a name", async () => {
    const user = userEvent.setup();
    render(<AccountPage />);
    await waitFor(() => screen.getByText("editor on my laptop"));

    await user.click(screen.getByRole("button", { name: /create credential/i }));
    expect(api.createMyApiToken).not.toHaveBeenCalled();
  });

  it("creates a read-only credential unless write access is asked for", async () => {
    const user = userEvent.setup();
    render(<AccountPage />);
    await waitFor(() => screen.getByText("editor on my laptop"));

    await user.type(screen.getByLabelText(/name/i), "new one");
    await user.click(screen.getByRole("button", { name: /create credential/i }));

    await waitFor(() =>
      expect(api.createMyApiToken).toHaveBeenCalledWith("new one", false),
    );
  });

  it("passes write access on when it is asked for", async () => {
    const user = userEvent.setup();
    render(<AccountPage />);
    await waitFor(() => screen.getByText("editor on my laptop"));

    await user.type(screen.getByLabelText(/name/i), "batch of failures");
    await user.click(screen.getByLabelText(/record findings/i));
    await user.click(screen.getByRole("button", { name: /create credential/i }));

    await waitFor(() =>
      expect(api.createMyApiToken).toHaveBeenCalledWith(
        "batch of failures",
        true,
      ),
    );
  });

  it("shows the secret once, and says it will not be shown again", async () => {
    const user = userEvent.setup();
    render(<AccountPage />);
    await waitFor(() => screen.getByText("editor on my laptop"));

    await user.type(screen.getByLabelText(/name/i), "new one");
    await user.click(screen.getByRole("button", { name: /create credential/i }));

    await waitFor(() =>
      expect(screen.getByText("shown once, never again")).toBeInTheDocument(),
    );
    expect(screen.getByText(/cannot be shown again/i)).toBeInTheDocument();
  });

  it("destroys a credential and stops showing it", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchMyApiTokens)
      .mockResolvedValueOnce({ tokens: [existing] } as never)
      .mockResolvedValueOnce({ tokens: [] } as never);

    render(<AccountPage />);
    await waitFor(() => screen.getByText("editor on my laptop"));

    await user.click(screen.getByRole("button", { name: /destroy/i }));
    // Destroying is not undoable, so it is confirmed rather than done on one
    // click — but the confirmation must not be so heavy that somebody who
    // thinks a credential has leaked hesitates.
    await user.click(screen.getByRole("button", { name: /^destroy it$/i }));

    await waitFor(() =>
      expect(api.destroyMyApiToken).toHaveBeenCalledWith("tok-1"),
    );
    await waitFor(() =>
      expect(screen.queryByText("editor on my laptop")).not.toBeInTheDocument(),
    );
  });

  it("reports a failure to load rather than showing an empty list", async () => {
    vi.mocked(api.fetchMyApiTokens).mockRejectedValue(
      new Error("database is away"),
    );
    render(<AccountPage />);
    await waitFor(() =>
      expect(screen.getByText(/database is away/i)).toBeInTheDocument(),
    );
    // An empty list would read as "you have no credentials", which is a
    // different and wrong statement.
    expect(screen.queryByText(/no credentials yet/i)).not.toBeInTheDocument();
  });
});
