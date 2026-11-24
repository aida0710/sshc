import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// 字が無く印だけのボタンは IconButton で描く（大きさ・角・字の色・載せたときの地を1か所で決めるため。2026-11-25、
// 場所ごとに別の作りになっていたのをそろえた）。この検査は、読み上げの名前（aria-label）を持ち、中身が印だけの
// <button> を見つける。形のわけがあるものは、<button の前の行に icon-button-exempt と理由を書いて外す。
const exemption = "icon-button-exempt";

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.tsx$/.test(path) && !/\.test\.tsx$/.test(path) && !path.endsWith("IconButton.tsx") ? [path] : [];
  });
}

// tagEnd は、開きタグの終わり（{} の中の => や > を数えない）。
function tagEnd(source: string, from: number): number {
  let depth = 0;
  let quote = "";
  for (let index = from; index < source.length; index += 1) {
    const character = source[index]!;
    if (quote !== "") {
      if (character === quote) quote = "";
    } else if (character === "\"" || character === "'" || character === "`") quote = character;
    else if (character === "{") depth += 1;
    else if (character === "}") depth -= 1;
    else if (character === ">" && depth === 0) return index;
  }
  return -1;
}

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
    const offenders: string[] = [];
    for (const path of sources(join(__dirname, ".."))) {
      const source = readFileSync(path, "utf8");
      for (const match of source.matchAll(/<button\b/g)) {
        const start = match.index;
        const end = tagEnd(source, start + 7);
        if (end < 0) continue;
        const attributes = source.slice(start, end);
        const body = source.slice(end + 1, source.indexOf("</button>", end));
        if (!attributes.includes("aria-label") || !iconOnly(body)) continue;
        const before = source.slice(0, start).split("\n");
        const previous = before.length >= 2 ? before[before.length - 2]! : "";
        if (previous.includes(exemption)) continue;
        offenders.push(`${path.slice(path.indexOf("/src/") + 1)}:${before.length}`);
      }
    }

    expect(offenders, `draw these with IconButton, or say why the shape differs:\n${offenders.join("\n")}`).toEqual([]);
  });
});
