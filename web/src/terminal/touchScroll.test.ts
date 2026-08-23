import { describe, expect, it } from "vitest";
import { newTouchScroll } from "./touchScroll";

function recorder() {
  const sent: number[] = [];
  return { view: { rows: 24, scrollLines: (amount: number) => sent.push(amount) }, sent };
}

describe("newTouchScroll", () => {
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

  it("carries what does not yet make a line", () => {
    const { view, sent } = recorder();
    const scroll = newTouchScroll(view, () => 20);
    scroll.start(300);
    expect(scroll.move(295)).toBe(0);
    expect(scroll.move(290)).toBe(0);
    expect(scroll.move(285)).toBe(0);
    expect(sent).toEqual([]);
    expect(scroll.move(280)).toBe(1);
    expect(sent).toEqual([1]);
  });

  it("forgets the carry when a new drag begins", () => {
    const { view, sent } = recorder();
    const scroll = newTouchScroll(view, () => 20);
    scroll.start(300);
    scroll.move(285);
    scroll.start(300);
    expect(scroll.move(290)).toBe(0);
    expect(sent).toEqual([]);
  });

  it("does nothing before the rows have a height", () => {
    const { view, sent } = recorder();
    const scroll = newTouchScroll(view, () => 0);
    scroll.start(300);
    expect(scroll.move(100)).toBe(0);
    expect(sent).toEqual([]);
  });
});
