import { describe, expect, it } from "vitest";
import { applicationSourceLines, mayHoldClassNames, readStylesheet } from "../testing/applicationSources";

// ページの中の重なりは z-50 までの数で書く。それより手前の層は index.css が名前を付ける。
const highestPlainLayer = 50;
const layerClass = /(?<![\w-])-?z-(?:\[(-?\d+)\]|(\d+))(?![\w-])/g;

function layersAboveThePage(line: string): string[] {
  if (!mayHoldClassNames(line)) return [];
  return [...line.matchAll(layerClass)]
    .filter((found) => Number(found[1] ?? found[2]) > highestPlainLayer)
    .map((found) => found[0]);
}

describe("the layers drawn over the whole screen", () => {
  it("are named in index.css, each above the menus and dialogs and in this order", () => {
    const named = [...readStylesheet().matchAll(/@utility z-([a-z-]+) \{\s*z-index: (\d+);/g)].map((found) => [found[1], Number(found[2])]);

    expect(named).toEqual([["palette", 70], ["above-dialog", 80], ["fallback", 100]]);
  });
});

describe("looking for a layer above the page that has no name", () => {
  it("reports a number above the menus and dialogs, written either way", () => {
    expect(layersAboveThePage('<div className="fixed z-[80] w-64" />')).toEqual(["z-[80]"]);
    expect(layersAboveThePage('<div className="fixed z-60" />')).toEqual(["z-60"]);
  });

  it("accepts the layers inside a page, up to the menus and dialogs", () => {
    expect(layersAboveThePage('<div className="sticky top-0 z-[2] z-10 z-50 -z-10" />')).toEqual([]);
  });

  it("accepts a layer taken by its name", () => {
    expect(layersAboveThePage('<div className="fixed z-above-dialog" />')).toEqual([]);
  });

  it("ignores the word in a comment", () => {
    expect(layersAboveThePage("// stays under the inspector (z-70)")).toEqual([]);
  });
});

describe("the application", () => {
  it("takes every layer above the menus and dialogs by its name", () => {
    const offenders = applicationSourceLines().flatMap((line) => layersAboveThePage(line.text).map((found) => `${line.where} ${found}`));

    expect(offenders, `use a layer named in index.css instead:\n${offenders.join("\n")}`).toEqual([]);
  });
});
