// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render,
  screen,
  waitFor,
  within,
  fireEvent,
  cleanup,
} from "@testing-library/react";
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
        responses: {
          "200": {
            description: "The answer.",
            content: {
              "application/json": {
                schema: {
                  type: "object",
                  properties: {
                    data: {
                      type: "array",
                      items: { $ref: "#/components/schemas/webapi.cookbookResp" },
                    },
                    pagination: { type: "object", properties: { page: { type: "integer" } } },
                  },
                },
              },
            },
          },
        },
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
        requestBody: {
          required: true,
          content: {
            "application/json": {
              schema: { $ref: "#/components/schemas/webapi.recordVerdictRequest" },
            },
          },
        },
      },
    },
    "/api/v1/admin/users": {
      get: {
        operationId: "getAdminUsers",
        summary: "The local accounts that can sign in.",
        "x-required-role": "admin",
      },
    },
    "/api/v1/admin/config/git-urls": {
      put: {
        operationId: "putAdminConfigGitUrls",
        summary: "Change how cookbook names are turned into repository addresses.",
        "x-required-role": "admin",
        requestBody: {
          required: true,
          content: {
            "application/json": { schema: { type: "array", items: { type: "string" } } },
          },
        },
      },
      get: {
        operationId: "getAdminConfigGitUrls",
        summary: "How cookbook names are turned into repository addresses.",
        "x-required-role": "admin",
        responses: {
          "200": {
            description: "The answer.",
            content: {
              "application/json": {
                schema: {
                  type: "array",
                  items: { $ref: "#/components/schemas/webapi.cookbookResp" },
                },
              },
            },
          },
        },
      },
    },
    "/api/v1/cookstyle/cops/{cop_name}/cookbooks": {
      get: {
        operationId: "getCookstyleCopsCopNameCookbooks",
        summary: "Which cookbooks this rule fires on.",
        "x-required-role": "authenticated",
        responses: {
          "200": {
            description: "The answer.",
            content: {
              "application/json": {
                schema: {
                  oneOf: [
                    {
                      type: "object",
                      properties: { grouped: { type: "boolean" }, cop_name: { type: "string" } },
                    },
                    {
                      type: "object",
                      properties: { cop_name: { type: "string" } },
                    },
                  ],
                },
              },
            },
          },
        },
      },
    },
  },
  components: {
    schemas: {
      "webapi.recordVerdictRequest": {
        type: "object",
        properties: {
          subject_name: { type: "string" },
          verdict: { type: "string" },
          evidence: { type: "string" },
          holder: { $ref: "#/components/schemas/webapi.holder" },
        },
      },
      "webapi.holder": {
        type: "object",
        properties: { holder_ref: { type: "string" } },
      },
      "webapi.cookbookResp": {
        type: "object",
        properties: { name: { type: "string" }, tk_status: { type: "string" } },
      },
    },
  },
};

