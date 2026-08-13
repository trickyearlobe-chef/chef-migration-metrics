// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { OwnershipConnectionPanel } from "./OwnershipConnectionPanel";
import * as api from "../api";

// journeys/ownership-connection.md — the panel where a connection is set up.
//
// Everything here is about what the administrator can SEE. The composing, the
// escaping and the five outcomes are measured on the server against real
// databases; what these hold is that the answers reach a screen, because a
// connection nobody can read is one nobody can correct.

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    listOwnershipConnections: vi.fn(),
    saveOwnershipConnection: vi.fn(),
    deleteOwnershipConnection: vi.fn(),
    showOwnershipConnection: vi.fn(),
    testOwnershipConnection: vi.fn(),
    fetchCredentials: vi.fn(),
  };
});

const stored = {
  name: "cmdb-connection",
  driver: "sqlserver",
  connection: `sqlserver://EXAMPLECORP\\svcaccount:${api.PASSWORD_MARKER}@dbhost.example.com:1433?database=cmdb`,
  password_credential: "cmdb-password",
};

beforeEach(() => {
  vi.mocked(api.listOwnershipConnections).mockResolvedValue({ data: [stored] });
  vi.mocked(api.fetchCredentials).mockResolvedValue({
    data: [{ name: "cmdb-password", credential_type: "generic" }],
  } as never);
});

function renderPanel(value = "") {
  const onChange = vi.fn();
  render(<OwnershipConnectionPanel value={value} onChange={onChange} />);
  return onChange;
}

async function startNew(user: ReturnType<typeof userEvent.setup>) {
  renderPanel();
  await user.click(await screen.findByRole("button", { name: /Set up a new one/ }));
}

// "The address, the database, the account and the domain in plain view, and
// editable."
it("shows a stored connection rather than hiding it", async () => {
  renderPanel("cmdb-connection");
  const shown = await screen.findByText(/dbhost\.example\.com/);
  expect(shown).toHaveTextContent("EXAMPLECORP");
  expect(shown).toHaveTextContent("database=cmdb");
  // The one value that is never shown, because it is the one nobody can check.
  expect(shown).not.toHaveTextContent("password=");
});

// "I say where it goes, and the screen tells me how to say it. But a marker I
// am expected to know about and can read nowhere is just a new thing to get
// wrong."
it("says how to mark where the password goes", async () => {
  const user = userEvent.setup();
  await startNew(user);

  expect(screen.getAllByText(api.PASSWORD_MARKER).length).toBeGreaterThan(0);
});

// "A connection proposed, not imposed. Show me one that would work for the kind
// of database I picked — then let me change any of it."
describe("a proposed connection", () => {
  it("offers one for the database chosen, and follows the choice", async () => {
    const user = userEvent.setup();
    await startNew(user);

    const field = screen.getByRole("textbox", { name: /Connection string/ }) as HTMLTextAreaElement;
    expect(field.value).toContain("sqlserver://");
    // The transport options travel with it. They were measured against a real
    // server, and are the thing nobody should have to discover twice.
    expect(field.value).toContain("encrypt=true");

    // The proposal is URL-shaped, so which database it is has been read from
    // the scheme and is not asked. Writing a keyword connection is what brings
    // the question back.
    await user.clear(field);
    await user.type(field, "server=mine;database=cmdb");
    await user.selectOptions(screen.getByRole("combobox", { name: /Database/ }), "postgres");
    expect((screen.getByRole("textbox", { name: /Connection string/ }) as HTMLTextAreaElement).value)
      .toBe("server=mine;database=cmdb");
  });

  it("does not overwrite what I have written", async () => {
    const user = userEvent.setup();
    await startNew(user);

    const field = screen.getByRole("textbox", { name: /Connection string/ }) as HTMLTextAreaElement;
    await user.clear(field);
    await user.type(field, `server=mine;database=cmdb;user id=svc;password=${api.PASSWORD_MARKER}`);
    // Changing the database must not throw away a connection somebody pasted
    // in from their own tooling — quiet rewriting is what this screen exists
    // to stop.
    await user.selectOptions(screen.getByRole("combobox", { name: /Database/ }), "postgres");

    expect(screen.getByRole("textbox", { name: /Connection string/ })).toHaveValue(
      `server=mine;database=cmdb;user id=svc;password=${api.PASSWORD_MARKER}`,
    );
  });
});

