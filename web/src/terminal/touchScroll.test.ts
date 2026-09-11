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

function flickHarness(options: { canScroll?: () => boolean; reducedMotion?: () => boolean } = {}) {
  const { view, sent } = recorder();
  let time = 0;
  let nextID = 0;
  const frames = new Map<number, FrameRequestCallback>();
  const scroll = newTouchScroll(view, () => 20, {
    ...options,
    now: () => time,
    requestFrame: (callback) => { frames.set(++nextID, callback); return nextID; },
    cancelFrame: (id) => { frames.delete(id); },
  });
  const advance = (milliseconds: number) => {
    time += milliseconds;
    const callbacks = [...frames.values()];
    frames.clear();
    for (const callback of callbacks) callback(time);
  };
  const flick = () => {
    scroll.start(200);
    advance(20);
    scroll.move(160);
    scroll.end();
  };
  return { scroll, sent, frames, advance, flick };
}

describe("touch scroll momentum", () => {
  it("continues a flick, slows down and eventually stops", () => {
    const { flick, advance, sent, frames } = flickHarness();
    flick();
    expect(sent).toEqual([2]);
    advance(16);
    expect(sent.length).toBeGreaterThan(1);
    for (let step = 0; step < 100; step++) advance(16);
    expect(frames.size).toBe(0);
    expect(sent.every((lines) => lines > 0)).toBe(true);
  });

  it("stops momentum immediately when a new finger lands", () => {
    const { flick, advance, sent, frames, scroll } = flickHarness();
    flick();
    scroll.start(100);
    advance(16);
    expect(frames.size).toBe(0);
    expect(sent).toEqual([2]);
  });

  it("cancels pending frames on disposal or a cancelled gesture", () => {
    const { flick, advance, sent, frames, scroll } = flickHarness();
    flick();
    scroll.cancel();
    expect(scroll.move(0)).toBe(0);
    scroll.end();
    advance(16);
    expect(frames.size).toBe(0);
    expect(sent).toEqual([2]);
  });

  it("does not move a selection established while coasting", () => {
    let canScroll = true;
    const { flick, advance, sent, frames } = flickHarness({ canScroll: () => canScroll });
    flick();
    canScroll = false;
    advance(16);
    expect(frames.size).toBe(0);
    expect(sent).toEqual([2]);
  });

  it("respects reduced motion and a finger held still before release", () => {
    const reduced = flickHarness({ reducedMotion: () => true });
    reduced.flick();
    expect(reduced.frames.size).toBe(0);

    const held = flickHarness();
    held.scroll.start(200);
    held.advance(20);
    held.scroll.move(160);
    held.advance(100);
    held.scroll.end();
    expect(held.frames.size).toBe(0);
  });

  it("does not jump after the page is backgrounded", () => {
    const { flick, advance, sent, frames } = flickHarness();
    flick();
    advance(5000);
    expect(frames.size).toBe(0);
    expect(sent).toEqual([2]);
  });
});