describe("ApiDocsPage", () => {
  beforeEach(() => {
    window.localStorage.clear();
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
    expect(badges.filter((b) => b === "authenticated")).toHaveLength(3);
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

  it("shows which query parameters a list accepts, and their limits", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchApiDocument).mockResolvedValue({
      ...doc,
      paths: {
        "/api/v1/cookbooks": {
          get: {
            operationId: "getCookbooks",
            summary: "Every cookbook with its verdict.",
            "x-required-role": "authenticated",
            parameters: [
              {
                name: "page",
                in: "query",
                required: false,
                schema: { type: "integer", default: 1, minimum: 1 },
              },
              {
                name: "per_page",
                in: "query",
                required: false,
                schema: { type: "integer", default: 50, maximum: 500 },
              },
            ],
          },
        },
      },
    });
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));

    await user.click(screen.getByRole("button", { name: /get \/api\/v1\/cookbooks/i }));

    const panel = screen.getByRole("complementary");
    expect(within(panel).getByText("page")).toBeInTheDocument();
    expect(within(panel).getByText("per_page")).toBeInTheDocument();
    // Without "query" beside them these read as path segments, which is a
    // different call entirely.
    expect(within(panel).getAllByText("query").length).toBe(2);
    // Saying it caps at 500 is the difference between a caller writing a loop
    // and a caller asking for the estate in one go and quietly getting 500.
    expect(panel.textContent).toContain("500");
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

  it("shows a write's fields, so somebody can build the call without our source", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getAllByText("/api/v1/failure-register"));

    await user.click(
      screen.getByRole("button", { name: /post \/api\/v1\/failure-register/i }),
    );

    const panel = screen.getByRole("complementary");
    for (const field of ["subject_name", "verdict", "evidence"]) {
      expect(within(panel).getByText(field)).toBeInTheDocument();
    }
    // Sending nothing is never right on a call that declares a body.
    expect(within(panel).queryByText(/body is not described/i)).toBeNull();
  });

  it("follows a reference rather than showing a caller our internal type name", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getAllByText("/api/v1/failure-register"));

    await user.click(
      screen.getByRole("button", { name: /post \/api\/v1\/failure-register/i }),
    );

    const panel = screen.getByRole("complementary");
    expect(panel.textContent).not.toContain("#/components/schemas");
    // A nested type resolves too, or the field reads as having no type at all.
    expect(within(panel).getByText("holder")).toBeInTheDocument();
    expect(within(panel).getByText("holder_ref")).toBeInTheDocument();
  });

  it("puts the fields into the curl, so the example is runnable", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getAllByText("/api/v1/failure-register"));

    await user.click(
      screen.getByRole("button", { name: /post \/api\/v1\/failure-register/i }),
    );

    const curl = screen.getByTestId("curl-example").textContent ?? "";
    expect(curl).toContain("subject_name");
    // The empty placeholder was right when nothing was described; now it would
    // be a worked example that fails.
    expect(curl).not.toContain("-d '{}'");
  });

  it("still says plainly when a write really is described as taking nothing", async () => {
    const user = userEvent.setup();
    vi.mocked(api.fetchApiDocument).mockResolvedValue({
      ...doc,
      paths: {
        "/api/v1/cookbooks/{name}/rescan": {
          post: {
            operationId: "postCookbooksNameRescan",
            summary: "Scan this cookbook again.",
            "x-required-role": "operator",
          },
        },
      },
    });
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks/{name}/rescan"));

    await user.click(
      screen.getByRole("button", { name: /post \/api\/v1\/cookbooks/i }),
    );

    // Nothing to send is a real answer and has to read as one, not as a gap.
    const panel = screen.getByRole("complementary");
    expect(within(panel).getByText(/reads nothing from the body/i)).toBeInTheDocument();
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

  async function openPanel() {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));
    await user.click(
      screen.getByRole("button", { name: "GET /api/v1/cookbooks" }),
    );
    return user;
  }

  it("offers a way to resize the split once a panel is open", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));
    // Nothing to resize until there are two panes.
    expect(screen.queryByRole("separator")).not.toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "GET /api/v1/cookbooks" }),
    );
    expect(screen.getByRole("separator")).toBeInTheDocument();
  });

  it("remembers the width it was left at", async () => {
    const user = await openPanel();
    screen.getByRole("separator").focus();
    await user.keyboard("{ArrowLeft}{ArrowLeft}");
    const chosen = screen.getByRole("complementary").style.width;

    cleanup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));
    await user.click(
      screen.getByRole("button", { name: "GET /api/v1/cookbooks" }),
    );

    expect(screen.getByRole("complementary").style.width).toBe(chosen);
  });

  it("ignores a stored width that is not usable", async () => {
    // Hand-edited, corrupted, or left by an older build with different bounds.
    window.localStorage.setItem("cmm.apiDocs.panelWidth", "not a number");
    await openPanel();
    const width = parseInt(
      screen.getByRole("complementary").style.width,
      10,
    );
    expect(width).toBeGreaterThanOrEqual(320);
    expect(width).toBeLessThanOrEqual(900);
  });

  it("resizes by keyboard, not only by dragging", async () => {
    const user = await openPanel();
    const panel = screen.getByRole("complementary");
    const before = panel.style.width;

    screen.getByRole("separator").focus();
    await user.keyboard("{ArrowLeft}");

    // Left widens the panel: the divider moves left, so the right pane grows.
    expect(panel.style.width).not.toBe(before);
    expect(parseInt(panel.style.width, 10)).toBeGreaterThan(
      parseInt(before, 10),
    );
  });

  it("resizes by dragging the divider", async () => {
    await openPanel();
    const panel = screen.getByRole("complementary");
    const before = parseInt(panel.style.width, 10);
    const separator = screen.getByRole("separator");

    fireEvent.pointerDown(separator, { clientX: 800 });
    fireEvent.pointerMove(window, { clientX: 700 });
    fireEvent.pointerUp(window, { clientX: 700 });

    // Dragging left by 100 widens the right-hand pane by 100.
    expect(parseInt(panel.style.width, 10)).toBe(before + 100);
  });

  it("will not let either pane be dragged away to nothing", async () => {
    await openPanel();
    const panel = screen.getByRole("complementary");
    const separator = screen.getByRole("separator");

    fireEvent.pointerDown(separator, { clientX: 800 });
    fireEvent.pointerMove(window, { clientX: -5000 });
    fireEvent.pointerUp(window, { clientX: -5000 });
    const widest = parseInt(panel.style.width, 10);

    fireEvent.pointerDown(separator, { clientX: 800 });
    fireEvent.pointerMove(window, { clientX: 5000 });
    fireEvent.pointerUp(window, { clientX: 5000 });
    const narrowest = parseInt(panel.style.width, 10);

    // Both ends are bounded, so the list can never be squeezed out of
    // existence and the panel can never become an unreadable sliver.
    expect(widest).toBeLessThanOrEqual(900);
    expect(narrowest).toBeGreaterThanOrEqual(320);
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

  it("shows what a call answers with, so a client has something to decode into", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookbooks"));

    await user.click(screen.getByRole("button", { name: /get \/api\/v1\/cookbooks/i }));

    const panel = screen.getByRole("complementary");
    const response = within(panel).getByTestId("response-shape");
    // The envelope, and the rows inside it. A caller that is only shown "data"
    // and "pagination" has been told the shape of the wrapper and nothing
    // about what it wraps, which is the part they came for.
    expect(within(response).getByText("data")).toBeInTheDocument();
    expect(within(response).getByText("pagination")).toBeInTheDocument();
    expect(within(response).getByText("tk_status")).toBeInTheDocument();
  });

  it("says plainly when nothing describes the answer, rather than showing nothing", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/admin/users"));

    await user.click(screen.getByRole("button", { name: /get \/api\/v1\/admin\/users/i }));

    const panel = screen.getByRole("complementary");
    // An empty section reads as "answers nothing", which is a different claim
    // and a wrong one.
    expect(within(panel).getByText(/not described yet/i)).toBeInTheDocument();
  });


  it("says when an answer comes back in one of several shapes", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/cookstyle/cops/{cop_name}/cookbooks"));

    await user.click(
      screen.getByRole("button", { name: /get \/api\/v1\/cookstyle\/cops/i }),
    );

    const panel = screen.getByRole("complementary");
    const response = within(panel).getByTestId("response-shape");
    // A caller has to branch, so both shapes are shown rather than one of them
    // chosen for them.
    expect(within(response).getByText(/one of 2/i)).toBeInTheDocument();
    expect(within(response).getByText("grouped")).toBeInTheDocument();
  });


  it("describes a body that is a list, rather than calling it undescribed", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getAllByText("/api/v1/admin/config/git-urls"));

    await user.click(
      screen.getByRole("button", { name: /put \/api\/v1\/admin\/config\/git-urls/i }),
    );

    const panel = screen.getByRole("complementary");
    // The description does say what this takes — a list of strings. Reading
    // "its fields are not described" sends somebody to the network tab for
    // something they were already told.
    expect(within(panel).queryByText(/fields are not described/i)).not.toBeInTheDocument();
    expect(within(panel).getByTestId("request-shape")).toHaveTextContent(/list of string/i);
  });

  it("shows what is inside a list, not just that it is a list", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getAllByText("/api/v1/admin/config/git-urls"));

    await user.click(
      screen.getByRole("button", { name: /get \/api\/v1\/admin\/config\/git-urls/i }),
    );

    const response = within(screen.getByRole("complementary")).getByTestId("response-shape");
    // "A list of object" tells a caller nothing they can decode into.
    expect(within(response).getByText("tk_status")).toBeInTheDocument();
  });


  it("keeps the detail panel reachable when it is taller than the window", async () => {
    const user = userEvent.setup();
    render(<ApiDocsPage />);
    await waitFor(() => screen.getByText("/api/v1/admin/users"));

    await user.click(screen.getByRole("button", { name: /get \/api\/v1\/admin\/users/i }));

    // Asserted on the mechanism rather than the effect: there is no layout in
    // this environment, so nothing here can measure that the last row is
    // reachable. What can be checked is that the panel is bounded and scrolls
    // itself — without both, a panel pinned to the top of a scrolling page
    // hangs its overflow below the fold with no way to get to it, which is
    // exactly what a settings section with thirty fields does.
    const panel = screen.getByRole("complementary");
    expect(panel.className).toMatch(/overflow-y-auto/);
    expect(panel.className).toMatch(/max-h-/);
  });

});
