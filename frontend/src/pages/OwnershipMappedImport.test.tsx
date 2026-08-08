import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as api from "../api";
import type { IntakeReport, IntakeSourceProfile } from "../types";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof api>("../api");
  return {
    ...actual,
    profileImportSource: vi.fn(),
    profileImportDatabase: vi.fn(),
    listImportDatabaseTables: vi.fn(),
    fetchCredentials: vi.fn(),
    previewOwnershipImport: vi.fn(),
    commitOwnershipImport: vi.fn(),
    createImportMapping: vi.fn(),
  };
});

const mockUseAuth = vi.fn();
vi.mock("../context/AuthContext", () => ({
  useAuth: () => mockUseAuth(),
}));

import { OwnershipMappedImport } from "./OwnershipMappedImport";
import { OwnershipImportPage } from "./OwnershipImportPage";

function Wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter>{children}</MemoryRouter>;
}

const profile: IntakeSourceProfile = {
  columns: [
    { name: "Owner Email", sample_values: ["alice@example.com"], non_empty_pct: 100, distinct_count: 2 },
    { name: "Repo", sample_values: ["web-app", "db-tools"], non_empty_pct: 100, distinct_count: 2 },
    { name: "Business Unit", sample_values: ["acme"], non_empty_pct: 50, distinct_count: 1 },
  ],
  row_count: 2,
  malformed_rows: 0,
  warnings: [],
};

function emptyReport(overrides: Partial<IntakeReport> = {}): IntakeReport {
  return {
    rows: [],
    new_owners: [],
    counts: {},
    alias_conflict_count: 0,
    row_count: 0,
    unmatched_owners: [],
    committed: false,
    created: 0,
    ...overrides,
  };
}

async function uploadFile(user: ReturnType<typeof userEvent.setup>) {
  const file = new File(["Owner Email,Repo\nalice@example.com,web-app\n"], "own.csv", {
    type: "text/csv",
  });
  const input = document.querySelector('input[type="file"]') as HTMLInputElement;
  await user.upload(input, file);
}

