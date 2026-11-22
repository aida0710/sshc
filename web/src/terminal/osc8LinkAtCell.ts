import type { IBuffer } from "@xterm/xterm";
import { osc8Link, type TerminalLinkMatch } from "./links";

// xterm.js の中の、OSC 8 のリンクを持つ部分。xterm.js の API には、セルのリンクを読む口がない（リンクを渡すのは、
// マウスが乗ったときと押したときの linkHandler だけ）。
type InternalCell = { extended?: { urlId?: number } };
type InternalTerminal = { _core?: { _oscLinkService?: { getLinkData(linkId: number): { uri: string } | undefined } } };

type LinkedTerminal = { readonly cols: number; readonly buffer: { readonly active: IBuffer } };

// 1つのリンクの文字を読むために上下へたどる行の上限。プログラムが折り返した URL は数行で終わる。
const maxLinkRows = 32;

function linkIdAt(buffer: IBuffer, x: number, y: number): number {
  return (buffer.getLine(y)?.getCell(x) as InternalCell | undefined)?.extended?.urlId ?? 0;
}

// linkedTextOnRow は、y の行のうち linkId のリンクが付いたセルの文字を返す。その行にリンクが無ければ空。
function linkedTextOnRow(view: LinkedTerminal, y: number, linkId: number): string {
  const line = view.buffer.active.getLine(y);
  if (line === undefined) return "";
  let text = "";
  for (let x = 0; x < view.cols; x += 1) {
    if (linkIdAt(view.buffer.active, x, y) === linkId) text += line.getCell(x)?.getChars() ?? "";
  }
  return text;
}

// osc8LinkAtCell は、0 から数えたセルに付いている OSC 8 のリンクを返す。プログラムが宛先を渡しているので、
// リンクが何行に折り返されていても、画面の文字から URL をつなぎ直さずに宛先が分かる。text は画面に出ている
// リンクの文字で、同じリンクが続く上下の行の分もつなぐ。
export function osc8LinkAtCell(view: LinkedTerminal, cell: { x: number; y: number }): TerminalLinkMatch | null {
  const buffer = view.buffer.active;
  const linkId = linkIdAt(buffer, cell.x, cell.y);
  if (linkId === 0) return null;
  const target = (view as unknown as InternalTerminal)._core?._oscLinkService?.getLinkData(linkId)?.uri;
  if (target === undefined) return null;

  let start = cell.x;
  while (start > 0 && linkIdAt(buffer, start - 1, cell.y) === linkId) start -= 1;
  let end = cell.x + 1;
  while (end < view.cols && linkIdAt(buffer, end, cell.y) === linkId) end += 1;

  let text = linkedTextOnRow(view, cell.y, linkId);
  for (let y = cell.y - 1; y > cell.y - maxLinkRows; y -= 1) {
    const above = linkedTextOnRow(view, y, linkId);
    if (above === "") break;
    text = above + text;
  }
  for (let y = cell.y + 1; y < cell.y + maxLinkRows; y += 1) {
    const below = linkedTextOnRow(view, y, linkId);
    if (below === "") break;
    text += below;
  }
  return osc8Link(target, text, start, end);
}
