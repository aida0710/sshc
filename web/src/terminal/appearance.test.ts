import { describe, expect, it } from "vitest";
import { chooseAppearance, resolveAppearance } from "./appearance";

describe("resolveAppearance", () => {
  it("lets the connection win over the overall choice", () => {
    expect(resolveAppearance({ palette: "nord" }, { palette: "dracula" }).palette).toBe("nord");
  });

  it("falls back to the overall choice", () => {
    expect(resolveAppearance({}, { palette: "dracula" }).palette).toBe("dracula");
    expect(resolveAppearance(undefined, { palette: "dracula" }).palette).toBe("dracula");
  });

  it("keeps the overall font when the connection only chose a palette", () => {
    const resolved = resolveAppearance({ palette: "nord" }, { palette: "dracula", font: "jetbrains-mono" });
    expect(resolved).toEqual({ palette: "nord", font: "jetbrains-mono", background: "", tint: undefined });
  });

  it("reads an empty choice as no choice, not as a reset of everything", () => {
    expect(resolveAppearance({ palette: "" }, { palette: "dracula" }).palette).toBe("dracula");
  });

  it("chooses nothing when nobody chose", () => {
    expect(resolveAppearance(undefined, undefined)).toEqual({
      palette: "",
      font: "",
      background: "",
      tint: undefined,
    });
  });
});

describe("かぶせる濃さ", () => {
  it("keeps a zero the connection actually chose", () => {
    expect(resolveAppearance({ backgroundTint: 0 }, { backgroundTint: 80 }).tint).toBe(0);
  });

  it("falls back only when the connection chose nothing", () => {
    expect(resolveAppearance({}, { backgroundTint: 80 }).tint).toBe(80);
    expect(resolveAppearance(undefined, undefined).tint).toBeUndefined();
  });
});

const identity = { path: "config", alias: "prod" };

describe("chooseAppearance", () => {
  it("changes one choice and leaves the other alone", () => {
    const next = chooseAppearance({ identity, appearance: { font: "jetbrains-mono" } }, { palette: "nord" });
    expect(next.appearance).toEqual({ font: "jetbrains-mono", palette: "nord" });
  });

  it("adds the section when there was none", () => {
    expect(chooseAppearance({ identity }, { palette: "nord" }).appearance).toEqual({ palette: "nord" });
  });

  it("drops the section once nothing is chosen any more", () => {
    const next = chooseAppearance({ identity, appearance: { palette: "nord" } }, { palette: "" });
    expect("appearance" in next).toBe(false);
  });

  it("keeps the section while something else is still chosen", () => {
    const next = chooseAppearance(
      { identity, appearance: { palette: "nord", font: "jetbrains-mono" } },
      { palette: "" },
    );
    expect(next.appearance).toEqual({ font: "jetbrains-mono" });
  });

  it("carries every other field of the metadata untouched", () => {
    const next = chooseAppearance({ identity, colour: "#123456", tags: ["a"] }, { palette: "nord" });
    expect(next).toEqual({ identity, colour: "#123456", tags: ["a"], appearance: { palette: "nord" } });
  });
});
