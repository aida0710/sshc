import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { findEnglishComments } from "./englishComments.mjs";

// addedFile は、全部の行を足した新しいファイルである。
function addedFile(source, { path = "web/src/session.ts", language = "javascript" } = {}) {
  const lineCount = source.split("\n").length;
  return {
    path,
    language,
    oldSource: undefined,
    newSource: source,
    removedLines: [],
    addedLines: Array.from({ length: lineCount }, (_, index) => index + 1),
  };
}

// removedFile は、全部の行を消したファイルである。
function removedFile(source, { path = "web/src/old.ts", language = "javascript" } = {}) {
  const lineCount = source.split("\n").length;
  return {
    path,
    language,
    oldSource: source,
    newSource: undefined,
    removedLines: Array.from({ length: lineCount }, (_, index) => index + 1),
    addedLines: [],
  };
}

function flaggedLines(...changedFiles) {
  return findEnglishComments(changedFiles).map(({ path, line }) => `${path}:${line}`);
}

describe("findEnglishComments", () => {
  it("英語だけのコメントを足すと、ファイルと行を返す", () => {
    const source = ["// returns the session of the alias", "export function sessionOf() {}"].join("\n");
    assert.deepEqual(findEnglishComments([addedFile(source)]), [
      { path: "web/src/session.ts", line: 1, text: "returns the session of the alias" },
    ]);
  });

  it("ブロックコメントの行と、コードの後ろのコメントも見る", () => {
    const source = ["/**", " * Returns the session of the alias.", " */", "const port = 22; // the default port of ssh"].join("\n");
    assert.deepEqual(flaggedLines(addedFile(source)), ["web/src/session.ts:2", "web/src/session.ts:4"]);
  });

  it("Go のコメントも同じに見る", () => {
    const source = ["package session", "", "// Open opens the session of the alias.", "func Open() {}"].join("\n");
    assert.deepEqual(flaggedLines(addedFile(source, { path: "internal/session/open.go", language: "go" })), [
      "internal/session/open.go:3",
    ]);
  });

  it("かなか漢字を含む行と、英単語が3つ並ばない行は通す", () => {
    const passing = [
      "// alias の session を返す",
      "// 接続名 (alias) to the session",
      "// ssh -J mirage pelican",
      "// foo.bar() returns nil",
      "// known_hosts host key",
      "// TODO: 直す",
    ];
    for (const comment of passing) {
      assert.deepEqual(flaggedLines(addedFile(comment)), [], comment);
    }
  });

  it("足していない行の英語のコメントは見ない", () => {
    const changed = { ...addedFile(["// returns the session of the alias", "// 接続名を返す"].join("\n")), addedLines: [2] };
    assert.deepEqual(flaggedLines(changed), []);
  });

  it("同じ変更で消したコメントの行を移しただけの行は通す", () => {
    const moved = "// returns the session of the alias";
    const before = removedFile(["export function old() {", `  ${moved}`, "}"].join("\n"));
    const after = addedFile(["export class Sessions {", `    ${moved}`, "}"].join("\n"));
    assert.deepEqual(flaggedLines(before, after), []);
  });

  it("移してから書き換えた行は落とす", () => {
    const before = removedFile("// returns the session of the alias");
    const after = addedFile("// returns the session of the connection");
    assert.deepEqual(flaggedLines(before, after), ["web/src/session.ts:1"]);
  });

  it("ツールが読む指示の行は通す", () => {
    const directives = [
      "//go:build linux && amd64",
      "//nolint:errcheck // the close error is not interesting",
      "//lint:ignore SA1019 kept for the old engine",
      "// oxlint-disable-next-line react-hooks/exhaustive-deps",
      "// eslint-disable-next-line no-console",
      "// @ts-expect-error the type of the upstream package is wrong",
    ];
    for (const directive of directives) {
      assert.deepEqual(flaggedLines(addedFile(directive)), [], directive);
    }
  });

  it("URL だけの行と、Go の文書コメントのコードの例は通す", () => {
    const source = ["// https://pelican.example/spec/terminal-modes", "//", "//\tsshc connect pelican --port 22"].join("\n");
    assert.deepEqual(flaggedLines(addedFile(source)), []);
  });

  it("comment-language-exempt と日本語の理由を書いた行に続く、コメントだけの行は通す", () => {
    const source = [
      "// comment-language-exempt: RFC の文をそのまま引く。",
      "// The server MUST send the exit status",
      "// before it closes the channel.",
      "const closing = true;",
      "// The client then closes its side.",
    ].join("\n");
    assert.deepEqual(flaggedLines(addedFile(source)), ["web/src/session.ts:5"]);
  });

  it("理由の無い comment-language-exempt では通さない", () => {
    const withoutReason = ["// comment-language-exempt", "// The server MUST send the exit status"].join("\n");
    assert.deepEqual(flaggedLines(addedFile(withoutReason)), ["web/src/session.ts:2"]);
    const englishReason = ["// comment-language-exempt: quoting the RFC", "// The server MUST send the exit status"].join("\n");
    assert.deepEqual(flaggedLines(addedFile(englishReason)), ["web/src/session.ts:1", "web/src/session.ts:2"]);
  });
});
