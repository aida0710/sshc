import { describe, expect, it } from "vitest";
import { applicationSourceFiles } from "../testing/applicationSources";
import { jsxElements } from "../testing/jsxElements";

// ダイアログの最後のボタンの行は ActionArea に載せ、線を引くかは置き場所（inline・band）で決める（2026-11-25、線の
// 有無・寄せ方・余白がダイアログごとに違っていたのをそろえた）。この検査は、ダイアログを開くファイルの中で、ボタンを
// 右に寄せる行を手で書いたものを見つける。外すものは、要素の前の行に action-area-exempt と理由を書く。
const exemption = "action-area-exempt";

const dialogFiles = new Set(applicationSourceFiles().filter((file) => file.text.includes("<ModalShell")).map((file) => file.name));

describe("rows of buttons at the end of a dialog", () => {
  it("use ActionArea", () => {
    const offenders = ["div", "footer"]
      .flatMap((tag) => jsxElements(tag))
      .filter((row) => dialogFiles.has(row.where.slice(0, row.where.lastIndexOf(":"))))
      .filter((row) => /justify-end/.test(row.attributes) && /<Button\b/.test(row.body) && !row.previousLine.includes(exemption))
      .map((row) => row.where);

    expect(offenders, `put these rows in ActionArea (placement inline or band), or say why not:\n${offenders.join("\n")}`).toEqual([]);
  });
});
