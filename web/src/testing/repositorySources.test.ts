import { describe, expect, it } from "vitest";
import { codeOutsideCommentsIn, commentsIn } from "./repositorySources";

describe("コメントの外", () => {
  it("Goの文字列の語は拾い、同じ行のコメントの語は拾わない", () => {
    const source = ['reason := "接続先の端末に到達できません" // 資格情報', "port := 22"].join("\n");

    expect(codeOutsideCommentsIn("internal/r100/reason.go", source)).toEqual([
      { text: 'reason := "接続先の端末に到達できません" ', location: "internal/r100/reason.go:1" },
      { text: "port := 22", location: "internal/r100/reason.go:2" },
    ]);
  });

  it("raw stringの中の // は文字列として拾う", () => {
    const source = ["usage := `sshc connect", "// 握手の前に表示する", "`"].join("\n");

    expect(codeOutsideCommentsIn("cmd/mirage/usage.go", source).map((line) => line.text)).toEqual([
      "usage := `sshc connect",
      "// 握手の前に表示する",
      "`",
    ]);
  });
});

describe("コメント", () => {
  it("コメントの語は拾い、同じ行の文字列の語は拾わない", () => {
    const source = ['const label = "資格情報"; // 握手', "/**", " * 指紋", " */"].join("\n");

    expect(commentsIn("web/src/pelican.ts", source)).toEqual([
      { text: "// 握手", location: "web/src/pelican.ts:1" },
      { text: "/**", location: "web/src/pelican.ts:2" },
      { text: " * 指紋", location: "web/src/pelican.ts:3" },
      { text: " */", location: "web/src/pelican.ts:4" },
    ]);
  });

  it("コメントを読めない種類のファイルからは拾わない", () => {
    expect(commentsIn("docs/dunlin.md", "// 資格情報")).toEqual([]);
  });
});
