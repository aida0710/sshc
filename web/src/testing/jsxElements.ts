import { applicationSourceFiles } from "./applicationSources";

// JsxElement は、.tsx のソースの中の1つの JSX の要素である。attributes は開きタグ（<button から > の前まで）、
// body は閉じタグまでの中身、previousLine は開きタグの前の行（検査から外すわけを書く場所）。
export type JsxElement = { where: string; attributes: string; body: string; previousLine: string };

// tagEnd は、開きタグの終わりの > の位置。{} の中の => や > と、文字列の中の > は数えない。
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

// jsxElements は、アプリのソースから名前が tag の要素を探す。中身は最初の閉じタグまでとし、同じ名前の要素の
// 入れ子は考えない（ボタンの中にボタンは置かないため）。
export function jsxElements(tag: string): JsxElement[] {
  const opening = new RegExp(`<${tag}\\b`, "g");
  return applicationSourceFiles()
    .filter((file) => file.name.endsWith(".tsx"))
    .flatMap((file) =>
      [...file.text.matchAll(opening)].flatMap((match) => {
        const start = match.index;
        const end = tagEnd(file.text, start + tag.length + 1);
        if (end < 0) return [];
        const selfClosing = file.text[end - 1] === "/";
        const close = selfClosing ? end : file.text.indexOf(`</${tag}>`, end);
        if (close < 0) return [];
        const before = file.text.slice(0, start).split("\n");
        return [{
          where: `${file.name}:${before.length}`,
          attributes: file.text.slice(start, end),
          body: selfClosing ? "" : file.text.slice(end + 1, close),
          previousLine: before.length >= 2 ? before[before.length - 2]! : "",
        }];
      }),
    );
}
