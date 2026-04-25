// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { getBaseColour, getSegmentColour } from "./colours";

describe("getBaseColour", () => {
  it("newest major gets green", () => {
    expect(getBaseColour(0).name).toBe("green");
  });

  it("second major gets blue", () => {
    expect(getBaseColour(1).name).toBe("blue");
  });

  it("third major gets amber", () => {
    expect(getBaseColour(2).name).toBe("amber");
  });

  it("fourth and older get red", () => {
    expect(getBaseColour(3).name).toBe("red");
    expect(getBaseColour(4).name).toBe("red");
    expect(getBaseColour(10).name).toBe("red");
  });
});

describe("getSegmentColour", () => {
  it("returns darkest shade for single version", () => {
    const colour = getSegmentColour(0, 0, 1);
    expect(colour).toBe(getBaseColour(0).shades[0]);
  });

  it("produces distinguishable values for 1-6 minors", () => {
    for (let count = 1; count <= 6; count++) {
      const colours = new Set<string>();
      for (let i = 0; i < count; i++) {
        colours.add(getSegmentColour(0, i, count));
      }
      // All colours should be different when count <= 6 shades
      expect(colours.size).toBe(count);
    }
  });

  it("newest minor gets darkest shade, oldest gets lightest", () => {
    const dark = getSegmentColour(0, 0, 3);
    const light = getSegmentColour(0, 2, 3);
    const shades = getBaseColour(0).shades;
    expect(dark).toBe(shades[0]);
    expect(light).toBe(shades[shades.length - 1]);
  });
});
