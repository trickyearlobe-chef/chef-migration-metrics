// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AdminTestKitchenPage } from "./AdminTestKitchenPage";

vi.mock("../api", () => ({
  fetchTestKitchenConfig: vi.fn(),
  saveTestKitchenConfig: vi.fn(),
  deleteTestKitchenConfig: vi.fn(),
  fetchCredentials: vi.fn(),
  fetchPlatformMappingStatus: vi.fn(),
  ApiError: class extends Error {
    code: number;
    constructor(code: number, message: string) {
      super(message);
      this.code = code;
    }
  },
}));

import {
  fetchTestKitchenConfig,
  saveTestKitchenConfig,
  fetchCredentials,
  fetchPlatformMappingStatus,
} from "../api";

const defaultConfig = {
  enabled: true,
  driver: "proxmox",
  timeout_minutes: 30,
  driver_settings: {},
  driver_secrets: {},
  image_field_name: "clone",
  chef_license_key_credential: "",
  images: [
    { name: "rhel9-tmpl", id: "tmpl-101", transport: null },
    { name: "ubuntu-tmpl", id: "tmpl-102", transport: null },
  ],
  platform_map: [],
  start_rate_window_minutes: 0,
  start_rate_max_per_window: 0,
};

const defaultMappingStatus = {
  discovered_platforms: [
    {
      platform_name: "rhel-9",
      normalised_name: "rhel-9",
      os_family: "rhel",
      cookbook_count: 45,
      node_count: 10,
      source: "both" as const,
      transport_type: "ssh",
      mapping_status: "unmapped" as const,
      matched_entry_index: -1,
      matched_image: "",
    },
    {
      platform_name: "ubuntu-22.04",
      normalised_name: "ubuntu-22.04",
      os_family: "debian",
      cookbook_count: 30,
      node_count: 0,
      source: "kitchen" as const,
      transport_type: "ssh",
      mapping_status: "mapped" as const,
      matched_entry_index: 0,
      matched_image: "ubuntu-tmpl",
    },
    {
      platform_name: "windows 2022",
      normalised_name: "windows 2022",
      os_family: "windows",
      cookbook_count: 0,
      node_count: 5,
      source: "nodes" as const,
      transport_type: "",
      mapping_status: "unmapped" as const,
      matched_entry_index: -1,
      matched_image: "",
    },
  ],
  templates: [],
  unmapped_count: 2,
  skipped_count: 0,
  mapped_count: 1,
};