describe("OwnershipMappedImport", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({ user: { role: "admin", username: "admin" } });
    vi.mocked(api.profileImportSource).mockResolvedValue(profile);
    vi.mocked(api.previewOwnershipImport).mockResolvedValue(emptyReport());
    vi.mocked(api.commitOwnershipImport).mockResolvedValue(emptyReport({ committed: true }));
  });

  it("profiles the file and shows what it found", async () => {
    const user = userEvent.setup();
    render(<OwnershipMappedImport />, { wrapper: Wrapper });

    await uploadFile(user);

    await waitFor(() => {
      expect(api.profileImportSource).toHaveBeenCalled();
    });
    // Fill rate and distinct count are what let the administrator tell an
    // identifier column from a free-text one without opening the file.
    // The column name also appears in every mapping dropdown, so scope the
    // assertion to the profile table rather than the whole page.
    const profileTable = (await screen.findByText("Filled")).closest("table")!;
    expect(profileTable).toHaveTextContent("Owner Email");
    expect(profileTable).toHaveTextContent("alice@example.com");
    expect(profileTable).toHaveTextContent("50%");
  });

  it("cannot preview until the required fields are mapped", async () => {
    const user = userEvent.setup();
    vi.mocked(api.profileImportSource).mockResolvedValue({
      ...profile,
      // Nothing here matches the guesser's vocabulary, so the form starts empty.
      columns: [{ name: "col_a", sample_values: ["x"], non_empty_pct: 100, distinct_count: 1 }],
    });
    render(<OwnershipMappedImport />, { wrapper: Wrapper });

    await uploadFile(user);

    const preview = await screen.findByRole("button", { name: /preview/i });
    expect(preview).toBeDisabled();
  });

  it("previews without writing anything, and only then offers to import", async () => {
    const user = userEvent.setup();
    render(<OwnershipMappedImport />, { wrapper: Wrapper });

    await uploadFile(user);

    const preview = await screen.findByRole("button", { name: /preview/i });
    await waitFor(() => expect(preview).toBeEnabled());

    // No import button exists before a preview has been run — the user cannot
    // reach a write without seeing what it would do first.
    expect(screen.queryByRole("button", { name: /^Import these/i })).toBeNull();

    await user.click(preview);

    await waitFor(() => expect(api.previewOwnershipImport).toHaveBeenCalled());
    expect(api.commitOwnershipImport).not.toHaveBeenCalled();
    expect(await screen.findByRole("button", { name: /^Import these/i })).toBeInTheDocument();
  });

  it("guesses a starting mapping from the column names", async () => {
    const user = userEvent.setup();
    render(<OwnershipMappedImport />, { wrapper: Wrapper });

    await uploadFile(user);
    await screen.findByText("3. Map the columns");

    const selects = screen.getAllByRole("combobox") as HTMLSelectElement[];
    const values = selects.map((s) => s.value);
    expect(values).toContain("Owner Email");
    expect(values).toContain("Repo");
  });

  it("names the people it could not place, and does not guess for them", async () => {
    const user = userEvent.setup();
    vi.mocked(api.previewOwnershipImport).mockResolvedValue(
      emptyReport({
        row_count: 1,
        counts: { rejected: 1 },
        unmatched_owners: [{ value: "Alice Smyth", count: 1 }],
        rows: [
          {
            source_row: 1,
            malformed: false,
            raw: {},
            owner: "",
            owner_raw: "Alice Smyth",
            entity_type: "git_repo",
            entity_key: "web-app",
            organisation: "",
            notes: "",
            display_name: "Alice Smyth",
            rejected_reason: "unknown_owner",
            owner_match: "fuzzy_suggestion",
            entity_match: "found",
            outcome: "rejected",
            creates_owner: false,
            alias_conflict: false,
            owner_suggestions: [{ owner_name: "asmith", score: 0.82 }],
          },
        ],
      }),
    );
    render(<OwnershipMappedImport />, { wrapper: Wrapper });

    await uploadFile(user);
    const preview = await screen.findByRole("button", { name: /preview/i });
    await waitFor(() => expect(preview).toBeEnabled());
    await user.click(preview);

    // The value appears twice by design: once in the unmatched list and once
    // in the not-imported rows.
    const unmatched = (await screen.findByText("Value in the file")).closest("table")!;
    expect(unmatched).toHaveTextContent("Alice Smyth");
    // The suggestion is offered for a person to confirm, never applied.
    expect(unmatched).toHaveTextContent("asmith");
    expect(screen.getByText(/Owner not recognised/i)).toBeInTheDocument();
  });

  // No similarity score can connect "Fat Tommy" to "Thomas Smith" — only a
  // person can. So the people being added are listed as people, where someone
  // scanning them has a chance to recognise one.
  it("lists the people it would add, by the name a human would recognise", async () => {
    const user = userEvent.setup();
    vi.mocked(api.previewOwnershipImport).mockResolvedValue(
      emptyReport({
        row_count: 2,
        counts: { would_create: 2 },
        new_owners: [
          {
            name: "fat-tommy",
            display_name: "Fat Tommy",
            source_value: "Fat Tommy",
            row_count: 2,
          },
        ],
      }),
    );
    render(<OwnershipMappedImport />, { wrapper: Wrapper });

    await uploadFile(user);
    const preview = await screen.findByRole("button", { name: /preview/i });
    await waitFor(() => expect(preview).toBeEnabled());
    await user.click(preview);

    const table = (await screen.findByText("Name in the file")).closest("table")!;
    expect(table).toHaveTextContent("Fat Tommy");
    // The slug is shown too, but the recognisable name is the point.
    expect(table).toHaveTextContent("fat-tommy");
    // Review is not a gate — the import is still offered.
    expect(screen.getByRole("button", { name: /^Import these/i })).toBeEnabled();
  });

  it("explains that an overlapping owner name does not block the import", async () => {
    const user = userEvent.setup();
    vi.mocked(api.previewOwnershipImport).mockResolvedValue(
      emptyReport({ row_count: 1, counts: { would_create: 1 }, alias_conflict_count: 1 }),
    );
    render(<OwnershipMappedImport />, { wrapper: Wrapper });

    await uploadFile(user);
    const preview = await screen.findByRole("button", { name: /preview/i });
    await waitFor(() => expect(preview).toBeEnabled());
    await user.click(preview);

    expect(
      await screen.findByText(/assignments are still\s+made/i),
    ).toBeInTheDocument();
  });

  it("commits only when asked", async () => {
    const user = userEvent.setup();
    vi.mocked(api.previewOwnershipImport).mockResolvedValue(
      emptyReport({ row_count: 1, counts: { would_create: 1 } }),
    );
    vi.mocked(api.commitOwnershipImport).mockResolvedValue(
      emptyReport({ row_count: 1, counts: { would_create: 1 }, committed: true, created: 1 }),
    );
    render(<OwnershipMappedImport />, { wrapper: Wrapper });

    await uploadFile(user);
    const preview = await screen.findByRole("button", { name: /preview/i });
    await waitFor(() => expect(preview).toBeEnabled());
    await user.click(preview);

    await user.click(await screen.findByRole("button", { name: /^Import these/i }));

    await waitFor(() => expect(api.commitOwnershipImport).toHaveBeenCalled());
    expect(await screen.findByText(/1 assignment imported/i)).toBeInTheDocument();
  });
});

