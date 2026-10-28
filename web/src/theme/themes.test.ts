import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { colorThemes } from "./theme";

// Adding a theme takes a line in colorThemes, a file in themes/ and an @import in index.css. These checks
// name the step that was missed, and that the new file paints everything the others do: a colour a theme
// leaves out would show the light theme's.

const themesDirectory = join(__dirname, "themes");

function themeFile(name: string): string {
  return readFileSync(join(themesDirectory, `${name}.css`), "utf8");
}

function declaredColours(css: string): string[] {
  return [...css.matchAll(/^\s*(--ui-[a-z-]+):/gm)].map((match) => match[1]!).sort();
}

function declaredScheme(css: string): string | undefined {
  return /^\s*color-scheme:\s*(light|dark);/m.exec(css)?.[1];
}

describe("the themes", () => {
  it("have one file each in themes/, and every file is a theme", () => {
    const files = readdirSync(themesDirectory).filter((file) => file.endsWith(".css")).sort();
    expect(files).toEqual(colorThemes.map((theme) => `${theme.name}.css`).sort());
  });

  it("are all imported by index.css", () => {
    const index = readFileSync(join(__dirname, "..", "index.css"), "utf8");
    for (const theme of colorThemes) expect(index).toContain(`@import "./theme/themes/${theme.name}.css";`);
  });

  it("paint under their own name with their scheme", () => {
    for (const theme of colorThemes) {
      const css = themeFile(theme.name);
      expect(css, theme.name).toContain(`[data-theme="${theme.name}"] {`);
      expect(declaredScheme(css), theme.name).toBe(theme.scheme);
    }
  });

  it("each set every colour the light theme sets", () => {
    const light = declaredColours(themeFile("light"));
    for (const theme of colorThemes) expect(declaredColours(themeFile(theme.name)), theme.name).toEqual(light);
  });

  it("give the top bar a plain #rrggbb, which Android paints its system bars with", () => {
    for (const theme of colorThemes) {
      expect(/^\s*--ui-toolbar:\s*(#[0-9a-f]{6});/m.exec(themeFile(theme.name))?.[1], theme.name).toMatch(/^#[0-9a-f]{6}$/);
    }
  });
});
