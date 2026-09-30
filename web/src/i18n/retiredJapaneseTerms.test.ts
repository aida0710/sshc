import { describe, expect, it } from "vitest";
import {
  androidJapaneseLabels,
  japanesePages,
  japaneseReadme,
  japaneseRepositoryDocuments,
  type JapanesePage,
} from "../testing/japaneseDocuments";
import { ja } from "./messages";

// docs/writing-style.mdの用語表で使わないと決めた語のうち、文脈によらず誤りと言えるもの。
// 「つなぐ」「名乗る」のように文脈によっては使える語は、誤検出になるので入れない。
const retiredTerms = new Map<string, string>([
  ["端末", "マシン（機器）またはターミナル（画面）"],
  ["資格情報", "認証情報"],
  ["施錠", "ロック"],
  ["解錠", "ロックを解除"],
  ["握手", "ハンドシェイク"],
  ["指紋", "フィンガープリント"],
  ["秘密値", "シークレット"],
  ["秘密情報", "シークレット"],
  ["ブラウザー", "ブラウザ"],
  ["フォルダー", "フォルダ"],
  ["エディター", "エディタ"],
]);

type Wording = { text: string; location: string };

function linesOf(pages: JapanesePage[]): Wording[] {
  return pages.flatMap((page) => page.lines.map((text, index) => ({ text, location: `${page.path}:${index + 1}` })));
}

function userFacingJapanese(): Wording[] {
  const screen = Object.entries(ja).map(([key, text]) => ({ text, location: `ja.ts ${key}` }));
  const android = androidJapaneseLabels().map((text) => ({ text, location: "values-ja/strings.xml" }));
  return [...screen, ...android, ...linesOf(japanesePages())];
}

function retiredTermsIn(wordings: Wording[]): string[] {
  return wordings.flatMap((wording) =>
    [...retiredTerms]
      .filter(([term]) => wording.text.includes(term))
      .map(([term, replacement]) => `${wording.location}: 「${term}」→「${replacement}」`),
  );
}

describe("利用者が読む日本語", () => {
  it("使わないと決めた語を含まない", () => {
    expect(retiredTermsIn(userFacingJapanese())).toEqual([]);
  });
});

// docs/の説明はpagesへ写されることがあるので、pagesと同じ語で書いておく。
describe("READMEとdocs/の日本語", () => {
  it("使わないと決めた語を含まない", () => {
    expect(retiredTermsIn(linesOf(japaneseRepositoryDocuments()))).toEqual([]);
  });
});

// 用語表の「バージョン」。「版」は「Android版」「デスクトップ版」のような種類の意味では使うので、
// 語だけでは誤りと言えない。バージョンを指すと分かる「前の版」「新しい版」「使っていた版」のような形だけを拾う。
const versionMeaningEdition = /(?:前の|以前の|今の|この|新しい|古い|別の|最新の|旧|た)版/;

function versionMeaningEditionsIn(wordings: Wording[]): string[] {
  return wordings
    .filter((wording) => versionMeaningEdition.test(wording.text))
    .map((wording) => `${wording.location}: 「版」（versionの意味）→「バージョン」`);
}

describe("versionの意味の「版」", () => {
  it("画面、pages、README、docs/では「バージョン」と書く", () => {
    const wordings = [...userFacingJapanese(), ...linesOf(japaneseRepositoryDocuments())];
    expect(versionMeaningEditionsIn(wordings)).toEqual([]);
  });
});

// 用語表の「sshcエンジン（文中）」。「エンジン」だけの形や「sshcのエンジン」は本文で使わない。
// Markdownの見出しとページのtitleは、今の表記のまま残してよいと決めた。
const engineWithoutProductName = /(?<!sshc)エンジン/;
const headingLine = /^(#{1,6}\s|title:)/;

function bareEngineNamesIn(wordings: Wording[]): string[] {
  return wordings
    .filter((wording) => !headingLine.test(wording.text) && engineWithoutProductName.test(wording.text))
    .map((wording) => `${wording.location}: 「エンジン」→「sshcエンジン」`);
}

describe("sshcエンジンの呼び方", () => {
  it("画面、pages、READMEの本文では「sshcエンジン」と書く", () => {
    const wordings = [...userFacingJapanese(), ...linesOf([japaneseReadme()])];
    expect(bareEngineNamesIn(wordings)).toEqual([]);
  });
});
