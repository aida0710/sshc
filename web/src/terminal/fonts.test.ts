import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { defaultStack, fontStack, fonts, knownFont } from "./fonts";

// 字体は 3 つの場所に跨がっている。名前は fonts.ts、@font-face は index.css、
// 実物は public/fonts。
//
// **跨がっているものは、跨がったまま検める。** 選べるのに入っていない字体は
// 黙って代替へ落ち、入っているのに選べない字体はただの重さである。どちらも、
// 実機で見るまで誰も気付かない。

const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(here, "..", "index.css"), "utf8");

/** faces は、CSS が宣言している面を返す。 */
function faces(): { family: string; weight: string; style: string; url: string }[] {
  const found: { family: string; weight: string; style: string; url: string }[] = [];
  for (const [, body] of css.matchAll(/@font-face\s*\{([^}]*)\}/g)) {
    const text = body ?? "";
    const read = (property: string) =>
      new RegExp(`${property}\\s*:\\s*([^;]+);`).exec(text)?.[1]?.trim() ?? "";
    found.push({
      family: read("font-family").replace(/^"|"$/g, ""),
      weight: read("font-weight"),
      style: read("font-style"),
      url: /url\("([^"]+)"\)/.exec(text)?.[1] ?? "",
    });
  }
  return found;
}

describe("端末の字体", () => {
  // **積んだ 4 面が揃っていること。** 太字だけ欠けると、ブラウザが合成して
  // 桁が広がる——プロンプトの行だけがずれる。
  it("ships every weight and slant the terminal actually draws", () => {
    const wanted = [
      ["400", "normal"],
      ["700", "normal"],
      ["400", "italic"],
      ["700", "italic"],
    ];
    for (const family of new Set(faces().map((face) => face.family))) {
      const mine = faces().filter((face) => face.family === family);
      const missing = wanted.filter(
        ([weight, style]) => !mine.some((face) => face.weight === weight && face.style === style),
      );
      expect(missing, `${family} に足りない面`).toEqual([]);
    }
  });

  // **名前があって実物が無いと、黙って代替へ落ちる。**
  it("has the file behind every face it declares", () => {
    const missing = faces()
      .map((face) => face.url)
      .filter((url) => !existsSync(join(here, "..", "..", "public", url)));
    expect(missing).toEqual([]);
  });

  // **入っているのに選べない字体は、ただの重さである。**
  it("offers every family it bundles, and bundles every family it offers", () => {
    const bundled = [...new Set(faces().map((face) => face.family))].sort();
    const offered = fonts
      .map((font) => /^"([^"]+)"/.exec(font.stack)?.[1] ?? "")
      .filter((family) => family !== "")
      .sort();
    expect(offered).toEqual(bundled);
  });

  // **代替を必ず持つ。** 同梱の名前だけを書くと、読み込みが失敗した瞬間に
  // 端末が読めなくなる。
  it("always keeps a fallback behind the bundled name", () => {
    for (const font of fonts) expect(font.stack.endsWith("monospace")).toBe(true);
  });

  it("treats a name it does not know as no choice at all", () => {
    expect(knownFont("jetbrains-mono")?.label).toBe("JetBrains Mono");
    expect(knownFont("nope")).toBeNull();
    expect(fontStack("nope")).toBe(defaultStack);
    expect(fontStack("")).toBe(defaultStack);
  });
});
