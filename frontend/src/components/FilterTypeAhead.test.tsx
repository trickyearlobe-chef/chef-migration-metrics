// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { FilterTypeAhead } from "./FilterTypeAhead";
import { apiFetch } from "../api/client";

vi.mock("../api/client", () => ({
  apiFetch: vi.fn(),
}));

const mockedApiFetch = vi.mocked(apiFetch);

function renderComponent(
  props: Partial<React.ComponentProps<typeof FilterTypeAhead>> = {},
) {
  const onChange = props.onChange ?? vi.fn();
  const defaultProps = {
    label: "Environment",
    endpoint: "/api/v1/environments",
    selected: [] as string[],
    onChange,
    ...props,
  };
  const result = render(<FilterTypeAhead {...defaultProps} />);
  return { ...result, onChange };
}

function changeInput(input: HTMLElement, value: string) {
  fireEvent.change(input, { target: { value } });
}

/** Flush the microtask queue so resolved promises propagate through .then/.catch/.finally chains. */
async function flushMicrotasks() {
  await act(async () => {
    for (let i = 0; i < 10; i++) {
      await Promise.resolve();
    }
  });
}

async function typeAndDebounce(input: HTMLElement, text: string, ms = 300) {
  changeInput(input, text);
  act(() => {
    vi.advanceTimersByTime(ms);
  });
  await flushMicrotasks();
}

describe("FilterTypeAhead", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockedApiFetch.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders label and input with correct placeholder", () => {
    renderComponent({ label: "Platform" });
    expect(screen.getByText("Platform")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Search platform…")).toBeInTheDocument();
  });

  it("does not search when input is below minChars threshold", async () => {
    renderComponent({ minChars: 3 });
    const input = screen.getByRole("textbox");

    changeInput(input, "ab");
    act(() => {
      vi.advanceTimersByTime(500);
    });
    await flushMicrotasks();

    expect(mockedApiFetch).not.toHaveBeenCalled();
  });

  it("calls apiFetch with correct URL after typing enough characters and debounce", async () => {
    mockedApiFetch.mockResolvedValue({ data: ["production"] });
    renderComponent();
    const input = screen.getByRole("textbox");

    await typeAndDebounce(input, "pro");

    expect(mockedApiFetch).toHaveBeenCalledWith("/api/v1/environments?q=pro");
  });

  it("shows dropdown results after successful fetch", async () => {
    mockedApiFetch.mockResolvedValue({ data: ["production", "preview"] });
    renderComponent();
    const input = screen.getByRole("textbox");

    await typeAndDebounce(input, "pro");

    expect(screen.getByText("production")).toBeInTheDocument();
    expect(screen.getByText("preview")).toBeInTheDocument();
  });

  it("clicking a result calls onChange with that value added to selected", async () => {
    mockedApiFetch.mockResolvedValue({ data: ["production", "preview"] });
    const { onChange } = renderComponent();
    const input = screen.getByRole("textbox");

    await typeAndDebounce(input, "pro");

    expect(screen.getByText("production")).toBeInTheDocument();
    fireEvent.click(screen.getByText("production"));

    expect(onChange).toHaveBeenCalledWith(["production"]);
  });

  it("after selecting, input is cleared and dropdown closes", async () => {
    mockedApiFetch.mockResolvedValue({ data: ["production"] });
    renderComponent();
    const input = screen.getByRole("textbox");

    await typeAndDebounce(input, "pro");

    expect(screen.getByText("production")).toBeInTheDocument();
    fireEvent.click(screen.getByText("production"));

    expect(input).toHaveValue("");
    // The dropdown button for "production" should no longer exist
    const dropdownButtons = screen
      .queryAllByRole("button")
      .filter((b) => b.textContent === "production");
    expect(dropdownButtons).toHaveLength(0);
  });

  it("displays removable chips for selected values", () => {
    renderComponent({ selected: ["production", "staging"] });
    expect(screen.getByText("production")).toBeInTheDocument();
    expect(screen.getByText("staging")).toBeInTheDocument();
  });

  it("clicking chip remove button calls onChange without that value", () => {
    const { onChange } = renderComponent({
      selected: ["production", "staging"],
    });

    fireEvent.click(screen.getByRole("button", { name: "Remove production" }));

    expect(onChange).toHaveBeenCalledWith(["staging"]);
  });

  it("chip remove button has correct aria-label", () => {
    renderComponent({ selected: ["production"] });
    expect(
      screen.getByRole("button", { name: "Remove production" }),
    ).toBeInTheDocument();
  });

  it("already-selected values are filtered from results", async () => {
    mockedApiFetch.mockResolvedValue({ data: ["production", "preview"] });
    renderComponent({ selected: ["production"] });
    const input = screen.getByRole("textbox");

    await typeAndDebounce(input, "pro");

    // "preview" should appear in the dropdown
    expect(screen.getByText("preview")).toBeInTheDocument();
    // "production" appears as a chip but should NOT appear as a dropdown option button
    const dropdownButtons = screen
      .getAllByRole("button")
      .filter(
        (b) =>
          b.textContent === "production" &&
          !b.getAttribute("aria-label")?.startsWith("Remove"),
      );
    expect(dropdownButtons).toHaveLength(0);
  });

  it("closes dropdown on outside click", async () => {
    mockedApiFetch.mockResolvedValue({ data: ["production"] });
    renderComponent();
    const input = screen.getByRole("textbox");

    await typeAndDebounce(input, "pro");

    // Confirm dropdown is open
    expect(screen.getByText("production")).toBeInTheDocument();

    // Click outside the component container
    act(() => {
      document.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    });

    // The dropdown button for "production" should be gone
    const dropdownButtons = screen
      .queryAllByRole("button")
      .filter((b) => b.textContent === "production");
    expect(dropdownButtons).toHaveLength(0);
  });

  it("closes dropdown on Escape key", async () => {
    mockedApiFetch.mockResolvedValue({ data: ["production"] });
    renderComponent();
    const input = screen.getByRole("textbox");

    await typeAndDebounce(input, "pro");

    // Confirm dropdown is open
    expect(screen.getByText("production")).toBeInTheDocument();

    fireEvent.keyDown(input, { key: "Escape" });

    // The dropdown button for "production" should be gone
    const dropdownButtons = screen
      .queryAllByRole("button")
      .filter((b) => b.textContent === "production");
    expect(dropdownButtons).toHaveLength(0);
  });
});
