import { describe, expect, it } from "vitest";
import { nextTerminalProgress, noTerminalProgress, type TerminalProgress } from "./osc9Progress";

describe("nextTerminalProgress", () => {
  it("進み具合を百分率で受け取る", () => {
    expect(nextTerminalProgress(noTerminalProgress, "4;1;42")).toEqual({ state: "normal", percent: 42 });
    expect(nextTerminalProgress(noTerminalProgress, "4;1;250")).toEqual({ state: "normal", percent: 100 });
  });

  it("0で消し、3で終わりの分からない進み具合にする", () => {
    const running: TerminalProgress = { state: "normal", percent: 30 };
    expect(nextTerminalProgress(running, "4;0")).toEqual(noTerminalProgress);
    expect(nextTerminalProgress(running, "4;3")).toEqual({ state: "indeterminate" });
  });

  it("エラーと一時停止は百分率が無ければ前の値を使う", () => {
    const running: TerminalProgress = { state: "normal", percent: 60 };
    expect(nextTerminalProgress(running, "4;2")).toEqual({ state: "error", percent: 60 });
    expect(nextTerminalProgress(running, "4;4;70")).toEqual({ state: "paused", percent: 70 });
    expect(nextTerminalProgress(noTerminalProgress, "4;2")).toEqual({ state: "error", percent: 0 });
  });

  it("進み具合ではないOSC 9は扱わない", () => {
    expect(nextTerminalProgress(noTerminalProgress, "build finished")).toBeNull();
    expect(nextTerminalProgress(noTerminalProgress, "9;C:\\Users")).toBeNull();
    expect(nextTerminalProgress(noTerminalProgress, "4;9;10")).toBeNull();
    expect(nextTerminalProgress(noTerminalProgress, "4;1")).toBeNull();
  });
});
