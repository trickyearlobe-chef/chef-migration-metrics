// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { CREDENTIAL_TYPES, typeLabel, BADGE_LABELS, BADGE_STYLES } from "./constants";

// A credential type the server validates but the screen never offers is a type
// nobody can use.
//
// The pairing is the thing to hold: every type the server accepts must be
// choosable, and every choosable type must be labelled wherever credentials are
// listed, or it shows as a raw identifier.

describe("credential types", () => {
  // Kept in step with internal/secrets/validation.go — ValidCredentialTypes
  // there has a matching completeness test, so the two lists cannot drift apart
  // without one of them failing.
  const SERVER_TYPES = ["chef_client_key", "generic"];

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

  // A database connection is not one of these any more: it is configuration on
  // the import screen with only its password held here, so there is no
  // connection shape for this dialog to state. See
  // journeys/ownership-connection.md.
});
