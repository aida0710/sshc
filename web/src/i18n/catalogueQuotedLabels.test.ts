import { describe, expect, it } from "vitest";
import { labelPatterns } from "../testing/catalogueLabels";
import { ja } from "./messages";

// 画面の文は、ほかの画面のボタンやタブの名前を［…］で引用する。画面を作り直して名前が変わると、
// 引用は自動では追従せず、もう無い場所を案内する文が残る（［sshc］タブ、［Basic］、#635）。

// ［…］で引用するが、sshcの画面の文言カタログにはないもの。足すときは理由も書く。
const namesOutsideTheCatalogue = new Map<string, string>([
  ["設定", "Androidの設定アプリの名前"],
  ["アプリ", "Androidの設定アプリの項目名"],
  ["sshc", "Androidの設定アプリに並ぶアプリの名前"],
  ["通知", "Androidの設定アプリの項目名"],
  ["プライバシーとセキュリティ", "macOSのシステム設定の項目の名前"],
  ["フルディスクアクセス", "macOSのシステム設定の項目の名前"],
]);

const quotedName = /［([^［］]+)］/g;

function namesQuotedOnScreen(): { name: string; key: string }[] {
  return Object.entries(ja).flatMap(([key, text]) =>
    [...text.matchAll(quotedName)].map(([, name = ""]) => ({ name, key })),
  );
}

describe("画面の文の［…］の引用", () => {
  it("今の画面の文言と一致する", () => {
    const patterns = labelPatterns(Object.values(ja));
    const quotes = namesQuotedOnScreen();
    const stale = quotes
      .filter(({ name }) => !namesOutsideTheCatalogue.has(name))
      .filter(({ name }) => !patterns.some((pattern) => pattern.test(name)))
      .map(({ name, key }) => `${key}: ［${name}］`);

    expect(quotes.length).toBeGreaterThan(0);
    expect(stale).toEqual([]);
  });

  it("カタログの外にあるとした名前は、今も引用している", () => {
    const quoted = new Set(namesQuotedOnScreen().map(({ name }) => name));
    const unused = [...namesOutsideTheCatalogue.keys()].filter((name) => !quoted.has(name));

    expect(unused).toEqual([]);
  });
});
