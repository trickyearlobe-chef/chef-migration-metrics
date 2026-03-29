// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FilterInput, FilterSelect, FilterCombobox } from "./FilterInputs";

describe("FilterInput", () => {
  it("renders a labelled text input", () => {
    render(<FilterInput label="Node Name" value="" onChange={() => {}} />);
    expect(screen.getByText("Node Name")).toBeInTheDocument();
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("displays the current value", () => {
    render(<FilterInput label="Name" value="web-01" onChange={() => {}} />);
    expect(screen.getByRole("textbox")).toHaveValue("web-01");
  });

  it("shows placeholder text", () => {
    render(
      <FilterInput
        label="Name"
        value=""
        onChange={() => {}}
        placeholder="Filter by name"
      />,
    );
    expect(screen.getByPlaceholderText("Filter by name")).toBeInTheDocument();
  });

  it("calls onChange when the user types", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<FilterInput label="Name" value="" onChange={onChange} />);

    await user.type(screen.getByRole("textbox"), "a");
    expect(onChange).toHaveBeenCalledWith("a");
  });

  it("calls onChange on each keystroke", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<FilterInput label="Host" value="" onChange={onChange} />);

    await user.type(screen.getByRole("textbox"), "ab");
    expect(onChange).toHaveBeenCalledTimes(2);
    // Controlled component with static value="" — each keystroke reports
    // only its own character because React resets the input to "".
    expect(onChange).toHaveBeenNthCalledWith(1, "a");
    expect(onChange).toHaveBeenNthCalledWith(2, "b");
  });
});

describe("FilterSelect", () => {
  const options = [
    { value: "", label: "All" },
    { value: "active", label: "Active" },
    { value: "inactive", label: "Inactive" },
  ];

  it("renders a labelled select with all options", () => {
    render(
      <FilterSelect
        label="Status"
        value=""
        onChange={() => {}}
        options={options}
      />,
    );
    expect(screen.getByText("Status")).toBeInTheDocument();
    const select = screen.getByRole("combobox");
    expect(select).toBeInTheDocument();

    const opts = screen.getAllByRole("option");
    expect(opts).toHaveLength(3);
    expect(opts[0]).toHaveTextContent("All");
    expect(opts[1]).toHaveTextContent("Active");
    expect(opts[2]).toHaveTextContent("Inactive");
  });

  it("reflects the current value", () => {
    render(
      <FilterSelect
        label="Status"
        value="active"
        onChange={() => {}}
        options={options}
      />,
    );
    expect(screen.getByRole("combobox")).toHaveValue("active");
  });

  it("calls onChange when the user selects an option", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <FilterSelect
        label="Status"
        value=""
        onChange={onChange}
        options={options}
      />,
    );

    await user.selectOptions(screen.getByRole("combobox"), "active");
    expect(onChange).toHaveBeenCalledWith("active");
  });

  it("applies wider width class when wide prop is true", () => {
    const { container } = render(
      <FilterSelect
        label="Type"
        value=""
        onChange={() => {}}
        options={options}
        wide
      />,
    );
    const select = container.querySelector("select");
    expect(select?.className).toContain("w-48");
    expect(select?.className).not.toContain("w-32");
  });

  it("applies default width class when wide prop is omitted", () => {
    const { container } = render(
      <FilterSelect
        label="Type"
        value=""
        onChange={() => {}}
        options={options}
      />,
    );
    const select = container.querySelector("select");
    expect(select?.className).toContain("w-32");
    expect(select?.className).not.toContain("w-48");
  });
});

describe("FilterCombobox", () => {
  it("renders a text input when options are empty (fallback)", () => {
    render(
      <FilterCombobox
        label="Environment"
        value=""
        onChange={() => {}}
        options={[]}
        placeholder="All environments"
      />,
    );
    expect(screen.getByRole("textbox")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("All environments")).toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  });

  it("renders a select when options are provided", () => {
    render(
      <FilterCombobox
        label="Environment"
        value=""
        onChange={() => {}}
        options={["production", "staging", "development"]}
        placeholder="All environments"
      />,
    );
    expect(screen.getByRole("combobox")).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });

  it("includes a placeholder option as the first entry", () => {
    render(
      <FilterCombobox
        label="Platform"
        value=""
        onChange={() => {}}
        options={["ubuntu", "centos"]}
        placeholder="All platforms"
      />,
    );
    const opts = screen.getAllByRole("option");
    expect(opts[0]).toHaveTextContent("All platforms");
    expect(opts[0]).toHaveValue("");
  });

  it("generates a default placeholder from the label when none given", () => {
    render(
      <FilterCombobox
        label="Role"
        value=""
        onChange={() => {}}
        options={["webserver", "database"]}
      />,
    );
    const opts = screen.getAllByRole("option");
    expect(opts[0]).toHaveTextContent("All roles");
  });

  it("lists all backend options after the placeholder", () => {
    render(
      <FilterCombobox
        label="Env"
        value=""
        onChange={() => {}}
        options={["prod", "staging"]}
      />,
    );
    const opts = screen.getAllByRole("option");
    expect(opts).toHaveLength(3); // placeholder + 2 options
    expect(opts[1]).toHaveTextContent("prod");
    expect(opts[2]).toHaveTextContent("staging");
  });

  it("reflects the current value", () => {
    render(
      <FilterCombobox
        label="Env"
        value="staging"
        onChange={() => {}}
        options={["prod", "staging"]}
      />,
    );
    expect(screen.getByRole("combobox")).toHaveValue("staging");
  });

  it("calls onChange when the user selects an option", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <FilterCombobox
        label="Env"
        value=""
        onChange={onChange}
        options={["prod", "staging"]}
      />,
    );

    await user.selectOptions(screen.getByRole("combobox"), "prod");
    expect(onChange).toHaveBeenCalledWith("prod");
  });

  it("calls onChange in fallback text input mode", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <FilterCombobox label="Env" value="" onChange={onChange} options={[]} />,
    );

    await user.type(screen.getByRole("textbox"), "x");
    expect(onChange).toHaveBeenCalledWith("x");
  });
});
