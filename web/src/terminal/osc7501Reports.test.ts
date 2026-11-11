import { describe, expect, it, vi } from "vitest";
import { attachProgramStatusReports } from "./osc7501Reports";

type OscHandler = (data: string) => boolean | Promise<boolean>;

function parserWithHandlers() {
  const handlers = new Map<number, OscHandler>();
  return {
    handlers,
    parser: {
      registerOscHandler: (identifier: number, handler: OscHandler) => {
        handlers.set(identifier, handler);
        return { dispose: () => handlers.delete(identifier) };
      },
    },
  };
}

describe("attachProgramStatusReports", () => {
  it("プログラムが状態を知らせるたびに伝える", () => {
    const { parser, handlers } = parserWithHandlers();
    const reported = vi.fn();
    attachProgramStatusReports(parser, reported);

    void handlers.get(7501)?.("state=working");
    void handlers.get(7501)?.("state=done");

    expect(reported).toHaveBeenCalledTimes(2);
  });

  it("外したあとは伝えない", () => {
    const { parser, handlers } = parserWithHandlers();
    const reported = vi.fn();
    attachProgramStatusReports(parser, reported).dispose();

    expect(handlers.has(7501)).toBe(false);
    expect(reported).not.toHaveBeenCalled();
  });
});
