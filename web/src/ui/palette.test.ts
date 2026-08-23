import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const palettes = [
  "slate", "gray", "grey", "zinc", "neutral", "stone",
  "red", "orange", "amber", "yellow", "lime", "green", "emerald",
  "teal", "cyan", "sky", "blue", "indigo", "violet", "purple",
  "fuchsia", "pink", "rose",
];

const literal = new RegExp(
  String.raw`\b(?:bg|text|border|outline|ring|accent|fill|stroke|from|to|via|decoration|divide|shadow)-(?:${palettes.join("|")})-\d{2,3}\b`,
  "g",
);

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.tsx?$/.test(path) && !/\.test\.tsx?$/.test(path) ? [path] : [];
  });
}

const arbitrary = /\b(?:bg|text|border|outline|ring|fill|stroke|shadow|from|to|via)-\[#[0-9a-fA-F]{3,8}\]/g;
const rawHex = /#[0-9a-fA-F]{6}\b/g;
const exemption = "palette-exempt";

describe("the palette", () => {
  it("is the only source of colour in the application", () => {
    const offenders: string[] = [];
    for (const path of sources(join(__dirname, ".."))) {
      if (path.endsWith("palette.test.ts")) continue;
      readFileSync(path, "utf8")
        .split("\n")
        .forEach((line, index) => {
          const where = `${path.slice(path.indexOf("/src/") + 1)}:${index + 1}`;
          for (const found of line.match(literal) ?? []) offenders.push(`${where} ${found}`);
          for (const found of line.match(arbitrary) ?? []) offenders.push(`${where} ${found}`);
          if (line.includes(exemption)) return;
          for (const found of line.match(rawHex) ?? []) offenders.push(`${where} ${found}`);
        });
    }

    expect(offenders, `use a token from index.css instead:\n${offenders.join("\n")}`).toEqual([]);
  });
});
