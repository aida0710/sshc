import { describe, expect, it } from "vitest";
import { newTouchScroll } from "./touchScroll";

function recorder() {
  const sent: number[] = [];
  return { view: { rows: 24, scrollLines: (amount: number) => sent.push(amount) }, sent };
}

describe("newTouchScroll", () => {
  // 上へ引けば内容は先へ進む。符号はホイールと同じ向きである。
  it("turns an upward drag into forward lines", () => {
    const { view, sent } = recorder();
    const scroll = newTouchScroll(view, () => 20);
    scroll.start(300);
    expect(scroll.move(240)).toBe(3);
    expect(sent).toEqual([3]);
  });

  it("turns a downward drag into backward lines", () => {
    const { view, sent } = recorder();
    const scroll = newTouchScroll(view, () => 20);
    scroll.start(100);
    expect(scroll.move(160)).toBe(-3);
    expect(sent).toEqual([-3]);
  });

  // **端数を捨てると、ゆっくり引いた指は何も起こさない。** 1 行に満たない
  // 動きが 20 回続いても、切り捨てていれば端末は一度も動かない。
  it("carries what does not yet make a line", () => {
    const { view, sent } = recorder();
    const scroll = newTouchScroll(view, () => 20);
    scroll.start(300);
    expect(scroll.move(295)).toBe(0);
    expect(scroll.move(290)).toBe(0);
    expect(scroll.move(285)).toBe(0);
    expect(sent).toEqual([]);
    // ここで合計 20px、ちょうど 1 行になる。
    expect(scroll.move(280)).toBe(1);
    expect(sent).toEqual([1]);
  });

  // 新しい指は前の指の端数を継がない。
  it("forgets the carry when a new drag begins", () => {
    const { view, sent } = recorder();
    const scroll = newTouchScroll(view, () => 20);
    scroll.start(300);
    scroll.move(285);
    scroll.start(300);
    expect(scroll.move(290)).toBe(0);
    expect(sent).toEqual([]);
  });

  // 高さを測れないうちは何もしない。**0 で割ると Infinity 行流れる。**
  it("does nothing before the rows have a height", () => {
    const { view, sent } = recorder();
    const scroll = newTouchScroll(view, () => 0);
    scroll.start(300);
    expect(scroll.move(100)).toBe(0);
    expect(sent).toEqual([]);
  });
});
