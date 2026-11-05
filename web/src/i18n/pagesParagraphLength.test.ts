import { describe, expect, it } from "vitest";
import { japanesePages, type JapanesePage } from "../testing/japaneseDocuments";

// スマホの幅（1行に約20文字）で15行になる長さ。これを超える段落は、機能を足すたびに文を足して
// 読めなくなった形なので、docs/writing-style.mdの「pagesの書き方」に沿って節、表、箇条書きに分ける。
const longestParagraphCharacters = 300;

function isProse(line: string): boolean {
  const trimmed = line.trimStart();
  // 表の行は列ごとに読むので、1行が長くても読みにくくならない。トップページの本文はHTMLで書いている。
  return !trimmed.startsWith("|") && !trimmed.startsWith("<");
}

function paragraphsOverTheLimit(page: JapanesePage): string[] {
  let insideCodeBlock = false;
  return page.lines.flatMap((line, index) => {
    if (line.startsWith("```")) insideCodeBlock = !insideCodeBlock;
    if (insideCodeBlock || !isProse(line) || line.length <= longestParagraphCharacters) return [];
    return [`${page.path}:${index + 1} ${line.length}文字: ${line.slice(0, 30)}…`];
  });
}

describe("日本語のpagesの段落", () => {
  it("スマホで読める長さに収まる", () => {
    expect(japanesePages().flatMap(paragraphsOverTheLimit)).toEqual([]);
  });
});
