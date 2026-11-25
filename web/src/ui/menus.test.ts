import { describe, expect, it } from "vitest";
import { applicationSourceFiles } from "../testing/applicationSources";

// ボタンから開くメニューは、枠を MenuPanel、項目を MenuItems で書く（2026-11-25、セッションの行・ターミナルの
// 「…」・SFTP・転送の一覧のメニューが、置き方・幅・重なりの順・項目のボタンをそれぞれ手で書いていたのをそろえた）。
// この検査は、それ以外の所で role="menu" の枠や role="menuitem" の項目を手で書いた行を見つける。スマホの SFTP の
// 下から出るシートは形が違うので、決まった2つのファイルだけを外す。外すものは、前の行に menu-exempt と理由を書く。
const exemption = "menu-exempt";
const sharedMenuFiles = ["src/ui/MenuPanel.tsx", "src/ui/MenuItems.tsx"];
// スマホの SFTP のシート（SFTPActionSheet）と、その高い項目（SFTPMenuActionList の mobile）。
const phoneSheetFiles = ["src/sftp/SFTPActionSheet.tsx", "src/sftp/SFTPMenuActionList.tsx"];
const handWrittenMenuRole = /role=["{]?"?menu(item)?"/;

function handWrittenMenus(): string[] {
  const allowed = new Set([...sharedMenuFiles, ...phoneSheetFiles]);
  return applicationSourceFiles()
    .filter((file) => file.name.endsWith(".tsx") && !allowed.has(file.name))
    .flatMap((file) => {
      const lines = file.text.split("\n");
      return lines.flatMap((line, index) =>
        handWrittenMenuRole.test(line) && !(lines[index - 1] ?? "").includes(exemption)
          ? [`${file.name}:${index + 1}`]
          : []);
    });
}

describe("menus opened from a button", () => {
  it("use MenuPanel and MenuItems", () => {
    const offenders = handWrittenMenus();
    expect(offenders, `build these with MenuPanel and MenuItems, or say why not:\n${offenders.join("\n")}`).toEqual([]);
  });
});
