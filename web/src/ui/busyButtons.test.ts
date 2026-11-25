import { describe, expect, it } from "vitest";
import { ja } from "../i18n/messages/ja";
import { jsxElements } from "../testing/jsxElements";

// 押したあとに待つボタンは、Button の busy で回る印を添え、文言は変えない（2026-11-25、「作成しています…」
// 「保存中…」に文言を替えるボタンと分かれていたのをそろえた）。この検査は、待っている間を言う文言をボタンの中に
// 出しているものを見つける。ボタンの名前でなく状態の行として出すもの（Home のカードの「接続中…」など）は、
// 要素の前の行に busy-label-exempt と理由を書いて外す。
const exemption = "busy-label-exempt";

// 待っている間を言う文言は「〜中…」「〜しています…」で終わる。「sshcエンジンを追加…」のように、押すと続きの
// 画面が開くことを言う「…」は入れない。
const waitingWording = /(中|ています)…$/;

function waitingKeys(body: string): string[] {
  return [...body.matchAll(/"([A-Za-z][\w.]*)"/g)]
    .map((match) => match[1]!)
    .filter((key) => key in ja && waitingWording.test(ja[key as keyof typeof ja]));
}

describe("buttons that wait for what they started", () => {
  it("finds the wording that says a button is waiting", () => {
    expect(waitingKeys('{editor.saving ? t("sftp.editorSaving") : t("sftp.save")}')).toEqual(["sftp.editorSaving"]);
    expect(waitingKeys('{t("sftp.save")}')).toEqual([]);
    expect(waitingKeys('<Icon name="plus" />{t("runTarget.add")}')).toEqual([]);
  });

  it("keep their label and show busy instead", () => {
    const offenders = ["Button", "button"]
      .flatMap((tag) => jsxElements(tag))
      .filter((button) => waitingKeys(button.body).length > 0 && !button.previousLine.includes(exemption))
      .map((button) => `${button.where} ${waitingKeys(button.body).join(", ")}`);

    expect(offenders, `give these busy and keep the label, or say why they differ:\n${offenders.join("\n")}`).toEqual([]);
  });
});
