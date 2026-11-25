import { describe, expect, it } from "vitest";
import { applicationSourceFiles } from "../testing/applicationSources";
import { sectionHeading } from "./form";

// 区画の中の小見出しは sectionHeading の見た目で書く（2026-11-25、23か所が同じ字の並びを手で書いていたのを
// sectionHeading にそろえた）。この検査は、見出し（h2〜h4 と開閉の見出し）に同じ字の並びを手で書いた行を
// 見つける。名前を出すだけの太字（鍵のパス、接続名）は見出しではないので数えない。外すものは、前の行に
// section-heading-exempt と理由を書く。
const exemption = "section-heading-exempt";
const headingElement = /<h[2-4]\b|<DisclosureSummary\b/;

function handWrittenSectionHeadings(): string[] {
  return applicationSourceFiles()
    .filter((file) => file.name.endsWith(".tsx"))
    .flatMap((file) => {
      const lines = file.text.split("\n");
      return lines.flatMap((line, index) =>
        headingElement.test(line) && line.includes(sectionHeading) && !(lines[index - 1] ?? "").includes(exemption)
          ? [`${file.name}:${index + 1}`]
          : []);
    });
}

describe("headings inside a section", () => {
  it("use sectionHeading", () => {
    const offenders = handWrittenSectionHeadings();
    expect(offenders, `use sectionHeading for these headings, or say why not:\n${offenders.join("\n")}`).toEqual([]);
  });
});
