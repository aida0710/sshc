import { describe, expect, it } from "vitest";
import { applicationSourceFiles } from "../testing/applicationSources";

// 区画の上の端に出す帯は Banner に載せ、地は tone で選ぶ（2026-11-25、ターミナルの上の帯・画面の上のエラーの帯・
// 区画の知らせ・プレビューの失敗の余白・字の大きさ・ボタンの置き方が、それぞれ違っていたのをそろえた）。この検査は、
// 知らせの地（琥珀か薄い赤）に下の線を引いた帯を手で書いた行を見つける。外すものは、前の行に banner-exempt と
// 理由を書く。
const exemption = "banner-exempt";
const bannerFile = "ui/Banner.tsx";

const noticeGround = /\bbg-(notice|danger\/)/;
const bottomLine = /\bborder-b\b/;

function handBuiltBanners(): string[] {
  return applicationSourceFiles()
    .filter((file) => file.name.endsWith(".tsx") && !file.name.endsWith(bannerFile))
    .flatMap((file) => {
      const lines = file.text.split("\n");
      return lines.flatMap((line, index) =>
        noticeGround.test(line) && bottomLine.test(line) && !(lines[index - 1] ?? "").includes(exemption)
          ? [`${file.name}:${index + 1}`]
          : []);
    });
}

describe("bands across the top of an area", () => {
  it("use Banner", () => {
    const offenders = handBuiltBanners();
    expect(offenders, `put these bands in Banner (choose the ground with tone), or say why not:\n${offenders.join("\n")}`).toEqual([]);
  });
});
