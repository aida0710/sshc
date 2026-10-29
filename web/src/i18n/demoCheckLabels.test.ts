import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { labelPatterns } from "../testing/catalogueLabels";
import { ja, type MessageKey } from "./messages";

// The browser demo's check (demo/scripts/check-browser.mjs) goes through the Japanese screens by the
// names of their buttons and dialogs. It runs only when a release is built, so a name changed on the
// screen stopped the release of v0.58.0 (the SFTP button 「ホスト」 had become 「接続先」). This finds
// such a name in the pull request instead.

// The names the check looks up on the sshc screen, with the wording each comes from. A common word is
// in the catalogue under other keys too (「ホスト」 still is), so a name is checked against its own key.
const screenNames = new Map<string, MessageKey>([
  ["左ペインのタブ", "sftp.primaryTabs"],
  ["接続先", "sftp.host"],
  ["接続先を選択", "sftp.chooseHostHeading"],
  ["welcome.txtの詳細", "sftp.detailsFor"],
  ["ファイルを編集", "sftp.editFile"],
  ["未保存", "sftp.unsaved"],
  ["保存", "sftp.save"],
  ["保存済み", "sftp.editorSaved"],
  ["接続済み", "terminal.connected"],
]);

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const read = (...path: string[]) => readFileSync(join(repositoryRoot, ...path), "utf8");
const check = read("demo", "scripts", "check-browser.mjs");
// The names on the demo page around the sshc screen: its wording and its language switch.
const demoPage = read("demo", "src", "messages-ja.js") + read("demo", "src", "language.js");

const japanese = /[\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Han}]/u;

// namesInTheCheck are the Japanese names the check looks things up by: role names, texts, and the
// whole-text patterns (/^…$/) it filters with.
function namesInTheCheck(): string[] {
  const quoted = [...check.matchAll(/(?:name:\s*|getByText\()"([^"]+)"/g)].map(([, name]) => name!);
  const patterns = [...check.matchAll(/hasText:\s*\/\^([^/$]+)\$\//g)].map(([, name]) => name!);
  return [...new Set([...quoted, ...patterns])].filter((name) => japanese.test(name));
}

function demoPageWording(): Set<string> {
  return new Set([...demoPage.matchAll(/"([^"]+)"/g)].map(([, text]) => text!));
}

describe("ブラウザデモの確認が探す日本語の名前", () => {
  it("sshcの画面のものは、その名前の文言のまま", () => {
    const changed = [...screenNames].filter(([name, key]) => !labelPatterns([ja[key]]).some((pattern) => pattern.test(name)));

    expect(changed.map(([name, key]) => `${name} → ${key}: ${ja[key]}`)).toEqual([]);
  });

  it("どれも、sshcの画面の名前としてここにあるか、デモのページの文言にある", () => {
    const demo = demoPageWording();
    const names = namesInTheCheck();
    const unknown = names.filter((name) => !screenNames.has(name) && !demo.has(name));

    expect(names.length).toBeGreaterThan(5);
    expect(unknown).toEqual([]);
  });

  it("ここに書いたsshcの画面の名前は、今も確認が使っている", () => {
    const names = new Set(namesInTheCheck());

    expect([...screenNames.keys()].filter((name) => !names.has(name))).toEqual([]);
  });
});
