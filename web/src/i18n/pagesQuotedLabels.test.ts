import { describe, expect, it } from "vitest";
import { androidJapaneseLabels, japanesePages } from "../testing/japaneseDocuments";
import { ja } from "./messages";

// 日本語のpagesは、画面のボタン名を［…］、画面に出る文を「…」で引用する。
// 画面の文言を変えても引用は自動では追従しないので、引用が今の画面の文言と一致するかをここで確かめる。

// 引用の形で書くが、sshcの画面の文言カタログにはないもの。足すときは理由も書く。
const quotesOutsideTheCatalogue = new Map<string, string>([
  ["…", "メニューを開くボタンの記号"],
  ["動きを減らす", "Androidの設定の名前"],
  ["アプリをインストール", "ChromeとEdgeのメニューの名前"],
  ["名前 → リンク先", "SFTPの一覧の表示形式の例。SFTPEntryList.tsxが組み立てる"],
  ["sshcエンジンの記録", "VPNのログの見出し。internal/vpn/status.goが書く"],
  ["コンテナのログ", "VPNのログの見出し。internal/vpn/status.goが書く"],
  ["Toggle Tab Key Moves Focus", "Monaco Editorのコマンドの名前。Monaco Editorが英語で表示する"],
  ["プライバシーとセキュリティ", "macOSのシステム設定の項目の名前"],
  ["フルディスクアクセス", "macOSのシステム設定の項目の名前"],
  ["ファイルとフォルダ", "macOSのシステム設定の項目の名前"],
  ["+", "macOSのシステム設定で、一覧に項目を追加するボタン"],
  ["-", "macOSのシステム設定で、一覧から項目を削除するボタン"],
]);

const quotePattern = /［([^［］]+)］|「([^「」]+)」/g;
const placeholderPattern = /\{[^{}]+\}/g;
// 「（バイト）」のような末尾の補足は、本文で省いて引用してよい。
const trailingNotePattern = /（[^（）]*）$/;
// {name}の差し込みは、pagesでは英数字の例（「bastionへ接続中 · 1/2」）か<名前>の形で書く。
// 日本語の文字を差し込みに含めると、「{a}の{b}」のような文言が「の」を含むどの引用とも一致してしまう。
const placeholderExample = String.raw`(?:<[^<>]*>|[\x20-\x7E]*)`;

type Quote = { text: string; location: string };

function quotesInJapanesePages(): Quote[] {
  return japanesePages().flatMap((page) =>
    page.lines.flatMap((line, index) =>
      [...line.matchAll(quotePattern)].map(([, bracketed, quoted]) => ({
        text: bracketed ?? quoted ?? "",
        location: `${page.path}:${index + 1}`,
      })),
    ),
  );
}

function escapeForPattern(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function toPattern(label: string): RegExp {
  const source = label.split(placeholderPattern).map(escapeForPattern).join(placeholderExample);
  return new RegExp(`^${source}$`, "u");
}

// 差し込みを除いて文字が残らない文言（「{detail}」など）は、どんな引用とも一致してしまうので照合に使わない。
function hasWording(label: string): boolean {
  return /\p{L}/u.test(label.replace(placeholderPattern, ""));
}

function labelPatterns(labels: string[]): RegExp[] {
  const variants = labels.flatMap((label) => [label, label.replace(trailingNotePattern, "")]);
  return [...new Set(variants)].filter(hasWording).map(toPattern);
}

describe("pagesの［…］と「…」の引用", () => {
  it("今の画面の文言と一致する", () => {
    const patterns = labelPatterns([...Object.values(ja), ...androidJapaneseLabels()]);
    const quotes = quotesInJapanesePages();
    const stale = quotes
      .filter((quote) => !quotesOutsideTheCatalogue.has(quote.text))
      .filter((quote) => !patterns.some((pattern) => pattern.test(quote.text)))
      .map((quote) => `${quote.location} ${quote.text}`);

    expect(quotes.length).toBeGreaterThan(0);
    expect(stale).toEqual([]);
  });

  it("カタログの外にあるとした引用は、pagesで今も使っている", () => {
    const quoted = new Set(quotesInJapanesePages().map((quote) => quote.text));
    const unused = [...quotesOutsideTheCatalogue.keys()].filter((text) => !quoted.has(text));

    expect(unused).toEqual([]);
  });
});
