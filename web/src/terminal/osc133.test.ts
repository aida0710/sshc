import { describe, expect, it, vi } from "vitest";
import { attachOSC133Commands } from "./osc133";

describe("OSC 133 command lifecycle", () => {
  it("reports where a command starts and that it ended", () => {
    let handler: (data: string) => boolean | Promise<boolean> = () => false;
    const parser = {
      registerOscHandler: vi.fn((_identifier: number, next: (data: string) => boolean) => {
        handler = next;
        return { dispose: vi.fn() };
      }),
    };
    const started = vi.fn();
    const completed = vi.fn();
    attachOSC133Commands(parser, { onCommandStarted: started, onCommandCompleted: completed });

    expect(handler("A")).toBe(true);
    expect(handler("B")).toBe(true);
    expect(handler("C")).toBe(true);
    expect(handler("D;7")).toBe(true);

    expect(started).toHaveBeenCalledOnce();
    expect(completed).toHaveBeenCalledOnce();
  });

  it("ignores a completion that has no matching command start", () => {
    let handler: (data: string) => boolean | Promise<boolean> = () => false;
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
