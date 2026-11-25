import { describe, expect, it } from "vitest";
import { ja } from "../i18n/messages/ja";
import { jsxElements } from "../testing/jsxElements";

// 押したあとに待つボタンは、Button の busy と busyLabel で、回る印と「〇〇中…」に替わる（2026-11-25、文言を
// 自分で替えるボタン、文言を残すボタン、薄くなるだけのボタンに分かれていたのをそろえた）。この検査は、そこから
// 外れるものを見つける。外すものは、要素の前の行に busy-label-exempt と理由を書く。
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
    expect(waitingKeys('{editor.saving ? t("busy.saving") : t("sftp.save")}')).toEqual(["busy.saving"]);
    expect(waitingKeys('{t("sftp.save")}')).toEqual([]);
    expect(waitingKeys('<Icon name="plus" />{t("runTarget.add")}')).toEqual([]);
  });

  it("say they are waiting through busyLabel, not by swapping their own label", () => {
    const offenders = ["Button", "button"]
      .flatMap((tag) => jsxElements(tag))
      .filter((button) => waitingKeys(button.body).length > 0 && !button.previousLine.includes(exemption))
      .map((button) => `${button.where} ${waitingKeys(button.body).join(", ")}`);

    expect(offenders, `give these busy and busyLabel, or say why they differ:\n${offenders.join("\n")}`).toEqual([]);
  });

  // 文言を替えず回る印だけにするのは、「〇〇中」の形にならない［やめる］［保存しない］［中止］だけと決めた
  // （2026-11-25）。ほかのボタンが黙って加わらないよう、1つずつ理由を書かせる。
  it("keep their label only where the reason is written next to them", () => {
    const offenders = jsxElements("Button")
      .filter((button) => button.attributes.includes("busyLabel={null}") && !button.previousLine.includes(exemption))
      .map((button) => button.where);

    expect(offenders, `give these a busyLabel, or say why they keep their label:\n${offenders.join("\n")}`).toEqual([]);
  });

  it("all say what they wait for in the same 〇〇中… form", () => {
    const busyWording = Object.entries(ja).filter(([key]) => key.startsWith("busy."));
    expect(busyWording.length).toBeGreaterThan(0);
    expect(busyWording.filter(([, wording]) => !/中…$/.test(wording))).toEqual([]);
  });
});
