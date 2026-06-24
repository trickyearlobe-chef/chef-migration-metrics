// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { CookstyleFailureRulesGrid } from "./CookstyleFailureRulesGrid";
import {
  COOKSTYLE_PRESETS,
  COOKSTYLE_NAMESPACES,
  COOKSTYLE_SEVERITIES,
} from "../types/config";

describe("CookstyleFailureRulesGrid", () => {
  const defaultRules = COOKSTYLE_PRESETS["default"];

  it("renders the preset dropdown with the current preset selected", () => {
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={vi.fn()}
        disabled={false}
      />,
    );
    const select = screen.getByRole("combobox", { name: /preset/i });
    expect(select).toHaveValue("default");
  });

  it("renders a 5×5 checkbox grid", () => {
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={vi.fn()}
        disabled={false}
      />,
    );
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes).toHaveLength(
      COOKSTYLE_NAMESPACES.length * COOKSTYLE_SEVERITIES.length,
    );
  });

  it("selecting strict preset populates the grid accordingly", () => {
    const onChange = vi.fn();
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={onChange}
        disabled={false}
      />,
    );
    fireEvent.change(screen.getByRole("combobox", { name: /preset/i }), {
      target: { value: "strict" },
    });
    expect(onChange).toHaveBeenCalledWith("strict", COOKSTYLE_PRESETS["strict"]);
  });

  it("selecting relaxed preset populates the grid accordingly", () => {
    const onChange = vi.fn();
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={onChange}
        disabled={false}
      />,
    );
    fireEvent.change(screen.getByRole("combobox", { name: /preset/i }), {
      target: { value: "relaxed" },
    });
    expect(onChange).toHaveBeenCalledWith(
      "relaxed",
      COOKSTYLE_PRESETS["relaxed"],
    );
  });

  it("manual checkbox change switches preset to custom", () => {
    const onChange = vi.fn();
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={onChange}
        disabled={false}
      />,
    );
    // The "default" preset only has error+fatal for "*". Tick "warning" for "Chef/Deprecations/".
    const checkbox = screen.getByRole("checkbox", {
      name: /Chef\/Deprecations.*warning/i,
    });
    fireEvent.click(checkbox);
    expect(onChange).toHaveBeenCalledWith(
      "custom",
      expect.objectContaining({
        "Chef/Deprecations/": ["warning"],
      }),
    );
  });

  it("unchecking a checked severity removes it from rules", () => {
    const strictRules = COOKSTYLE_PRESETS["strict"];
    const onChange = vi.fn();
    render(
      <CookstyleFailureRulesGrid
        preset="strict"
        rules={strictRules}
        onChange={onChange}
        disabled={false}
      />,
    );
    // strict has warning+error+fatal for Chef/Deprecations/. Uncheck "warning".
    const checkbox = screen.getByRole("checkbox", {
      name: /Chef\/Deprecations.*warning/i,
    });
    expect(checkbox).toBeChecked();
    fireEvent.click(checkbox);
    expect(onChange).toHaveBeenCalledWith(
      "custom",
      expect.objectContaining({
        "Chef/Deprecations/": expect.not.arrayContaining(["warning"]),
      }),
    );
  });

  it("disables all inputs when disabled prop is true", () => {
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={vi.fn()}
        disabled={true}
      />,
    );
    const select = screen.getByRole("combobox", { name: /preset/i });
    expect(select).toBeDisabled();
    const checkboxes = screen.getAllByRole("checkbox");
    checkboxes.forEach((cb) => expect(cb).toBeDisabled());
  });

  it("renders namespace labels in the grid", () => {
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={vi.fn()}
        disabled={false}
      />,
    );
    expect(screen.getByText("Chef/Deprecations")).toBeInTheDocument();
    expect(screen.getByText("Chef/Correctness")).toBeInTheDocument();
    expect(screen.getByText("Chef/Style")).toBeInTheDocument();
    expect(screen.getByText("Chef/Modernize")).toBeInTheDocument();
    expect(screen.getByText("Other (catch-all)")).toBeInTheDocument();
  });

  it("renders severity column headers", () => {
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={vi.fn()}
        disabled={false}
      />,
    );
    for (const sev of COOKSTYLE_SEVERITIES) {
      expect(screen.getByText(sev)).toBeInTheDocument();
    }
  });

  it("default preset checks error+fatal for catch-all only", () => {
    render(
      <CookstyleFailureRulesGrid
        preset="default"
        rules={defaultRules}
        onChange={vi.fn()}
        disabled={false}
      />,
    );
    // catch-all error and fatal should be checked
    const catchAllError = screen.getByRole("checkbox", {
      name: /Other.*error/i,
    });
    const catchAllFatal = screen.getByRole("checkbox", {
      name: /Other.*fatal/i,
    });
    expect(catchAllError).toBeChecked();
    expect(catchAllFatal).toBeChecked();
    // catch-all convention should not be checked
    const catchAllConvention = screen.getByRole("checkbox", {
      name: /Other.*convention/i,
    });
    expect(catchAllConvention).not.toBeChecked();
  });
});
