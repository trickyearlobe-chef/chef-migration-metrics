// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { HolderRef } from "./HolderRef";

// The ticket field addresses a system CMM does not read, so it is free text.
// People paste ServiceNow and Jira links into it, and a link you have to
// select and copy is a link that does not get followed.

describe("HolderRef", () => {
  it("makes a pasted https link clickable", () => {
    render(<HolderRef value="https://example-corp.service-now.com/incident/INC0012345" />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute(
      "href",
      "https://example-corp.service-now.com/incident/INC0012345",
    );
    expect(link).toHaveTextContent("INC0012345");
  });

  it("opens in a new tab without handing the page over", () => {
    render(<HolderRef value="https://jira.example-corp.com/browse/OPS-42" />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("target", "_blank");
    // Without noopener the opened page can navigate this one.
    expect(link.getAttribute("rel")).toContain("noopener");
    expect(link.getAttribute("rel")).toContain("noreferrer");
  });

  it("leaves a plain ticket number as text", () => {
    render(<HolderRef value="INC0012345" />);

    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByText("INC0012345")).toBeInTheDocument();
  });

  // The register is free text that anyone with an operator role can write, and
  // everyone else reads. A javascript: URL rendered as a link is a script one
  // colleague runs in another's session, so only http and https are linked.
  it("refuses to link a javascript: URL", () => {
    render(<HolderRef value="javascript:alert(document.cookie)" />);

    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(
      screen.getByText("javascript:alert(document.cookie)"),
    ).toBeInTheDocument();
  });

  it("refuses other schemes too", () => {
    for (const value of ["data:text/html,<script>x</script>", "file:///etc/passwd"]) {
      const { unmount } = render(<HolderRef value={value} />);
      expect(screen.queryByRole("link")).not.toBeInTheDocument();
      unmount();
    }
  });

  it("shows nothing at all when there is no reference", () => {
    const { container } = render(<HolderRef value="" />);
    expect(container).toBeEmptyDOMElement();
  });

  // A whole URL in a narrow column pushes the table about; the last meaningful
  // part is what identifies the ticket to a person.
  it("shows the tail of a long link, with the full address on hover", () => {
    const url = "https://example-corp.service-now.com/nav_to.do?uri=incident.do?sys_id=INC0012345";
    render(<HolderRef value={url} />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("title", url);
  });
});
