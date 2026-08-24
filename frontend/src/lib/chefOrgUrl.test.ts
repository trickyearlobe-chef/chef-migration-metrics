// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { chefOrgURLError } from "./chefOrgUrl";

describe("chefOrgURLError", () => {
  it("accepts a full org URL", () => {
    expect(chefOrgURLError("https://chef.example.com/organizations/myorg")).toBeNull();
  });

  it("accepts http and a hostname with a port", () => {
    expect(chefOrgURLError("http://chef.example.com:8443/organizations/example-corp")).toBeNull();
  });

  it("rejects an empty value", () => {
    expect(chefOrgURLError("")).toMatch(/required/i);
    expect(chefOrgURLError("   ")).toMatch(/required/i);
  });

  it("rejects a non-URL", () => {
    expect(chefOrgURLError("not a url")).toMatch(/valid url/i);
  });

  it("rejects a base URL with no /organizations/<org> segment", () => {
    expect(chefOrgURLError("https://chef.example.com")).toMatch(/full organisation url/i);
    expect(chefOrgURLError("https://chef.example.com/organizations/")).toMatch(/full organisation url/i);
  });

  it("rejects a non-http(s) scheme", () => {
    expect(chefOrgURLError("ftp://chef.example.com/organizations/myorg")).toMatch(/https/i);
  });
});
