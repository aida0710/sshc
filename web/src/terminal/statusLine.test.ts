import { describe, expect, it } from "vitest";
import leftoverModeReset from "../../../internal/terminal/testdata/leftover-mode-reset.json";
import { createStatusLineRouter } from "./statusLine";

const show = "\u001b[2$~";
const indicator = "\u001b[1$~";
const hide = "\u001b[0$~";
const toStatusLine = "\u001b[1$}";
const toScreen = "\u001b[0$}";

function harness() {
  const screen: string[] = [];
  const statusLine: string[] = [];
  const shown: boolean[] = [];
  const callbacks: string[] = [];
  const router = createStatusLineRouter({
    screen: { write: (text, callback) => { if (text !== "") screen.push(text); if (callback) { callbacks.push(screen.join("")); callback(); } } },
    statusLine: () => ({ write: (text) => statusLine.push(text) }),
    shownChanged: (value) => shown.push(value),
  });
  return {
    router,
    shown,
    callbacks,
    screen: () => screen.join(""),
    statusLine: () => statusLine.join(""),
  };
}

describe("the status line router", () => {
  it("sends what a program writes to the status line there, and the rest to the screen", () => {
    const { router, shown, screen, statusLine } = harness();
    router.write(`before${show}${toStatusLine}\u001b[1;1H\u001b[1mbuild 3/7\u001b[K${toScreen}after`);
    expect(shown).toEqual([true]);
    expect(statusLine()).toBe("\u001b[1;1H\u001b[1mbuild 3/7\u001b[K");
    expect(screen()).toBe("beforeafter");
  });

  it("finds the sequences however the output is cut into writes", () => {
    const output = `a${show}b${toStatusLine}status\u001b[31mline${toScreen}c`;
    for (let cut = 0; cut <= output.length; cut += 1) {
      const { router, screen, statusLine } = harness();
      router.write(output.slice(0, cut));
      router.write(output.slice(cut));
      expect([screen(), statusLine()], `cut at ${cut}`).toEqual(["abc", "status\u001b[31mline"]);
    }
  });

  it("leaves the screen's own sequences alone, and a question about the status line in a DCS string", () => {
    const { router, screen, statusLine } = harness();
    // DECRQSS asks for the active display with $} as its text.
    const own = `\u001b[2J\u001b[?2004h\u001b]0;build $} done\u0007\u001bP$q$}\u001b\\\u001b(B`;
    router.write(`${show}${own}`);
    expect(screen()).toBe(own);
    expect(statusLine()).toBe("");
  });

  it("keeps the output on the screen while no status line is shown, and for the indicator status line", () => {
    const { router, shown, screen, statusLine } = harness();
    router.write(`${toStatusLine}one${indicator}${toStatusLine}two`);
    expect(screen()).toBe("onetwo");
    expect(statusLine()).toBe("");
    expect(shown).toEqual([]);
  });

  it("returns the output to the screen when the status line is hidden", () => {
    const { router, shown, screen, statusLine } = harness();
    router.write(`${show}${toStatusLine}status${hide}screen`);
    expect(shown).toEqual([true, false]);
    expect(statusLine()).toBe("status");
    expect(screen()).toBe("screen");
  });

  it("leaves the status line on a soft reset and hides it on a full reset, handing the reset to the screen", () => {
    const { router, shown, screen } = harness();
    router.write(`${show}${toStatusLine}\u001b[!pafter-soft${toStatusLine}\u001bcafter-full`);
    expect(screen()).toBe("\u001b[!pafter-soft\u001bcafter-full");
    expect(shown).toEqual([true, false]);
  });

  it("returns to the screen and hides the status line with the reset the engine adds after a program ends", () => {
    const { router, shown, screen, statusLine } = harness();
    router.write(`${show}${toStatusLine}${leftoverModeReset.sequence}$ `);
    expect(statusLine()).toBe("");
    expect(screen().endsWith("$ ")).toBe(true);
    expect(shown).toEqual([true, false]);
  });

  it("returns to the screen where sshc marks lost output with CAN, even inside a sequence", () => {
    const { router, screen, statusLine } = harness();
    router.write(`${show}${toStatusLine}build 3/7\u0018$ ls`);
    router.write(`${toStatusLine}\u001b[1\u0018more`);
    expect(statusLine()).toBe("build 3/7\u0018\u001b[1\u0018");
    expect(screen()).toBe("\u0018$ ls\u0018more");
  });

  it("runs a write's callback once the screen has had everything before it", () => {
    const { router, callbacks } = harness();
    router.write(`one${show}${toStatusLine}two`, () => undefined);
    router.write("three", () => undefined);
    expect(callbacks).toEqual(["one", "one"]);
  });
});
