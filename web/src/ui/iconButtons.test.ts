import { describe, expect, it } from "vitest";
import { jsxElements } from "../testing/jsxElements";

// 字が無く印だけのボタンは IconButton で描く（大きさ・角・字の色・載せたときの地を1か所で決めるため。2026-11-25、
// 場所ごとに別の作りになっていたのをそろえた）。この検査は、読み上げの名前（aria-label）を持ち、中身が印だけの
// <button> を見つける。形のわけがあるものは、<button の前の行に icon-button-exempt と理由を書いて外す。
const exemption = "icon-button-exempt";

function iconOnly(body: string): boolean {
  if (!body.includes("<Icon")) return false;
  const rest = body
    .replace(/<Icon\b[^>]*\/>/g, "")
    .replace(/\{\/\*[\s\S]*?\*\/\}/g, "")
    .replace(/\{[^{}]*\?\s*\(?\s*\)?\s*:\s*\(?\s*\)?\s*\}/g, "")
    .replace(/\{\s*\}/g, "")
    .trim();
  return rest === "";
}

describe("buttons that show only an icon", () => {
  it("are drawn with IconButton", () => {
    const offenders = jsxElements("button")
      .filter((button) => button.attributes.includes("aria-label") && iconOnly(button.body) && !button.previousLine.includes(exemption))
      .map((button) => button.where);

    expect(offenders, `draw these with IconButton, or say why the shape differs:\n${offenders.join("\n")}`).toEqual([]);
  });
});
