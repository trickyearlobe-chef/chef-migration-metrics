// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { CREDENTIAL_TYPES, typeLabel, BADGE_LABELS, BADGE_STYLES } from "./constants";
import { ValueField } from "./ValueField";
import { render, screen } from "@testing-library/react";

// A credential type the server validates but the screen never offers is a type
// nobody can use. That is what shipped in v2.21.1: database connections were
// checked on the way in, and the picker only knew about Chef keys and generic
// values, so the check could never fire. Found by the owner opening the dialog.
//
// The pairing is the thing to hold: every type the server accepts must be
// choosable, and every choosable type must be labelled wherever credentials are
// listed, or it shows as a raw identifier.

describe("credential types", () => {
  // Kept in step with internal/secrets/validation.go — ValidCredentialTypes
  // there has a matching completeness test, so the two lists cannot drift apart
  // without one of them failing.
  const SERVER_TYPES = ["chef_client_key", "generic", "database_url"];

  it("offers every type the server accepts", () => {
    for (const t of SERVER_TYPES) {
      expect(
        CREDENTIAL_TYPES.some((ct) => ct.value === t),
        `the server accepts ${t} but the picker does not offer it, so nobody can create one`,
      ).toBe(true);
    }
  });

  it("labels every type it offers, in both the picker and the list", () => {
    for (const ct of CREDENTIAL_TYPES) {
      expect(typeLabel(ct.value)).not.toBe(ct.value);
      expect(BADGE_LABELS[ct.value], `${ct.value} has no badge label`).toBeTruthy();
      expect(BADGE_STYLES[ct.value], `${ct.value} has no badge style`).toBeTruthy();
    }
  });

  // The placeholder is the only place the required shape is stated, and a
  // connection without a database in it is refused.
  it("shows the shape a database connection has to have", () => {
    render(
      <ValueField credentialType="database_url" value="" onChange={() => {}} />,
    );
    const field = screen.getByRole("textbox") as HTMLTextAreaElement;
    expect(field.placeholder).toMatch(/postgres:\/\//);
    expect(field.placeholder).toMatch(/DATABASE/);
  });
});
