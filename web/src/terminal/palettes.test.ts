import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { knownPalette, palettes } from "./palettes";


const css = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "..", "index.css"), "utf8");
function definedPalettes(): Map<string, Set<string>> {
  const found = new Map<string, Set<string>>();
  const block = /\[data-term-palette="([^"]+)"\]\s*\{([^}]*)\}/g;
  for (const [, name, body] of css.matchAll(block)) {
    const tokens = new Set<string>();
    for (const [, token] of (body ?? "").matchAll(/(--ui-term-[a-z-]+)\s*:/g)) {
      if (token !== undefined) tokens.add(token);
    }
    found.set(name ?? "", tokens);
  }
  return found;
}
function themeTokens(): Set<string> {
  const tokens = new Set<string>();
  for (const [, selector, body] of css.matchAll(/([^{}]+)\{([^}]*)\}/g)) {
    if ((selector ?? "").includes("[data-term-")) continue;
    for (const [, token] of (body ?? "").matchAll(/(--ui-term-[a-z-]+)\s*:/g)) {
      if (token !== undefined) tokens.add(token);
    }
  }
  return tokens;
}

describe("端末の配色", () => {
  it("names the same palettes the stylesheet paints", () => {
    expect([...definedPalettes().keys()].sort()).toEqual(palettes.map((palette) => palette.name).sort());
  });

  it("gives every palette every colour the theme gives", () => {
    const expected = themeTokens();
    const incomplete: string[] = [];
    for (const [name, tokens] of definedPalettes()) {
      const missing = [...expected].filter((token) => !tokens.has(token));
      if (missing.length > 0) incomplete.push(`${name}: ${missing.join(", ")}`);
    }
    expect(incomplete).toEqual([]);
  });

  it("treats a name it does not know as no choice at all", () => {
    expect(knownPalette("dracula")?.label).toBe("Dracula");
    expect(knownPalette("nope")).toBeNull();
    expect(knownPalette("")).toBeNull();
  });
});
