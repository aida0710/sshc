import { describe, expect, it } from "vitest";
import { applicationSourceFiles } from "../testing/applicationSources";

// 補足の文（小さく淡い説明の段落）は hintText の見た目で書く（2026-11-25、18か所が同じ字の並びを手で書いていたのを
// hintText にそろえた）。この検査は、段落（<p>）に同じ字の並びを手で書いた行を見つける。日時や接続先のような値を
// 小さく出す span や段落は補足の文ではないので数えない。外すものは、前の行に hint-text-exempt と理由を書く。
const exemption = "hint-text-exempt";
// 余白（m*-、p*-）のほかに何も足していない段落だけを見る。等幅の値や、横に並べる段落は補足の文ではない。
const spacing = String.raw`(?:m[tbxy]?|p[tbxy]?)-[0-9.]+`;
const handWrittenHintParagraph = new RegExp(String.raw`<p className="(?:${spacing} )*text-xs text-ink-muted(?: ${spacing})*">`);

function handWrittenHints(): string[] {
  return applicationSourceFiles()
    .filter((file) => file.name.endsWith(".tsx"))
    .flatMap((file) => {
      const lines = file.text.split("\n");
      return lines.flatMap((line, index) =>
        handWrittenHintParagraph.test(line) && !(lines[index - 1] ?? "").includes(exemption)
          ? [`${file.name}:${index + 1}`]
          : []);
    });
}

describe("small explanatory paragraphs", () => {
  it("use hintText", () => {
    const offenders = handWrittenHints();
    expect(offenders, `use hintText for these paragraphs, or say why not:\n${offenders.join("\n")}`).toEqual([]);
  });
});
