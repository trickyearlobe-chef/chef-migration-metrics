// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { resolve as resolvePath } from "node:path";
import { describesShape } from "./ApiDocsPage";
import type { ApiSchema } from "../types/auth";

// The contract between the description and the page that renders it.
//
// Nothing checked that the page could say anything about the shapes the
// generator emits, and it could not: every list, map and one-of read as
// "not described", so a body the service spells out as a list of strings was
// reported as unknown and whoever read it went to the browser's network tab
// for something they had already been told. A gap in the page is
// indistinguishable from a gap in the description, and the wrong one gets
// fixed.
//
// So this reads what the service really emits — the recording the Go build
// keeps true, in internal/webapi/testdata/response_shapes.json — and fails
// when the page cannot describe one of them. It is not a second copy: nothing
// here is written down, the file is generated, and a Go test fails if it
// stops matching what is served.
//
// It covers the answers. Request bodies are described by the same reflection
// over the same types and so draw on the same vocabulary of shapes; what this
// cannot prove is that a shape only ever used by a body is renderable. That
// gap is named rather than papered over.

const record = JSON.parse(
  readFileSync(
    resolvePath(__dirname, "../../../internal/webapi/testdata/response_shapes.json"),
    "utf8",
  ),
) as Record<string, ApiSchema>;

describe("the reference page can describe everything the service describes", () => {
  it("has a recording to check against", () => {
    // A recording that has gone missing would make every assertion below pass
    // by vacuum.
    expect(Object.keys(record).length).toBeGreaterThan(50);
  });

  it("says something about every answer the description carries", () => {
    // The shapes are recorded resolved, so there is nothing to look up.
    const speechless = Object.entries(record)
      .filter(([, schema]) => !describesShape(schema, {}))
      .map(([operation]) => operation);

    expect(
      speechless,
      "the page renders these as undescribed, but the service describes them — " +
        "a reader cannot tell a gap in the page from a gap in the description",
    ).toEqual([]);
  });

  it("describes the shapes a body can be, not only the ones an answer can be", () => {
    // The vocabulary the generator emits, each an actual shape it produces:
    // a named type, a list of a named type, a list of scalars, a map, a
    // scalar, and one of several. Anything added to the generator belongs
    // here too.
    const emitted: Array<[string, ApiSchema]> = [
      ["an object", { type: "object", properties: { name: { type: "string" } } }],
      ["a list of objects", {
        type: "array",
        items: { type: "object", properties: { name: { type: "string" } } },
      }],
      ["a list of strings", { type: "array", items: { type: "string" } }],
      ["a map", { type: "object", additionalProperties: { type: "integer" } }],
      ["a scalar", { type: "string" }],
      ["one of several", {
        oneOf: [
          { type: "object", properties: { a: { type: "string" } } },
          { type: "object", properties: { b: { type: "string" } } },
        ],
      }],
      ["several at once", {
        allOf: [
          { type: "object", properties: { a: { type: "string" } } },
          { type: "object", properties: { b: { type: "string" } } },
        ],
      }],
    ];

    for (const [what, schema] of emitted) {
      expect(describesShape(schema, {}), `${what} reads as undescribed`).toBe(true);
    }
  });

  it("still calls an empty schema undescribed, because that is what it is", () => {
    // The generator emits {} where the service genuinely does not decide the
    // shape — the telemetry sink. Claiming to describe that would be worse
    // than the gap.
    expect(describesShape({}, {})).toBe(false);
    expect(describesShape(undefined, {})).toBe(false);
  });
});
