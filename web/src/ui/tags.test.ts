import { describe, expect, it } from "vitest";
import { applicationSourceFiles } from "../testing/applicationSources";

// 名前の横の短い状態や数は、札（Tag、tagClass）で書く（2026-11-25、クイック接続の最後に接続した日、設定の
// 「このマシン」、プリセットの「使用中」、ターミナルの見出しの「〇〇で実行中」が、高さ・余白・字の大きさ・色を
// それぞれ手で書いていたのをそろえた）。この検査は、丸い地に字を置いた札を手で書いた行を見つける。外すものは、
// 前の行に tag-exempt と理由を書く。
const exemption = "tag-exempt";
const tagFile = "src/ui/Tag.tsx";
const roundGround = /\brounded-full\b.*\bbg-(?:accent\/15|select-fill|well|notice|live\/15)\b|\bbg-(?:accent\/15|select-fill|well|notice|live\/15)\b.*\brounded-full\b/;
const smallText = /\btext-(?:2xs|xs)\b/;
const sidePadding = /\bpx-/;

function handWrittenTags(): string[] {
  return applicationSourceFiles()
    .filter((file) => file.name.endsWith(".tsx") && file.name !== tagFile)
    .flatMap((file) => {
      const lines = file.text.split("\n");
      return lines.flatMap((line, index) =>
        roundGround.test(line) && smallText.test(line) && sidePadding.test(line) && !(lines[index - 1] ?? "").includes(exemption)
          ? [`${file.name}:${index + 1}`]
          : []);
    });
}

describe("short states and counts beside a name", () => {
  it("use Tag", () => {
    const offenders = handWrittenTags();
    expect(offenders, `build these with Tag or tagClass, or say why not:\n${offenders.join("\n")}`).toEqual([]);
  });
});
