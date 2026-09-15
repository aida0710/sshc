import { describe, expect, it, vi } from "vitest";
import { attachOSC133Commands } from "./osc133";

describe("OSC 133 command lifecycle", () => {
  it("reports a completed command and its exit status", () => {
    let handler: (data: string) => boolean = () => false;
    const parser = {
      registerOscHandler: vi.fn((_identifier: number, next: (data: string) => boolean) => {
        handler = next;
        return { dispose: vi.fn() };
      }),
    };
    const started = vi.fn();
    const completed = vi.fn();
    let now = 1_000;
    attachOSC133Commands(parser, { onCommandStarted: started, onCommandCompleted: completed, now: () => now });

    expect(handler("A")).toBe(true);
    expect(handler("B")).toBe(true);
    expect(handler("C")).toBe(true);
    now = 34_250;
    expect(handler("D;7")).toBe(true);

    expect(started).toHaveBeenCalledOnce();
    expect(completed).toHaveBeenCalledWith({ durationMilliseconds: 33_250, exitCode: 7 });
  });

  it("ignores a completion that has no matching command start", () => {
    let handler: (data: string) => boolean = () => false;
    const completed = vi.fn();
    attachOSC133Commands({
      registerOscHandler: (_identifier, next) => {
        handler = next;
        return { dispose: vi.fn() };
      },
    }, { onCommandCompleted: completed });

    expect(handler("D;0")).toBe(true);
    expect(handler("unsupported")).toBe(false);
    expect(completed).not.toHaveBeenCalled();
  });
});
