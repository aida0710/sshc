import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { defaultStack, fontStack, fonts, knownFont, symbolFontFamily } from "./fonts";


const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(here, "..", "index.css"), "utf8");
type Face = { family: string; weight: string; style: string; url: string; unicodeRange: string };
function faces(): Face[] {
  const found: Face[] = [];
  for (const [, body] of css.matchAll(/@font-face\s*\{([^}]*)\}/g)) {
    const text = body ?? "";
    const read = (property: string) =>
      new RegExp(`${property}\\s*:\\s*([^;]+);`).exec(text)?.[1]?.trim() ?? "";
    found.push({
      family: read("font-family").replace(/^"|"$/g, ""),
      weight: read("font-weight"),
      style: read("font-style"),
      url: /url\("([^"]+)"\)/.exec(text)?.[1] ?? "",
      unicodeRange: read("unicode-range"),
    });
  }
  return found;
}

// The fonts a person chooses for the text, unlike the symbols behind every one of them.
function textFaces(): Face[] {
  return faces().filter((face) => face.family !== symbolFontFamily);
}

describe("端末の字体", () => {
  it("ships every weight and slant the terminal actually draws", () => {
    const wanted = [
      ["400", "normal"],
      ["700", "normal"],
      ["400", "italic"],
      ["700", "italic"],
    ];
    for (const family of new Set(textFaces().map((face) => face.family))) {
      const mine = textFaces().filter((face) => face.family === family);
      const missing = wanted.filter(
        ([weight, style]) => !mine.some((face) => face.weight === weight && face.style === style),
      );
      expect(missing, `${family} に足りない面`).toEqual([]);
    }
  });

  it("has the file behind every face it declares", () => {
    const missing = faces()
      .map((face) => face.url)
      .filter((url) => !existsSync(join(here, "..", "..", "public", url)));
    expect(missing).toEqual([]);
  });

  it("offers every family it bundles, and bundles every family it offers", () => {
    const bundled = [...new Set(textFaces().map((face) => face.family))].sort();
    const offered = fonts
      .map((font) => /^"([^"]+)"/.exec(font.stack)?.[1] ?? "")
      .filter((family) => family !== "")
      .sort();
    expect(offered).toEqual(bundled);
  });

  it("always keeps a fallback behind the bundled name", () => {
    for (const font of fonts) expect(font.stack.endsWith("monospace")).toBe(true);
  });

  it("falls back to the Windows monospace fonts before the browser's generic one, after the other systems' fonts", () => {
    const families = defaultStack.split(",").map((family) => family.trim().replace(/^"|"$/g, ""));
    const firstWindowsFont = families.indexOf("Cascadia Mono");
    expect(families.slice(firstWindowsFont)).toEqual(["Cascadia Mono", "Consolas", symbolFontFamily, "monospace"]);
    expect(families.slice(0, firstWindowsFont)).toEqual(
      expect.arrayContaining(["Menlo", "Roboto Mono", "Droid Sans Mono"]),
    );
  });

  it("draws only the symbols with the bundled symbol font, behind the text fonts of every stack", () => {
    const symbols = faces().filter((face) => face.family === symbolFontFamily);
    expect(symbols.map((face) => face.unicodeRange)).toEqual(["U+23F4-23FA"]);
    for (const stack of [defaultStack, ...fonts.map((font) => font.stack)]) {
      expect(stack.endsWith(`"${symbolFontFamily}", monospace`)).toBe(true);
    }
  });

  it("treats a name it does not know as no choice at all", () => {
    expect(knownFont("jetbrains-mono")?.label).toBe("JetBrains Mono");
    expect(knownFont("nope")).toBeNull();
    expect(fontStack("nope")).toBe(defaultStack);
    expect(fontStack("")).toBe(defaultStack);
  });
});
