import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it } from "vitest";
import leftoverModeReset from "../../../internal/terminal/testdata/leftover-mode-reset.json";
import { attachGraphemeClusterMode } from "./graphemeClusterMode";
import { applyCurrentUnicodeWidths } from "./unicodeWidths";

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

const on = "\u001b[?2027h";
const off = "\u001b[?2027l";
const question = "\u001b[?2027$p";

function harness() {
  const view = new Terminal({ cols: 20, rows: 2, allowProposedApi: true });
  applyCurrentUnicodeWidths(view);
  const state = { replaying: false };
  const replies: string[] = [];
  const mode = attachGraphemeClusterMode(view, { replaying: () => state.replaying, reply: (sequence) => replies.push(sequence) });
  cleanups.push(() => {
    mode.dispose();
    view.dispose();
  });
  const write = (data: string) => new Promise<void>((resolve) => view.write(data, resolve));
  // cellsOf is how far the cursor moves for the text, from the start of a fresh line.
  const cellsOf = async (text: string) => {
    await write(`\r\n${text}`);
    return view.buffer.active.cursorX;
  };
  return { view, state, replies, write, cellsOf };
}

// 👨‍👩‍👧、🏳️‍🌈、🇯🇵、👍🏽、❤️、1️⃣、☺︎、👁‍🗨（VS16 の無い ZWJ の並び）
const family = "\u{1F468}‍\u{1F469}‍\u{1F467}";
const rainbowFlag = "\u{1F3F3}️‍\u{1F308}";
const japaneseFlag = "\u{1F1EF}\u{1F1F5}";
const thumbsUpMediumSkin = "\u{1F44D}\u{1F3FD}";
const redHeart = "❤️";
const keycapOne = "1️⃣";
const smileText = "☺︎";
const eyeInSpeechBubble = "\u{1F441}‍\u{1F5E8}";

describe("mode 2027（書記素クラスタ）", () => {
  it("切っている間は wcwidth と同じに数え、絵文字の並びをばらばらのマスに置く", async () => {
    const { cellsOf } = harness();
    expect(await cellsOf(family)).toBe(6);
    expect(await cellsOf(rainbowFlag)).toBe(3);
    expect(await cellsOf(japaneseFlag)).toBe(2);
    expect(await cellsOf(thumbsUpMediumSkin)).toBe(4);
    expect(await cellsOf(redHeart)).toBe(1);
    expect(await cellsOf(keycapOne)).toBe(1);
    expect(await cellsOf(smileText)).toBe(1);
  });

  it("入れている間は1つの絵文字を1つのマスにまとめて2マスで数え、VS15 の付いた文字は1マスのまま", async () => {
    const { cellsOf, write } = harness();
    await write(on);
    expect(await cellsOf(family)).toBe(2);
    expect(await cellsOf(rainbowFlag)).toBe(2);
    expect(await cellsOf(japaneseFlag)).toBe(2);
    expect(await cellsOf(thumbsUpMediumSkin)).toBe(2);
    expect(await cellsOf(redHeart)).toBe(2);
    expect(await cellsOf(keycapOne)).toBe(2);
    expect(await cellsOf(eyeInSpeechBubble)).toBe(2);
    expect(await cellsOf(smileText)).toBe(1);
  });

  it("まとめた絵文字は1つのマスに入り、後ろの文字は続くマスに書かれる", async () => {
    const { view, write } = harness();
    await write(`${on}${family}${japaneseFlag}${japaneseFlag}X`);
    const line = view.buffer.active.getLine(0);
    expect(line?.getCell(0)?.getChars()).toBe(family);
    expect(line?.getCell(0)?.getWidth()).toBe(2);
    expect(line?.getCell(2)?.getChars()).toBe(japaneseFlag);
    expect(line?.getCell(4)?.getChars()).toBe(japaneseFlag);
    expect(line?.getCell(6)?.getChars()).toBe("X");
  });

  it("入れている間も、wcwidth で幅0の文字はカーソルを進めない", async () => {
    const { cellsOf, write } = harness();
    await write(on);
    // ゼロ幅スペース、前に付く文字（U+0600）、ハングルの字母、結合文字
    expect(await cellsOf("a​b")).toBe(2);
    expect(await cellsOf("a؀1")).toBe(2);
    expect(await cellsOf("각")).toBe(2);
    expect(await cellsOf("éx")).toBe(2);
    expect(await cellsOf("abc")).toBe(3);
  });

  it("DECRQM に入っているか切っているかを答え、再生中の出力の問い合わせには答えない", async () => {
    const { state, replies, write } = harness();
    await write(question);
    await write(on + question);
    await write(off + question);
    state.replaying = true;
    await write(question);
    expect(replies).toEqual(["\u001b[?2027;2$y", "\u001b[?2027;1$y", "\u001b[?2027;2$y"]);
  });

  it("DECSTR、RIS、プログラムが終わったあとのエンジンのリセットで切れる", async () => {
    const { cellsOf, write } = harness();
    for (const reset of ["\u001b[!p", "\u001bc", leftoverModeReset.sequence]) {
      await write(on);
      expect(await cellsOf(family)).toBe(2);
      await write(reset);
      expect(await cellsOf(family)).toBe(6);
    }
  });

  it("同じ列のほかのモードは xterm.js が受け持つ", async () => {
    const { view, cellsOf, write } = harness();
    await write("\u001b[?2004;2027h");
    expect(view.modes.bracketedPasteMode).toBe(true);
    expect(await cellsOf(family)).toBe(2);
    await write("\u001b[?2027;2004l");
    expect(view.modes.bracketedPasteMode).toBe(false);
    expect(await cellsOf(family)).toBe(6);
  });
});
