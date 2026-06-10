// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import * as api from "../api";
import { TLSDegradedBanner } from "./TLSDegradedBanner";

vi.mock("../api");

describe("TLSDegradedBanner", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("renders nothing when TLS is healthy", async () => {
    vi.mocked(api.fetchTLSStatus).mockResolvedValue({ degraded: false });
    const { container } = render(<TLSDegradedBanner />);
    await vi.runOnlyPendingTimersAsync();
    expect(container).toBeEmptyDOMElement();
  });

  it("warns about an untrusted self-signed cert with the reason when degraded", async () => {
    vi.mocked(api.fetchTLSStatus).mockResolvedValue({
      degraded: true,
      kind: "self-signed",
      reason: "TLS listener setup failed: open /etc/ssl/server.crt: no such file",
    });
    render(<TLSDegradedBanner />);
    await vi.runOnlyPendingTimersAsync();

    const alert = screen.getByRole("alert");
    expect(alert).toBeInTheDocument();
    expect(alert).toHaveTextContent(/untrusted self-signed certificate/i);
    expect(alert).toHaveTextContent(/no such file/);
  });

  it("warns about INSECURE plain HTTP for the last-resort kind", async () => {
    vi.mocked(api.fetchTLSStatus).mockResolvedValue({
      degraded: true,
      kind: "plain",
      reason: "TLS listener setup failed: self-signed generation failed",
    });
    render(<TLSDegradedBanner />);
    await vi.runOnlyPendingTimersAsync();

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent(/INSECURE/i);
    expect(alert).toHaveTextContent(/plain HTTP/i);
  });

  it("defaults to the self-signed message when kind is absent", async () => {
    vi.mocked(api.fetchTLSStatus).mockResolvedValue({
      degraded: true,
      reason: "TLS listener setup failed: bad cert",
    });
    render(<TLSDegradedBanner />);
    await vi.runOnlyPendingTimersAsync();

    expect(screen.getByRole("alert")).toHaveTextContent(/untrusted self-signed certificate/i);
  });

  it("renders nothing when the status poll fails", async () => {
    vi.mocked(api.fetchTLSStatus).mockRejectedValue(new Error("network"));
    const { container } = render(<TLSDegradedBanner />);
    await vi.runOnlyPendingTimersAsync();
    expect(container).toBeEmptyDOMElement();
  });
});