describe("OwnershipImportPage tabs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({ user: { role: "admin", username: "admin" } });
    vi.mocked(api.profileImportSource).mockResolvedValue(profile);
  });

  // The fixed-header flow is the fast lane for files already in CMM's format.
  // Adding the new one must not move it.
  it("shows the fixed-format flow by default", () => {
    render(<OwnershipImportPage />, { wrapper: Wrapper });

    expect(screen.getByText("Import Format")).toBeInTheDocument();
    expect(
      screen.getByText("owner,entity_type,entity_key,organisation,notes"),
    ).toBeInTheDocument();
  });

  it("switches to the mapping flow", async () => {
    const user = userEvent.setup();
    render(<OwnershipImportPage />, { wrapper: Wrapper });

    await user.click(screen.getByRole("button", { name: "File or database" }));

    expect(
      screen.getByText("1. Choose where the owners come from"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Import Format")).toBeNull();
  });

  it("keeps the role gate on both flows", () => {
    mockUseAuth.mockReturnValue({ user: { role: "viewer", username: "v" } });
    render(<OwnershipImportPage />, { wrapper: Wrapper });

    expect(screen.getByText(/Access denied/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "File or database" })).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Importing from a database
//
// The customer's owner list lives in a system of record, not always in an
// export somebody remembered to take. The mapping flow after the source is
// unchanged — only where the rows come from differs.
// ---------------------------------------------------------------------------

describe("importing owners from a database", () => {
  beforeEach(() => {
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchCredentials).mockResolvedValue({
      data: [
        { name: "cmdb-connection", credential_type: "generic" },
        { name: "chef-key", credential_type: "chef_client_key" },
      ],
    } as never);
    vi.mocked(api.profileImportDatabase).mockResolvedValue(profile);
  });

  async function chooseDatabase(user: ReturnType<typeof userEvent.setup>) {
    render(<OwnershipMappedImport />, { wrapper: Wrapper });
    await user.click(screen.getByRole("radio", { name: /A database/ }));
  }

  it("offers the saved credentials rather than asking for a password", async () => {
    const user = userEvent.setup();
    await chooseDatabase(user);

    expect(
      await screen.findByRole("option", { name: "cmdb-connection" }),
    ).toBeInTheDocument();
    // No password field anywhere: the connection string never comes through
    // the browser.
    expect(
      document.querySelector('input[type="password"]'),
    ).not.toBeInTheDocument();
  });

  it("reads the query's columns into the mapping step", async () => {
    const user = userEvent.setup();
    await chooseDatabase(user);

    await user.selectOptions(
      await screen.findByRole("combobox", { name: /Connection/ }),
      "cmdb-connection",
    );
    await user.type(
      screen.getByRole("textbox", { name: /Query/ }),
      "SELECT owner_email FROM asset_owner",
    );
    await user.click(screen.getByRole("button", { name: /Read the query/ }));

    await waitFor(() => {
      expect(api.profileImportDatabase).toHaveBeenCalledWith(
        expect.objectContaining({
          driver: "sqlserver",
          credential: "cmdb-connection",
          query: "SELECT owner_email FROM asset_owner",
        }),
      );
    });
    // The query's columns are what the mapping step then offers.
    expect((await screen.findAllByText("Owner Email")).length).toBeGreaterThan(0);
  });

  it("cannot read a query before a connection is chosen", async () => {
    const user = userEvent.setup();
    await chooseDatabase(user);

    await user.type(
      screen.getByRole("textbox", { name: /Query/ }),
      "SELECT 1",
    );
    expect(screen.getByRole("button", { name: /Read the query/ })).toBeDisabled();
  });

  // The connection string is saved as a secret on another page entirely, but
  // this is the page somebody is looking at when they need to know what one
  // looks like — and they are importing owners, not administering databases.
  // Sending them elsewhere to find the format is how they arrive with a
  // connection string that does not work.
  it("shows what the connection string looks like for the chosen database", async () => {
    const user = userEvent.setup();
    await chooseDatabase(user);

    // SQL Server is the default.
    expect(
      await screen.findByText(/sqlserver:\/\/user:pass@host:1433\?database=/),
    ).toBeInTheDocument();

    await user.selectOptions(
      screen.getByRole("combobox", { name: /Database/ }),
      "postgres",
    );

    // The example has to follow the database chosen: the two formats are not
    // interchangeable, and a PostgreSQL example under a SQL Server selection
    // is worse than none.
    expect(
      await screen.findByText(/postgres:\/\/user:pass@host:5432\//),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/sqlserver:\/\/user:pass@host:1433/),
    ).not.toBeInTheDocument();
  });

  // An unreadable credential list must not render as an empty one: they read
  // the same on screen and mean opposite things.
  it("says so when the saved credentials cannot be loaded", async () => {
    vi.mocked(api.fetchCredentials).mockRejectedValue(new Error("nope"));
    const user = userEvent.setup();
    await chooseDatabase(user);

    expect(
      await screen.findByText(/Could not load the saved credentials/),
    ).toBeInTheDocument();
  });
});

// Whoever sets the import up often cannot inspect the customer's database, so
// writing SQL blind is the thing to avoid: the first anyone would learn of a
// wrong query is when the import runs.
describe("browsing the database", () => {
  beforeEach(() => {
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchCredentials).mockResolvedValue({
      data: [{ name: "cmdb-connection", credential_type: "generic" }],
    } as never);
    vi.mocked(api.listImportDatabaseTables).mockResolvedValue({
      data: [
        { schema: "dbo", name: "staff", kind: "table", qualified_name: "[dbo].[staff]" },
        { schema: "dbo", name: "asset_owner", kind: "table", qualified_name: "[dbo].[asset_owner]" },
        { schema: "dbo", name: "v_owners", kind: "view", qualified_name: "[dbo].[v_owners]" },
      ],
    } as never);
  });

  async function browse(user: ReturnType<typeof userEvent.setup>) {
    render(<OwnershipMappedImport />, { wrapper: Wrapper });
    await user.click(screen.getByRole("radio", { name: /A database/ }));
    await user.selectOptions(
      await screen.findByRole("combobox", { name: /Connection/ }),
      "cmdb-connection",
    );
    await user.click(screen.getByRole("button", { name: /Browse tables/ }));
  }

  it("lists the tables and views the connection can see", async () => {
    const user = userEvent.setup();
    await browse(user);

    expect(await screen.findByText("dbo.staff")).toBeInTheDocument();
    expect(screen.getByText("dbo.asset_owner")).toBeInTheDocument();
    // Views are offered too: an operations team has often already built one.
    expect(screen.getByText("dbo.v_owners")).toBeInTheDocument();
  });

  it("writes a query when a table is chosen, quoted for its database", async () => {
    const user = userEvent.setup();
    await browse(user);

    await user.click(await screen.findByText("dbo.staff"));

    expect(screen.getByRole("textbox", { name: /Query/ })).toHaveValue(
      "SELECT * FROM [dbo].[staff]",
    );
  });

  it("cannot browse before a connection is chosen", async () => {
    const user = userEvent.setup();
    render(<OwnershipMappedImport />, { wrapper: Wrapper });
    await user.click(screen.getByRole("radio", { name: /A database/ }));

    expect(screen.getByRole("button", { name: /Browse tables/ })).toBeDisabled();
  });

  it("says so when the tables cannot be listed", async () => {
    vi.mocked(api.listImportDatabaseTables).mockRejectedValue(
      new Error("login failed"),
    );
    const user = userEvent.setup();
    await browse(user);

    expect(await screen.findByText(/login failed/)).toBeInTheDocument();
  });
});

// A database import can run on a schedule, so nobody has to remember to
// re-import. The cron field says back in English what the expression means,
// following the collection settings — an expression somebody cannot read is an
// expression they cannot check, and this one runs unattended.
describe("scheduling a database import", () => {
  beforeEach(() => {
    mockUseAuth.mockReturnValue({
      isOperator: true,
      isAdmin: true,
      user: { role: "admin", username: "test" },
    });
    vi.mocked(api.fetchCredentials).mockResolvedValue({
      data: [{ name: "cmdb-connection", credential_type: "generic" }],
    } as never);
    vi.mocked(api.profileImportDatabase).mockResolvedValue(profile);
    vi.mocked(api.createImportMapping).mockResolvedValue({
      id: 1,
      name: "cmdb-nightly",
    } as never);
  });

  // Reaches the mapping step with a database source ready to save.
  async function readyToSave(user: ReturnType<typeof userEvent.setup>) {
    render(<OwnershipMappedImport />, { wrapper: Wrapper });
    await user.click(screen.getByRole("radio", { name: /A database/ }));
    await user.selectOptions(
      await screen.findByRole("combobox", { name: /Connection/ }),
      "cmdb-connection",
    );
    await user.type(
      screen.getByRole("textbox", { name: /Query/ }),
      "SELECT owner_email FROM asset_owner",
    );
    await user.click(screen.getByRole("button", { name: /Read the query/ }));
    await screen.findByRole("checkbox", { name: /Run this import on a schedule/ });
  }

  it("says in English when the schedule will run", async () => {
    const user = userEvent.setup();
    await readyToSave(user);

    await user.click(screen.getByRole("checkbox", { name: /Run this import on a schedule/ }));
    const cron = screen.getByRole("textbox", { name: /Schedule/ });
    await user.clear(cron);
    await user.type(cron, "0 2 * * *");

    expect(await screen.findByText(/At 02:00/i)).toBeInTheDocument();
  });

  it("sends the connection, the query and the schedule when saving", async () => {
    const user = userEvent.setup();
    await readyToSave(user);

    await user.click(screen.getByRole("checkbox", { name: /Run this import on a schedule/ }));
    const cron = screen.getByRole("textbox", { name: /Schedule/ });
    await user.clear(cron);
    await user.type(cron, "0 2 * * *");
    await user.type(screen.getByPlaceholderText(/Name this import/), "cmdb-nightly");
    await user.click(screen.getByRole("button", { name: /^Save import$/ }));

    await waitFor(() => {
      expect(api.createImportMapping).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "cmdb-nightly",
          source_kind: "database",
          db_credential: "cmdb-connection",
          db_query: "SELECT owner_email FROM asset_owner",
          schedule: "0 2 * * *",
          schedule_enabled: true,
        }),
      );
    });
  });

  // A file import has no stored source, so there is nothing for a schedule to
  // read. Offering the control would be offering something that cannot work.
  it("offers no schedule for a file import", async () => {
    const user = userEvent.setup();
    render(<OwnershipMappedImport />, { wrapper: Wrapper });
    await user.click(screen.getByRole("radio", { name: /A file/ }));

    expect(
      screen.queryByRole("checkbox", { name: /Run this import on a schedule/ }),
    ).not.toBeInTheDocument();
  });
});
