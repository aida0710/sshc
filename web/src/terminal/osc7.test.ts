import { describe, expect, it } from "vitest";
import { parseOSC7Directory } from "./osc7";

describe("OSC 7 working directory", () => {
  it("accepts an absolute file URI and decodes its path", () => {
    expect(parseOSC7Directory("file://server/home/aida/My%20Files")).toBe("/home/aida/My Files");
  });

  it("rejects commands and non-file locations", () => {
    expect(parseOSC7Directory("https://example.test/home")).toBeNull();
    expect(parseOSC7Directory("file://server/home/aida%0Aecho%20oops")).toBeNull();
  });
});
