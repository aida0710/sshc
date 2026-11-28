import { describe, expect, it } from "vitest";
import {
  androidJapaneseLabels,
  japaneseAgentInstructions,
  japanesePages,
  japaneseReadme,
  japaneseRepositoryDocuments,
  type JapanesePage,
} from "../testing/japaneseDocuments";
import { engineCodeOutsideComments, repositoryComments } from "../testing/repositorySources";
import { androidCatalogue } from "./androidCatalogue";
import { ja, type MessageKey } from "./messages";

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
  ["同期キー", "暗号化キー"],
  ["接続エイリアス", "接続名"],
  ["接続先名", "接続名"],
  ["この機械", "このマシン"],
  ["二段目", "二要素認証"],
]);

type Wording = { text: string; location: string };

function linesOf(pages: JapanesePage[]): Wording[] {
  return pages.flatMap((page) => page.lines.map((text, index) => ({ text, location: `${page.path}:${index + 1}` })));
}

// Androidでだけ出す文。androidCatalogue.tsが画面の文を差し替えたものと言い換えたもの。ja.tsと同じ文は、
// 画面の文として読む。
function androidOnlyJapanese(): Wording[] {
  const android = androidCatalogue("ja");
  return (Object.keys(ja) as MessageKey[])
    .filter((key) => android[key] !== ja[key])
    .map((key) => ({ text: android[key], location: `androidCatalogue.ts ${key}` }));
}

// 利用者が読む日本語: 画面、Androidの文言、pages、sshcエンジンが書く文。
function userFacingJapanese(): Wording[] {
  const screen = Object.entries(ja).map(([key, text]) => ({ text, location: `ja.ts ${key}` }));
  const android = androidJapaneseLabels().map((text) => ({ text, location: "values-ja/strings.xml" }));
  return [...screen, ...androidOnlyJapanese(), ...android, ...linesOf(japanesePages()), ...engineCodeOutsideComments()];
}

function retiredTermsIn(wordings: Wording[], terms: Map<string, string>): string[] {
  return wordings.flatMap((wording) =>
    [...terms]
      .filter(([term]) => wording.text.includes(term))
      .map(([term, replacement]) => `${wording.location}: 「${term}」→「${replacement}」`),
  );
}

describe("利用者が読む日本語", () => {
  it("使わないと決めた語を含まない", () => {
    expect(retiredTermsIn(userFacingJapanese(), retiredTerms)).toEqual([]);
  });
});

// docs/の説明はpagesへ写されることがあり、AGENTS.mdの語はAIエージェントが画面やコメントへ写すので、
// pagesと同じ語で書いておく。
describe("README、AGENTS.md、docs/の日本語", () => {
  it("使わないと決めた語を含まない", () => {
    expect(retiredTermsIn(linesOf(japaneseRepositoryDocuments()), retiredTerms)).toEqual([]);
  });
});

// コードのコメントで探さない語と、その理由。ここに無い語は、コメントでも使わない。
const termsAllowedInComments = new Map<string, string>([
  ["端末", "コメントにはまだ残っている"],
  ["経路", "コメントにはまだ残っている"],
  ["ディレクトリ", "Goの標準ライブラリ（os.ReadDir、os.MkdirAllなど）と同じ語で説明する"],
  ["実行先", "コードの識別子runTargetを説明する語として残すと決めた"],
]);
// 使わない語そのものを、一覧と説明のコメントに書いているファイル。
const termListFile = "web/src/i18n/retiredJapaneseTerms.test.ts";

// AIエージェントはコメントの語を画面やpagesへ写すので、コメントも同じ語で書く。
describe("コードのコメント", () => {
  const checkedComments = () => repositoryComments().filter((comment) => !comment.location.startsWith(`${termListFile}:`));

  it("コメントで探さないとした語のほかは、使わないと決めた語を含まない", () => {
    const termsForComments = new Map([...retiredTerms].filter(([term]) => !termsAllowedInComments.has(term)));
    expect(retiredTermsIn(checkedComments(), termsForComments)).toEqual([]);
  });

  // コメントから無くなった語は、ここで落ちる。除外から外せば、コメントでも検査される。
  it("コメントで探さないとした語は、どれも一覧にあり、今もコメントで使っている", () => {
    const comments = checkedComments();
    const stale = [...termsAllowedInComments.keys()].filter(
      (term) => !retiredTerms.has(term) || !comments.some((comment) => comment.text.includes(term)),
    );
    expect(stale).toEqual([]);
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
  it("利用者が読む日本語と、README、AGENTS.md、docs/では「バージョン」と書く", () => {
    const wordings = [...userFacingJapanese(), ...linesOf(japaneseRepositoryDocuments())];
    expect(misusesIn(wordings, versionMeaningEdition)).toEqual([]);
  });
});

// 用語表の「シークレット」。「秘密」は「秘密鍵」（private key）では使うので、語だけでは誤りと言えない。
// 「秘密値」「秘密情報」でない形（「秘密のファイル」など）も拾えるよう、鍵が続かない「秘密」を探す。
const secretMeaningHimitsu: Misuse = { pattern: /秘密(?!鍵)/, correction: "「秘密」→「シークレット」" };

describe("secretの意味の「秘密」", () => {
  it("利用者が読む日本語では「シークレット」と書く", () => {
    expect(misusesIn(userFacingJapanese(), secretMeaningHimitsu)).toEqual([]);
  });
});

// 用語表の「sshcエンジン（文中）」。「エンジン」だけの形や「sshcのエンジン」は本文で使わない。
// Markdownの見出しとページのtitleは、今の表記のまま残してよいと決めた。
const engineWithoutProductName: Misuse = { pattern: /(?<!sshc)エンジン/, correction: "「エンジン」→「sshcエンジン」" };
const headingLine = /^(#{1,6}\s|title:)/;

describe("sshcエンジンの呼び方", () => {
  it("利用者が読む日本語と、README、AGENTS.mdの本文では「sshcエンジン」と書く", () => {
    const body = [...userFacingJapanese(), ...linesOf([japaneseReadme(), japaneseAgentInstructions()])].filter(
      (wording) => !headingLine.test(wording.text),
    );
    expect(misusesIn(body, engineWithoutProductName)).toEqual([]);
  });
});
