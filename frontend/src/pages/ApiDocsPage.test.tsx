// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as api from "../api";
import { ApiDocsPage } from "./ApiDocsPage";

vi.mock("../api");

const doc = {
  openapi: "3.0.3",
  info: {
    title: "Chef Migration Metrics",
    version: "2.21.12",
    description: "What is stopping this estate moving to the target version.",
  },
  paths: {
    "/api/v1/cookbooks": {
      get: {
        operationId: "getCookbooks",
        summary: "Every cookbook with its verdict.",
        "x-required-role": "authenticated",
      },
    },
    "/api/v1/failure-register": {
      get: {
        operationId: "getFailureRegister",
        summary: "Findings a person recorded after seeing something run.",
        "x-required-role": "authenticated",
      },
      post: {
        operationId: "postFailureRegister",
        summary: "Record what you saw when you ran it.",
        "x-required-role": "operator",
      },
    },
    "/api/v1/admin/users": {
      get: {
        operationId: "getAdminUsers",
        summary: "The local accounts that can sign in.",
        "x-required-role": "admin",
      },
    },
  },
};

describe("ApiDocsPage", () => {
  beforeEach(() => {
    vi.mocked(api.fetchApiDocument).mockResolvedValue(doc as never);
  });

  it("names the service and the version the description came from", async () => {
    render(<ApiDocsPage />);
    await waitFor(() =>
      expect(screen.getByText(/Chef Migration Metrics/)).toBeInTheDocument(),
    );
    // The version matters: a description is only true of the build that served
    // it, and somebody comparing against a running service needs to know which.
    expect(screen.getByText(/2\.21\.12/)).toBeInTheDocument();
  });

  it("lists every address with its method and summary", async () => {
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));

    // One row per operation, so a path answering GET and POST appears twice.
    // That is the point: the two need different access and do different things.
    expect(screen.getAllByText("/api/v1/failure-register")).toHaveLength(2);
    expect(
      screen.getByText("Record what you saw when you ran it."),
    ).toBeInTheDocument();
    expect(screen.getAllByText("post")).toHaveLength(1);
  });

  it("shows the access each operation needs", async () => {
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/admin/users"));

    // Asked by the badge's own label rather than by text: "admin" is also a
    // group name, and matching on the word alone would pass on the heading.
    const badges = screen
      .getAllByTitle("The access this operation needs")
      .map((el) => el.textContent);
    expect(badges).toContain("admin");
    expect(badges).toContain("operator");
    expect(badges.filter((b) => b === "authenticated")).toHaveLength(2);
  });

  it("groups addresses so 195 of them can be found", async () => {
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));
    expect(
      screen.getByRole("heading", { name: /^cookbooks$/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /^admin$/i }),
    ).toBeInTheDocument();
  });

  it("filters by path", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));

    await user.type(screen.getByRole("searchbox"), "failure");

    expect(screen.getAllByText("/api/v1/failure-register")).toHaveLength(2);
    expect(screen.queryByText("/api/v1/cookbooks")).not.toBeInTheDocument();
  });

  it("filters by what an operation is for, not only its address", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));

    // Somebody looking for where to sign in does not know the address; that is
    // the whole reason they opened this page.
    await user.type(screen.getByRole("searchbox"), "local accounts");

    expect(screen.getByText("/api/v1/admin/users")).toBeInTheDocument();
    expect(screen.queryByText("/api/v1/cookbooks")).not.toBeInTheDocument();
  });

  it("says so when a search matches nothing", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));

    await user.type(screen.getByRole("searchbox"), "nothing matches this");
    expect(screen.getByText(/no addresses match/i)).toBeInTheDocument();
  });

  it("offers the raw document, because that is what tooling consumes", async () => {
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));

    const link = screen.getByRole("link", { name: /openapi\.json/i });
    expect(link).toHaveAttribute("href", "/api/v1/openapi.json");
  });

  it("opens a detail panel for the operation that was clicked", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/admin/users"));

    await user.click(screen.getByRole("button", { name: /get \/api\/v1\/admin\/users/i }));

    const panel = screen.getByRole("complementary");
    expect(within(panel).getByText("getAdminUsers")).toBeInTheDocument();
    expect(
      within(panel).getByText("The local accounts that can sign in."),
    ).toBeInTheDocument();
  });

  it("gives a runnable curl with the token passed as a variable", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getAllByText("/api/v1/failure-register"));

    await user.click(
      screen.getByRole("button", { name: /post \/api\/v1\/failure-register/i }),
    );

    const panel = screen.getByRole("complementary");
    const curl = within(panel).getByTestId("curl-example").textContent ?? "";
    // A literal secret in an example is a secret somebody pastes into a ticket.
    expect(curl).toContain("$APITOKEN");
    expect(curl).toContain("Authorization: Bearer $APITOKEN");
    expect(curl).toContain("-X POST");
    expect(curl).toContain("/api/v1/failure-register");
  });

  it("does not send a body on a read", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));

    await user.click(screen.getByRole("button", { name: /get \/api\/v1\/cookbooks/i }));
    const curl = screen.getByTestId("curl-example").textContent ?? "";
    expect(curl).not.toContain("-d ");
    expect(curl).not.toContain("-X GET");
  });

  it("shows path parameters as placeholders to be substituted", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchApiDocument).mockResolvedValue({
      ...doc,
      paths: {
        "/api/v1/cookbooks/{name}": {
          get: {
            operationId: "getCookbooksName",
            summary: "One cookbook.",
            "x-required-role": "authenticated",
            parameters: [
              { name: "name", in: "path", required: true, schema: { type: "string" } },
            ],
          },
        },
      },
    } as never);

    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks/{name}"));
    await user.click(
      screen.getByRole("button", { name: /get \/api\/v1\/cookbooks/i }),
    );

    const panel = screen.getByRole("complementary");
    expect(within(panel).getByText("name")).toBeInTheDocument();
    expect(within(panel).getByText(/path/i)).toBeInTheDocument();
    expect(screen.getByTestId("curl-example").textContent).toContain(
      "cookbooks/NAME",
    );
  });

  it("says plainly what the description does not yet carry", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getAllByText("/api/v1/failure-register"));

    await user.click(
      screen.getByRole("button", { name: /post \/api\/v1\/failure-register/i }),
    );

    // The body is not described. Saying so beats an empty section, which reads
    // as "this call takes nothing" — and would send somebody to a 400.
    const panel = screen.getByRole("complementary");
    expect(
      within(panel).getByText(/request body is not described/i),
    ).toBeInTheDocument();
  });

  it("copies the curl to the clipboard", async () => {
    // setup() installs its own clipboard stub, so ours has to go on after it
    // or it is silently replaced and the assertion sees no calls.
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));
    await user.click(screen.getByRole("button", { name: /get \/api\/v1\/cookbooks/i }));
    await user.click(screen.getByRole("button", { name: /copy/i }));

    expect(writeText).toHaveBeenCalledWith(
      expect.stringContaining("$APITOKEN"),
    );
  });

  it("reports a failure to load rather than an empty API", async () => {
    vi.mocked(api.fetchApiDocument).mockRejectedValue(
      new Error("service is away"),
    );
    render(<ApiDocsPage />);
    await waitFor(() =>
      expect(screen.getByText(/service is away/i)).toBeInTheDocument(),
    );
    // An empty list would read as "this service serves nothing", which is a
    // different and wrong statement.
    expect(screen.queryByText(/no addresses match/i)).not.toBeInTheDocument();
  });
});
