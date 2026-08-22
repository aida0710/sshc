import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { knownPalette, palettes } from "./palettes";

// 配色は 2 つの場所に跨がっている。名前は palettes.ts、色は index.css。
//
// **跨がっているものは、跨がったまま検める。** CSS にしか無い名前は選べない
// 配色になり、TypeScript にしか無い名前は選べるのに何も変わらない配色になる。
// どちらも、動かしてみるまで誰も気付かない種類の壊れ方である。

const css = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "..", "index.css"), "utf8");

/** definedPalettes は、CSS が実際に色を与えている名前と、その中身を返す。 */
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

/**
 * themeTokens は、テーマが与えている端末の**色**の全部である。
 *
 * <p>**数え上げない。** 20 個をここに並べると、21 個目を足した日に、この検査は
 * 何も言わないまま古い数を守り続ける。
 *
 * <p>ただし `[data-term-…]` の下で定義されるものは数えない。あそこにあるのは
 * 配色ではなく、選ばれた画像や濃さから作られる値である——**配色に「画像」を
 * 定義せよと要求しても意味がない。**
 */
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

  // **半分だけの配色を出荷しない。** 1 つ欠けると、その色だけがテーマから
  // 漏れてくる——Dracula の上に前のテーマの赤が混ざる。
  it("gives every palette every colour the theme gives", () => {
    const expected = themeTokens();
    const incomplete: string[] = [];
    for (const [name, tokens] of definedPalettes()) {
      const missing = [...expected].filter((token) => !tokens.has(token));
      if (missing.length > 0) incomplete.push(`${name}: ${missing.join(", ")}`);
    }
    expect(incomplete).toEqual([]);
  });

  // **知らない名前で端末が開けなくなってはならない。**
  it("treats a name it does not know as no choice at all", () => {
    expect(knownPalette("dracula")?.label).toBe("Dracula");
    expect(knownPalette("nope")).toBeNull();
    expect(knownPalette("")).toBeNull();
  });
});
