import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { commentLinesOf, languageOfPath } from "./commentLines.mjs";

// commentsOf は、行の番号ごとの、その行のコメントの断片を返す。
function commentsOf(source, language) {
  return Object.fromEntries([...commentLinesOf(source, language)].map(([number, { pieces }]) => [number, pieces]));
}

describe("commentLinesOf", () => {
  it("文字列・テンプレート・正規表現の中の // をコメントにしない", () => {
    const source = [
      'const address = "https://pelican.example/";',
      'const joined = `${base}//mirage ${"//"} ${`//`}`;',
      "const pattern = /https?:\\/\\/dunlin\\//g; // 正規表現の後",
      "const quoted = `a ${`b // c`} d`; /* テンプレートの後 */",
    ].join("\n");
    assert.deepEqual(commentsOf(source, "javascript"), { 3: ["// 正規表現の後"], 4: ["/* テンプレートの後 */"] });
  });

  it("テンプレートの ${ } の中に書いたコメントは読む", () => {
    const source = "const label = `${value /* 単位は秒 */}s`;";
    assert.deepEqual(commentsOf(source, "javascript"), { 1: ["/* 単位は秒 */"] });
  });

  it("割り算の / を正規表現の始まりと読まない", () => {
    const source = ["const half = (width) / 2; // 半分", "const ratio = total / count / 2; // 割合"].join("\n");
    assert.deepEqual(commentsOf(source, "javascript"), { 1: ["// 半分"], 2: ["// 割合"] });
  });

  it("JSX の地の文の URL と閉じるタグをコメントにしない", () => {
    const source = [
      "const link = <a href={docs}>https://r100.example/docs</a>;",
      "const row = <p><b>{name}</b> {/* 閉じるタグの後 */}</p>;",
    ].join("\n");
    assert.deepEqual(commentsOf(source, "javascript"), { 2: ["/* 閉じるタグの後 */"] });
  });

  it("閉じていない ' は行の終わりで打ち切り、次の行のコメントを読む", () => {
    const source = ["const text = <p>it's here</p>;", "// 次の行"].join("\n");
    assert.deepEqual(commentsOf(source, "javascript"), { 2: ["// 次の行"] });
  });

  it("ブロックコメントを行ごとに分け、コメントだけの行かを返す", () => {
    const source = ["/**", " * 一行目", " */ const port = 22; // 既定", "const a = 1;"].join("\n");
    const lines = commentLinesOf(source, "javascript");
    assert.deepEqual(
      [...lines].map(([number, { pieces, commentOnly }]) => [number, pieces, commentOnly]),
      [
        [1, ["/**"], true],
        [2, [" * 一行目"], true],
        [3, [" */", "// 既定"], false],
      ],
    );
  });

  it("コメントのある行の、コメントを除いた残りを返す。文字列の中の // は残りに入る", () => {
    const source = ['reason := "// 接続に失敗しました" // 理由', "/* 一行目", "二行目 */ port := 22"].join("\n");
    const lines = commentLinesOf(source, "go");
    assert.deepEqual(
      [...lines].map(([number, { code }]) => [number, code]),
      [
        [1, 'reason := "// 接続に失敗しました" '],
        [2, ""],
        [3, " port := 22"],
      ],
    );
  });

  it("Go の文字列・rune・raw string の中の // をコメントにしない", () => {
    const source = [
      'url := "https://pelican.example/" // 文字列の後',
      "quote := '\"' // rune の後",
      "usage := `sshc connect",
      "// raw string の中",
      "`",
    ].join("\n");
    assert.deepEqual(commentsOf(source, "go"), { 1: ["// 文字列の後"], 2: ["// rune の後"] });
  });

  it("Java のテキストブロックの中の // をコメントにしない", () => {
    const source = ['String page = """', "  // テキストブロックの中", '  """; // 後ろ'].join("\n");
    assert.deepEqual(commentsOf(source, "java"), { 3: ["// 後ろ"] });
  });

  it("Kotlin の入れ子のブロックコメントを外側の */ まで読み、${ } の中の文字列を読み飛ばす", () => {
    const source = ['val path = "${root("//")}/x" /* 外 /* 内 */ まだ外 */', "fun `doesn't crash`() {} // 名前の後"].join("\n");
    assert.deepEqual(commentsOf(source, "kotlin"), { 1: ["/* 外 /* 内 */ まだ外 */"], 2: ["// 名前の後"] });
  });

  it("CSS は /* */ だけをコメントにし、url() の // と文字列の中の /* を読み飛ばす", () => {
    const source = [
      ".banner { background: url(//pelican.example/banner.png); }",
      '.mark::before { content: "/* 文字列の中 */"; } /* 文字列の後 */',
      "/* 一行目",
      "   二行目 */",
    ].join("\n");
    assert.deepEqual(commentsOf(source, "css"), { 2: ["/* 文字列の後 */"], 3: ["/* 一行目"], 4: ["   二行目 */"] });
  });
});

describe("languageOfPath", () => {
  it("コメントを読める拡張子だけに言語を返す", () => {
    assert.equal(languageOfPath("internal/sftp/service.go"), "go");
    assert.equal(languageOfPath("web/src/App.tsx"), "javascript");
    assert.equal(languageOfPath("web/src/index.css"), "css");
    assert.equal(languageOfPath("scripts/ci/check-gofmt.sh"), undefined);
    assert.equal(languageOfPath("README.md"), undefined);
  });
});
