import { describe, expect, it } from "vitest";
import {
  androidJapaneseLabels,
  japaneseAgentInstructions,
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
  ["設定の組", "プリセット"],
  ["ブラウザー", "ブラウザ"],
  ["フォルダー", "フォルダ"],
  ["エディター", "エディタ"],
  ["ディレクトリ", "フォルダ"],
  ["経路", "ルート（手段の意味なら「別の手段」）"],
  ["リモートホスト", "接続先"],
  ["実行先", "リモートセッション（マシンの意味なら「リモートのsshcエンジン」）"],
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

// docs/の説明はpagesへ写されることがあり、AGENTS.mdの語はAIエージェントが画面やコメントへ写すので、
// pagesと同じ語で書いておく。
describe("README、AGENTS.md、docs/の日本語", () => {
  it("使わないと決めた語を含まない", () => {
    expect(retiredTermsIn(linesOf(japaneseRepositoryDocuments()))).toEqual([]);
  });
});

// 語だけでは誤りと言えない語は、誤りと分かる形（pattern）で探す。correction は直し方。
type Misuse = { pattern: RegExp; correction: string };

function misusesIn(wordings: Wording[], misuse: Misuse): string[] {
  return wordings
    .filter((wording) => misuse.pattern.test(wording.text))
    .map((wording) => `${wording.location}: ${misuse.correction}`);
}

// 用語表の「バージョン」。「版」は「Android版」「デスクトップ版」のような種類の意味では使うので、
// 語だけでは誤りと言えない。バージョンを指すと分かる「前の版」「新しい版」「使っていた版」のような形だけを拾う。
const versionMeaningEdition: Misuse = {
  pattern: /(?:前の|以前の|今の|この|新しい|古い|別の|最新の|旧|た)版/,
  correction: "「版」（versionの意味）→「バージョン」",
};

describe("versionの意味の「版」", () => {
  it("画面、pages、README、AGENTS.md、docs/では「バージョン」と書く", () => {
    const wordings = [...userFacingJapanese(), ...linesOf(japaneseRepositoryDocuments())];
    expect(misusesIn(wordings, versionMeaningEdition)).toEqual([]);
  });
});

// 用語表の「シークレット」。「秘密」は「秘密鍵」（private key）では使うので、語だけでは誤りと言えない。
// 「秘密値」「秘密情報」でない形（「秘密のファイル」など）も拾えるよう、鍵が続かない「秘密」を探す。
const secretMeaningHimitsu: Misuse = { pattern: /秘密(?!鍵)/, correction: "「秘密」→「シークレット」" };

describe("secretの意味の「秘密」", () => {
  it("画面、Androidの文言、pagesでは「シークレット」と書く", () => {
    expect(misusesIn(userFacingJapanese(), secretMeaningHimitsu)).toEqual([]);
  });
});

// 用語表の「sshcエンジン（文中）」。「エンジン」だけの形や「sshcのエンジン」は本文で使わない。
// Markdownの見出しとページのtitleは、今の表記のまま残してよいと決めた。
const engineWithoutProductName: Misuse = { pattern: /(?<!sshc)エンジン/, correction: "「エンジン」→「sshcエンジン」" };
const headingLine = /^(#{1,6}\s|title:)/;

describe("sshcエンジンの呼び方", () => {
  it("画面、pages、README、AGENTS.mdの本文では「sshcエンジン」と書く", () => {
    const body = [...userFacingJapanese(), ...linesOf([japaneseReadme(), japaneseAgentInstructions()])].filter(
      (wording) => !headingLine.test(wording.text),
    );
    expect(misusesIn(body, engineWithoutProductName)).toEqual([]);
  });
});