// The scheme already names the database, so asking again is not merely
// redundant: the two can disagree and neither driver says so.
describe("which database it is", () => {
  it("is not asked when the connection already says", async () => {
    const user = userEvent.setup();
    await startNew(user);

    // The proposal is URL-shaped, so the question does not appear.
    expect(screen.queryByRole("combobox", { name: /Database/ })).not.toBeInTheDocument();
  });

  it("is asked when the connection carries no scheme", async () => {
    const user = userEvent.setup();
    await startNew(user);

    const field = screen.getByRole("textbox", { name: /Connection string/ });
    await user.clear(field);
    await user.type(field, "server=mine;database=cmdb");

    expect(screen.getByRole("combobox", { name: /Database/ })).toBeInTheDocument();
  });
});

// "To be shown what will actually be sent, with the password masked. This is
// the thing I have been missing."
it("shows what will actually be sent, masked", async () => {
  vi.mocked(api.showOwnershipConnection).mockResolvedValue({
    driver: "sqlserver",
    connection:
      "sqlserver://EXAMPLECORP%5Csvcaccount:********@dbhost.example.com:1433?database=cmdb",
    form: "url",
  });
  const user = userEvent.setup();
  await startNew(user);

  await user.click(screen.getByRole("button", { name: /Show what will be sent/ }));

  const shown = await screen.findByText(/dbhost\.example\.com:1433/);
  expect(shown).toHaveTextContent("********");
  // The encoding of the account is visible rather than behind them: this is
  // the glance that answers whether the account came out wrong.
  expect(shown).toHaveTextContent("EXAMPLECORP%5Csvcaccount");
});

// "A failure that tells me which of the five it was, in the words of whatever
// refused me" — and each of the five is a different person to go and talk to.
describe("testing it", () => {
  it("says which of the five it was, and whose it is", async () => {
    vi.mocked(api.testOwnershipConnection).mockResolvedValue({
      outcome: "untrusted-domain",
      connection: "sqlserver://EXAMPLECORP%5Csvc:********@dbhost.example.com:1433?database=cmdb",
      form: "url",
      detail: "mssql: login error: Login failed. The login is from an untrusted domain.",
    });
    const user = userEvent.setup();
    await startNew(user);
    await user.selectOptions(
      await screen.findByRole("combobox", { name: /Password credential/ }),
      "cmdb-password",
    );

    await user.click(screen.getByRole("button", { name: /Test it/ }));

    expect(await screen.findByText(/not the database's to check/i)).toBeInTheDocument();
    // Whose problem it is, because that is the decision the answer exists for.
    expect(screen.getByText(/directory/i)).toBeInTheDocument();
    // In the words of whatever refused, not tidied into "could not connect".
    expect(screen.getByText(/untrusted domain/)).toBeInTheDocument();
  });

  it("shows what it sent, so a failure can be read rather than guessed at", async () => {
    vi.mocked(api.testOwnershipConnection).mockResolvedValue({
      outcome: "refused",
      connection: "sqlserver://svc:********@dbhost.example.com:1433?database=cmdb",
      form: "url",
      detail: "mssql: login error: Login failed for user 'svc'.",
    });
    const user = userEvent.setup();
    await startNew(user);
    await user.selectOptions(
      await screen.findByRole("combobox", { name: /Password credential/ }),
      "cmdb-password",
    );

    await user.click(screen.getByRole("button", { name: /Test it/ }));

    const shown = await screen.findByText(/dbhost\.example\.com:1433/);
    expect(shown).toHaveTextContent("********");
  });

  it("cannot be asked for before the password is named", async () => {
    const user = userEvent.setup();
    await startNew(user);

    expect(screen.getByRole("button", { name: /Test it/ })).toBeDisabled();
  });
});

// Testing comes before keeping, so a connection is kept only when somebody has
// decided to — but keeping it does not require a passing test, because the
// server may be behind a firewall nobody has opened yet.
it("keeps a connection without demanding a passing test", async () => {
  vi.mocked(api.saveOwnershipConnection).mockResolvedValue(stored);
  const user = userEvent.setup();
  await startNew(user);

  await user.type(screen.getByRole("textbox", { name: /Connection name/ }), "cmdb-connection");
  await user.selectOptions(
    await screen.findByRole("combobox", { name: /Password credential/ }),
    "cmdb-password",
  );
  await user.click(screen.getByRole("button", { name: /Keep it/ }));

  await waitFor(() => {
    expect(api.saveOwnershipConnection).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "cmdb-connection",
        password_credential: "cmdb-password",
      }),
    );
  });
});

// An unreadable list must not render as an empty one: they read the same on
// screen and mean opposite things.
it("says so when the connections cannot be loaded", async () => {
  vi.mocked(api.listOwnershipConnections).mockRejectedValue(new Error("nope"));
  renderPanel();

  expect(
    await screen.findByText(/Could not load the connections/),
  ).toBeInTheDocument();
});
