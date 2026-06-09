// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FilterMultiCheckbox } from "./FilterMultiCheckbox";

const defaultOptions = [
  { value: "ubuntu", label: "Ubuntu" },
  { value: "centos", label: "CentOS" },
  { value: "windows", label: "Windows" },
];

describe("FilterMultiCheckbox", () => {
  it("renders the label and trigger button", () => {
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={[]}
        onChange={() => {}}
      />,
    );
    const allPlatform = screen.getAllByText("Platform");
    expect(allPlatform.length).toBeGreaterThanOrEqual(1);
    expect(
      screen.getByRole("button", { name: "Platform" }),
    ).toBeInTheDocument();
  });

  it("shows count of selected items in button text", () => {
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={["ubuntu", "centos", "windows"]}
        onChange={() => {}}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Platform (3)" }),
    ).toBeInTheDocument();
  });

  it("opens dropdown on click and shows checkboxes for all options", async () => {
    const user = userEvent.setup();
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={[]}
        onChange={() => {}}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Platform" }));

    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes).toHaveLength(3);
    expect(screen.getByText("Ubuntu")).toBeInTheDocument();
    expect(screen.getByText("CentOS")).toBeInTheDocument();
    expect(screen.getByText("Windows")).toBeInTheDocument();
  });

  it("calls onChange with value added when toggling an unchecked option", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={["ubuntu"]}
        onChange={onChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Platform (1)" }));
    await user.click(screen.getByRole("checkbox", { name: /CentOS/ }));

    expect(onChange).toHaveBeenCalledWith(["ubuntu", "centos"]);
  });

  it("calls onChange with value removed when unchecking a checked option", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={["ubuntu", "centos"]}
        onChange={onChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Platform (2)" }));
    await user.click(screen.getByRole("checkbox", { name: /Ubuntu/ }));

    expect(onChange).toHaveBeenCalledWith(["centos"]);
  });

  it("shows 'No options' when options array is empty", async () => {
    const user = userEvent.setup();
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={[]}
        selected={[]}
        onChange={() => {}}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Platform" }));

    expect(screen.getByText("No options")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("displays removable chips for selected values", () => {
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={["ubuntu", "centos"]}
        onChange={() => {}}
      />,
    );

    // Chips show the label text
    const chips = screen.getAllByText(/×/);
    expect(chips).toHaveLength(2);
    expect(screen.getByText("Ubuntu")).toBeInTheDocument();
    expect(screen.getByText("CentOS")).toBeInTheDocument();
  });

  it("calls onChange without the value when chip remove button is clicked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={["ubuntu", "centos"]}
        onChange={onChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove Ubuntu" }));

    expect(onChange).toHaveBeenCalledWith(["centos"]);
  });

  it("chip remove button has correct aria-label", () => {
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={["ubuntu", "windows"]}
        onChange={() => {}}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Remove Ubuntu" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove Windows" }),
    ).toBeInTheDocument();
  });

  it("falls back to raw value when selected value is not in options", () => {
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={["unknown"]}
        onChange={() => {}}
      />,
    );

    expect(screen.getByText("unknown")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove unknown" }),
    ).toBeInTheDocument();
  });

  it("shows count next to option label when count is provided", async () => {
    const user = userEvent.setup();
    const optionsWithCount = [
      { value: "ubuntu", label: "Ubuntu", count: 42 },
      { value: "centos", label: "CentOS", count: 0 },
    ];
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={optionsWithCount}
        selected={[]}
        onChange={() => {}}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Platform" }));

    expect(screen.getByText("(42)")).toBeInTheDocument();
    expect(screen.getByText("(0)")).toBeInTheDocument();
  });

  it("closes dropdown on outside click", async () => {
    const user = userEvent.setup();
    render(
      <div>
        <span>Outside</span>
        <FilterMultiCheckbox
          label="Platform"
          options={defaultOptions}
          selected={[]}
          onChange={() => {}}
        />
      </div>,
    );

    await user.click(screen.getByRole("button", { name: "Platform" }));
    expect(screen.getAllByRole("checkbox")).toHaveLength(3);

    await user.click(screen.getByText("Outside"));
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("closes dropdown on Escape key", async () => {
    const user = userEvent.setup();
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={[]}
        onChange={() => {}}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Platform" }));
    expect(screen.getAllByRole("checkbox")).toHaveLength(3);

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("toggles dropdown closed on second button click", async () => {
    const user = userEvent.setup();
    render(
      <FilterMultiCheckbox
        label="Platform"
        options={defaultOptions}
        selected={[]}
        onChange={() => {}}
      />,
    );

    const trigger = screen.getByRole("button", { name: "Platform" });
    await user.click(trigger);
    expect(screen.getAllByRole("checkbox")).toHaveLength(3);

    await user.click(trigger);
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });
});
