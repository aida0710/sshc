import { describe, expect, it } from "vitest";
import { jsxElements } from "../testing/jsxElements";
import { waitingKeys } from "../testing/waitingWording";

// 区画の一部や全体を読み込んでいる間の1行は、PanelState（layout line か block）で出す（2026-11-25、字の大きさ・色・
// 回る印の有無が場所ごとに違っていたのをそろえた）。この検査は、待っている間を言う文言を素の要素に直に書いたものを
// 見つける。ボタンは busyButtons.test.ts が見る。ほかの役のもの（画面の上に浮かぶ知らせ、接続中のパネル、札など）
// は、要素の前の行に loading-line-exempt と理由を書いて外す。
const exemption = "loading-line-exempt";

// withoutAttributes は、中に置いた部品の属性（busyLabel={...}、title={...} など）を除く。属性に渡した文言は、その部品が
// 出し方を決めるので、この要素の字ではない。
function withoutAttributes(body: string): string {
  let text = "";
  for (let index = 0; index < body.length; index += 1) {
    const attribute = /^[A-Za-z][\w-]*=\{/.exec(body.slice(index));
    if (attribute === null) {
      text += body[index];
      continue;
    }
    let depth = 0;
    for (index += attribute[0].length - 1; index < body.length; index += 1) {
      if (body[index] === "{") depth += 1;
      else if (body[index] === "}" && --depth === 0) break;
    }
  }
  return text;
}

describe("lines that say part of a screen is still coming", () => {
  it("use PanelState", () => {
    // 字を出すのは p と span。div は中の p や span を包むだけなので見ない。
    const offenders = ["p", "span"]
      .flatMap((tag) => jsxElements(tag))
      .map((element) => ({ element, keys: waitingKeys(withoutAttributes(element.body)) }))
      .filter(({ element, keys }) => keys.length > 0 && !element.previousLine.includes(exemption))
      .map(({ element, keys }) => `${element.where} ${keys.join(", ")}`);

    expect(offenders, `show these with PanelState, or say why they differ:\n${offenders.join("\n")}`).toEqual([]);
  });
});