describe("AdminTestKitchenPage — Platform Map Section", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(fetchTestKitchenConfig).mockResolvedValue({
      ...defaultConfig,
    });
    vi.mocked(fetchCredentials).mockResolvedValue({ data: [], total: 0 });
    vi.mocked(fetchPlatformMappingStatus).mockResolvedValue({
      ...defaultMappingStatus,
    });
  });

  it("renders discovered platforms in the mapping table", async () => {
    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(screen.getByText("rhel-9")).toBeInTheDocument();
    });

    expect(screen.getByText("ubuntu-22.04")).toBeInTheDocument();
    expect(screen.getByText("windows 2022")).toBeInTheDocument();
  });

  it("shows source badges for each platform", async () => {
    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(screen.getByText("rhel-9")).toBeInTheDocument();
    });

    expect(screen.getByText("both")).toBeInTheDocument();
    expect(screen.getByText("kitchen")).toBeInTheDocument();
    expect(screen.getByText("nodes")).toBeInTheDocument();
  });

  it("shows OS family badges", async () => {
    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(screen.getByText("rhel-9")).toBeInTheDocument();
    });

    expect(screen.getByText("rhel")).toBeInTheDocument();
    expect(screen.getByText("debian")).toBeInTheDocument();
    expect(screen.getByText("windows")).toBeInTheDocument();
  });

  it("shows cookbook and node counts", async () => {
    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(screen.getByText("rhel-9")).toBeInTheDocument();
    });

    expect(screen.getByText(/45 cookbooks/)).toBeInTheDocument();
    expect(screen.getByText(/10 nodes/)).toBeInTheDocument();
    expect(screen.getByText(/5 nodes/)).toBeInTheDocument();
  });

  it("pre-selects mapped image in dropdown", async () => {
    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(screen.getByText("ubuntu-22.04")).toBeInTheDocument();
    });

    // Find the row for ubuntu-22.04 and check its select value.
    const selects = screen.getAllByRole("combobox");
    const ubuntuSelect = selects.find(
      (s) => (s as HTMLSelectElement).value === "ubuntu-tmpl",
    );
    expect(ubuntuSelect).toBeDefined();
  });

  it("defaults unmapped platforms to skip", async () => {
    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(screen.getByText("rhel-9")).toBeInTheDocument();
    });

    // Unmapped platforms should have empty value (skip) in their selects.
    const selects = screen.getAllByRole("combobox");
    const skipSelects = selects.filter(
      (s) => (s as HTMLSelectElement).value === "",
    );
    // rhel-9 and windows 2022 are unmapped
    expect(skipSelects.length).toBeGreaterThanOrEqual(2);
  });

  it("updates config when image is selected", async () => {
    const user = userEvent.setup();
    vi.mocked(saveTestKitchenConfig).mockResolvedValue({
      value: defaultConfig,
      restartRequired: false,
    });

    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(screen.getByText("rhel-9")).toBeInTheDocument();
    });

    // Find the rhel-9 row and change its image dropdown.
    const rows = screen.getAllByRole("row");
    const rhelRow = rows.find((r) => within(r).queryByText("rhel-9"));
    expect(rhelRow).toBeDefined();

    const select = within(rhelRow!).getByRole("combobox");
    await user.selectOptions(select, "rhel9-tmpl");

    expect((select as HTMLSelectElement).value).toBe("rhel9-tmpl");
  });

  it("shows mapping status badges", async () => {
    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(screen.getByText("rhel-9")).toBeInTheDocument();
    });

    expect(screen.getByText("1 mapped")).toBeInTheDocument();
    expect(screen.getByText("2 unmapped")).toBeInTheDocument();
  });

  it("renders rate-limit fields from config and saves edited values", async () => {
    const user = userEvent.setup();
    vi.mocked(fetchTestKitchenConfig).mockResolvedValue({
      ...defaultConfig,
      start_rate_window_minutes: 60,
      start_rate_max_per_window: 25,
    });
    vi.mocked(saveTestKitchenConfig).mockResolvedValue({
      value: defaultConfig,
      restartRequired: false,
    });

    render(<AdminTestKitchenPage />);

    const windowInput = await screen.findByLabelText(/Window \(minutes\)/);
    const maxInput = screen.getByLabelText(/Max starts per window/);
    expect((windowInput as HTMLInputElement).value).toBe("60");
    expect((maxInput as HTMLInputElement).value).toBe("25");

    await user.clear(windowInput);
    await user.type(windowInput, "90");

    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(saveTestKitchenConfig).toHaveBeenCalled();
    });
    const saved = vi.mocked(saveTestKitchenConfig).mock.calls[0][0];
    expect(saved.start_rate_window_minutes).toBe(90);
    expect(saved.start_rate_max_per_window).toBe(25);
  });

  it("opts an image in to IP-release on teardown and saves the flag", async () => {
    const user = userEvent.setup();
    vi.mocked(saveTestKitchenConfig).mockResolvedValue({
      value: defaultConfig,
      restartRequired: false,
    });

    render(<AdminTestKitchenPage />);

    const checkboxes = await screen.findAllByLabelText(
      /Release the DHCP lease on teardown/,
    );
    // Default off for the first image.
    expect((checkboxes[0] as HTMLInputElement).checked).toBe(false);

    await user.click(checkboxes[0]);
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(saveTestKitchenConfig).toHaveBeenCalled();
    });
    const saved = vi.mocked(saveTestKitchenConfig).mock.calls[0][0];
    expect(saved.images[0].release_ip_on_destroy).toBe(true);
    expect(saved.images[1].release_ip_on_destroy).not.toBe(true);
  });

  it("edits per-OS-family setup-script patterns and saves them", async () => {
    const user = userEvent.setup();
    vi.mocked(saveTestKitchenConfig).mockResolvedValue({
      value: defaultConfig,
      restartRequired: false,
    });

    render(<AdminTestKitchenPage />);

    const linux = await screen.findByLabelText(/Linux patterns/);
    await user.type(linux, "test/setup/a.sh{Enter}test/setup/b.sh");

    const windows = screen.getByLabelText(/Windows patterns/);
    await user.type(windows, "test/setup/users.ps1");

    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(saveTestKitchenConfig).toHaveBeenCalled();
    });
    const saved = vi.mocked(saveTestKitchenConfig).mock.calls[0][0];
    expect(saved.setup_scripts?.linux).toEqual([
      "test/setup/a.sh",
      "test/setup/b.sh",
    ]);
    expect(saved.setup_scripts?.windows).toEqual(["test/setup/users.ps1"]);
  });

  it("renders existing setup-script patterns from config", async () => {
    vi.mocked(fetchTestKitchenConfig).mockResolvedValue({
      ...defaultConfig,
      setup_scripts: { linux: ["scripts/*.sh"], windows: [] },
    });

    render(<AdminTestKitchenPage />);

    const linux = await screen.findByLabelText(/Linux patterns/);
    expect((linux as HTMLTextAreaElement).value).toBe("scripts/*.sh");
  });

  it("shows empty state when no platforms discovered", async () => {
    vi.mocked(fetchPlatformMappingStatus).mockResolvedValue({
      discovered_platforms: [],
      templates: [],
      unmapped_count: 0,
      skipped_count: 0,
      mapped_count: 0,
    });

    render(<AdminTestKitchenPage />);

    await waitFor(() => {
      expect(
        screen.getByText(/No platforms discovered yet/),
      ).toBeInTheDocument();
    });
  });
});
