import { describe, expect, it } from "vitest";
import { emptyTerminalArtOf, withEmptyTerminalArt } from "./emptyTerminalArt";

describe("empty terminal art choice", () => {
  it("draws nothing for a preset that has not chosen, or names an art this version does not know", () => {
    expect(emptyTerminalArtOf({ id: "default" })).toBe("none");
    expect(emptyTerminalArtOf({ id: "default", appearance: { emptyTerminalArt: "aquarium" as never } })).toBe("none");
  });

  it("keeps the default as no value and reads back the art chosen", () => {
    const chosen = withEmptyTerminalArt({ id: "default", appearance: { accent: "sky" } }, "globe");
    expect(emptyTerminalArtOf(chosen)).toBe("globe");
    expect(withEmptyTerminalArt(chosen, "none").appearance).toEqual({ accent: "sky" });
  });
});
