import { describe, expect, it } from "vitest";
import { applicationSourceFiles } from "../testing/applicationSources";

// 開閉の見出しは DisclosureSummary で書く（2026-11-25、同期の詳細・除外・設定と貼り付けの確認が、ブラウザの三角の
// 消し方・印の色・載せたときの印の色をそれぞれ手で書いていたのをそろえた）。この検査は、ほかの所で <summary> を
// 手で書いた行を見つける。外すものは、前の行に disclosure-exempt と理由を書く。
const exemption = "disclosure-exempt";
const sharedSummaryFile = "src/ui/DisclosureSummary.tsx";
// コメントの中で <summary> に触れた行は数えない。
const commentLine = /^\s*(\/\/|\*|\{\/\*)/;

function handWrittenSummaries(): string[] {
  return applicationSourceFiles()
    .filter((file) => file.name.endsWith(".tsx") && file.name !== sharedSummaryFile)
    .flatMap((file) => {
      const lines = file.text.split("\n");
      return lines.flatMap((line, index) =>
        /<summary\b/.test(line) && !commentLine.test(line) && !(lines[index - 1] ?? "").includes(exemption)
          ? [`${file.name}:${index + 1}`]
          : []);
    });
}

describe("headings that open and close a section", () => {
  it("use DisclosureSummary", () => {
    const offenders = handWrittenSummaries();
    expect(offenders, `build these with DisclosureSummary, or say why not:\n${offenders.join("\n")}`).toEqual([]);
  });
});
