import { describe, expect, it } from "vitest";
import { applicationSourceLines, mayHoldClassNames, readStylesheet } from "../testing/applicationSources";

// Tailwind の段階のうち、使ってよいもの。これより大きい文字は使っていない。
const tailwindSteps = ["xs", "sm", "base", "lg", "xl", "2xl"];

// index.css の @theme が足した段階を、名前と大きさで読む。
function stepsAddedInTheme(): Record<string, string> {
  return Object.fromEntries([...readStylesheet().matchAll(/^\s*--text-([a-z0-9]+):\s*([^;]+);/gm)].map((found) => [found[1], found[2]]));
}

// 文字の大きさのクラスは text-xs か text-[13px] の形をとる。text-ink（色）や text-left（そろえ方）は大きさではない。
const tailwindSizeName = /^(?:\d?x[sl]|sm|base|lg)$/;
const lengthWritten = /^\[[\d.]+(?:px|rem|em)\]$/;
// 丸い印の中に描く記号（✓ や ! ）は、8px 以下の大きさで描く。文字の段階には入れない。
const signInsideMark = /^\[[1-8]px\]$/;
const textClass = /(?<![\w-])text-([\w.[\]]+)(?![\w-])/g;

function fontSizesOutsideScale(line: string, added: string[]): string[] {
  if (!mayHoldClassNames(line)) return [];
  return [...line.matchAll(textClass)]
    .filter((found) => {
      const value = found[1] ?? "";
      if (signInsideMark.test(value)) return false;
      if (lengthWritten.test(value)) return true;
      return tailwindSizeName.test(value) && !tailwindSteps.includes(value) && !added.includes(value);
    })
    .map((found) => found[0]);
}

describe("the font size scale", () => {
  it("adds three steps to Tailwind's, each given in pixels", () => {
    expect(stepsAddedInTheme()).toEqual({ "3xs": "10px", "2xs": "11px", ui: "13px" });
  });

  it("gives the steps it adds no line height, so a field keeps the line height it sets", () => {
    expect(readStylesheet()).not.toMatch(/--text-[a-z0-9]+--line-height/);
  });
});

describe("looking for a font size outside the scale", () => {
  const added = ["2xs", "ui"];

  it("reports a size written in pixels, whole or half", () => {
    expect(fontSizesOutsideScale('<p className="text-[13px] text-ink" />', added)).toEqual(["text-[13px]"]);
    expect(fontSizesOutsideScale('<p className="md:text-[12.5px]" />', added)).toEqual(["text-[12.5px]"]);
  });

  it("reports a size written in rem", () => {
    expect(fontSizesOutsideScale('<span className="text-[0.65rem]" />', added)).toEqual(["text-[0.65rem]"]);
  });

  it("reports a Tailwind step larger than the scale has", () => {
    expect(fontSizesOutsideScale('<h1 className="text-3xl font-semibold" />', added)).toEqual(["text-3xl"]);
  });

  it("accepts Tailwind's steps and the ones index.css adds", () => {
    expect(fontSizesOutsideScale('<p className="text-xs text-sm text-base text-lg text-xl text-2xl text-2xs text-ui" />', added)).toEqual([]);
  });

  it("accepts a colour, an alignment and a wrapping, which share the prefix", () => {
    expect(fontSizesOutsideScale('<p className="text-ink text-left text-balance text-[color-mix(in_srgb,red,blue)]" />', added)).toEqual([]);
  });

  it("accepts the sign drawn inside a round mark", () => {
    expect(fontSizesOutsideScale('<span className="size-[11px] text-[8px]" />', added)).toEqual([]);
    expect(fontSizesOutsideScale('<span className="text-[9px]" />', added)).toEqual(["text-[9px]"]);
  });

  it("ignores the word in a comment", () => {
    expect(fontSizesOutsideScale("// the row's text-[13px] name", added)).toEqual([]);
  });
});

describe("the application", () => {
  it("takes every font size from the scale", () => {
    const added = Object.keys(stepsAddedInTheme());
    const offenders = applicationSourceLines().flatMap((line) => fontSizesOutsideScale(line.text, added).map((found) => `${line.where} ${found}`));

    expect(offenders, `use a step of the scale instead (index.css names the ones it adds):\n${offenders.join("\n")}`).toEqual([]);
  });
});
