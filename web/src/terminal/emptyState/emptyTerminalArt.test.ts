import { describe, expect, it } from "vitest";
import { emptyTerminalArtOf, withEmptyTerminalArt } from "./emptyTerminalArt";

describe("empty terminal art choice", () => {
  it("draws the network for a preset that has not chosen, or names an art this version does not know", () => {
    expect(emptyTerminalArtOf({ id: "default" })).toBe("network");
    expect(emptyTerminalArtOf({ id: "default", appearance: { emptyTerminalArt: "lava-lamp" as never } })).toBe("network");
  });

  it("keeps the default as no value and reads back the art chosen", () => {
    const chosen = withEmptyTerminalArt({ id: "default", appearance: { accent: "sky" } }, "globe");
    expect(emptyTerminalArtOf(chosen)).toBe("globe");
    expect(withEmptyTerminalArt(chosen, "network").appearance).toEqual({ accent: "sky" });
  });

  it("keeps a choice of no picture, so it is not taken for the default", () => {
    const nothing = withEmptyTerminalArt({ id: "default" }, "none");
    expect(nothing.appearance).toEqual({ emptyTerminalArt: "none" });
    expect(emptyTerminalArtOf(nothing)).toBe("none");
  });
});
