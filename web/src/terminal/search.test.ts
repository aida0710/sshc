import { describe, expect, it } from "vitest";
import { findBufferMatches } from "./search";

describe("terminal scrollback search", () => {
  it("finds every case-insensitive match", () => {
    expect(findBufferMatches(["Alpha beta alpha", "none"], "ALPHA")).toEqual([
      { row: 0, column: 0, length: 5 },
      { row: 0, column: 11, length: 5 },
    ]);
  });
});
