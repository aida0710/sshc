import { describe, expect, it } from "vitest";
import { newTouchScroll } from "./touchScroll";

function recorder() {
  const sent: number[] = [];
  return { view: { scrollLines: (amount: number) => sent.push(amount) }, sent };
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
  let nextId = 0;
  const frames = new Map<number, FrameRequestCallback>();
  const scroll = newTouchScroll(view, () => 20, {
    ...options,
    now: () => time,
    requestFrame: (callback) => { frames.set(++nextId, callback); return nextId; },
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

  it("carries a fast flick far, as a page flung on a phone goes", () => {
    const { scroll, advance, sent } = flickHarness();
    scroll.start(400);
    advance(20);
    // 160 px in 20 ms: 8 px a millisecond, the fastest flick.
    scroll.move(240);
    scroll.end();
    for (let step = 0; step < 200; step++) advance(16);
    const rows = sent.reduce((total, lines) => total + lines, 0);
    // About 8 px/ms × 350 ms of coasting, in rows of 20 px, after the 8 rows the finger moved.
    expect(rows).toBeGreaterThan(100);
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

// smoothHarness is a scrollback of rows a finger moves by the pixel, with `above` older and `below` newer rows to go.
function smoothHarness({ above = 50, below = 50 }: { above?: number; below?: number } = {}) {
  const room = { above, below };
  const sent: number[] = [];
  const shown: number[] = [];
  let time = 0;
  let nextId = 0;
  const frames = new Map<number, FrameRequestCallback>();
  const scroll = newTouchScroll({
    scrollLines: (amount) => {
      sent.push(amount);
      room.above += amount;
      room.below -= amount;
    },
  }, () => 20, {
    now: () => time,
    requestFrame: (callback) => { frames.set(++nextId, callback); return nextId; },
    cancelFrame: (id) => { frames.delete(id); },
  });
  const smooth = { room: () => ({ ...room }), show: (fraction: number) => shown.push(fraction) };
  const advance = (milliseconds: number) => {
    time += milliseconds;
    const callbacks = [...frames.values()];
    frames.clear();
    for (const callback of callbacks) callback(time);
  };
  // hold moves the finger slowly enough that lifting it does not coast.
  const hold = (milliseconds: number) => { time += milliseconds; };
  return { scroll, smooth, sent, shown, frames, advance, hold };
}

describe("touch scroll by the pixel through the scrollback", () => {
  it("draws the rows moved by part of a row, and scrolls a row once the finger has moved one", () => {
    const { scroll, smooth, sent, shown } = smoothHarness();
    scroll.start(300, smooth);
    scroll.move(290);
    expect(sent).toEqual([]);
    expect(shown.at(-1)).toBe(0.5);
    scroll.move(270);
    expect(sent).toEqual([1]);
    expect(shown.at(-1)).toBeCloseTo(0.5);
  });

  it("moves back through older lines by drawing the row before moved down", () => {
    const { scroll, smooth, sent, shown } = smoothHarness();
    scroll.start(300, smooth);
    scroll.move(305);
    // A quarter of a row toward older lines is the row before, drawn three quarters of the way down.
    expect(sent).toEqual([-1]);
    expect(shown.at(-1)).toBeCloseTo(0.75);
  });

  it("draws nothing past the newest or the oldest row", () => {
    const newest = smoothHarness({ above: 50, below: 0 });
    newest.scroll.start(300, newest.smooth);
    newest.scroll.move(290);
    expect(newest.sent).toEqual([]);
    expect(newest.shown.at(-1)).toBe(0);

    const oldest = smoothHarness({ above: 0, below: 50 });
    oldest.scroll.start(300, oldest.smooth);
    oldest.scroll.move(310);
    expect(oldest.sent).toEqual([]);
    expect(oldest.shown.at(-1)).toBe(0);
  });

  it("settles on the nearer row once the finger lets go, slowing down on the way", () => {
    const { scroll, smooth, sent, shown, advance, hold } = smoothHarness();
    scroll.start(300, smooth);
    scroll.move(288);
    hold(200);
    scroll.end();
    advance(40);
    const midway = shown.at(-1)!;
    expect(midway).toBeGreaterThan(0.6);
    expect(midway).toBeLessThan(1);
    advance(200);
    expect(sent).toEqual([1]);
    expect(shown.at(-1)).toBe(0);

    const back = smoothHarness();
    back.scroll.start(300, back.smooth);
    back.scroll.move(294);
    back.hold(200);
    back.scroll.end();
    back.advance(200);
    expect(back.sent).toEqual([]);
    expect(back.shown.at(-1)).toBe(0);
  });

  it("lets a finger that lands while the rows settle take them where they are drawn", () => {
    const { scroll, smooth, shown, advance, hold } = smoothHarness();
    scroll.start(300, smooth);
    scroll.move(290);
    hold(200);
    scroll.end();
    advance(30);
    const drawn = shown.at(-1)!;
    scroll.start(200, smooth);
    expect(shown.at(-1)).toBe(drawn);
    scroll.move(196);
    expect(shown.at(-1)).toBeCloseTo(drawn + 0.2);
  });

  it("puts the rows back on a row when the gesture is cancelled", () => {
    const { scroll, smooth, shown } = smoothHarness();
    scroll.start(300, smooth);
    scroll.move(290);
    scroll.cancel();
    expect(shown.at(-1)).toBe(0);
  });
});
