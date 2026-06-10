import { describe, it, expect } from "vitest";

import { isSetupAllowedPath } from "./App";

// During setup mode (no organisations configured) the SetupModeGuard
// redirects admins to the wizard. It must NOT redirect routes the wizard
// depends on — otherwise the user cannot create the Chef API key credential
// the organisation step requires, and setup deadlocks.
describe("isSetupAllowedPath", () => {
  it("allows the setup wizard itself", () => {
    expect(isSetupAllowedPath("/admin/setup")).toBe(true);
  });

  it("allows the credentials page so a credential can be created during setup", () => {
    expect(isSetupAllowedPath("/admin/credentials")).toBe(true);
  });

  it("allows nested paths under an allowed prefix", () => {
    expect(isSetupAllowedPath("/admin/credentials/new")).toBe(true);
  });

  it("redirects other admin pages back to the wizard", () => {
    expect(isSetupAllowedPath("/admin/settings")).toBe(false);
    expect(isSetupAllowedPath("/admin/organisations")).toBe(false);
  });

  it("redirects ordinary app pages back to the wizard", () => {
    expect(isSetupAllowedPath("/")).toBe(false);
    expect(isSetupAllowedPath("/nodes")).toBe(false);
  });
});
