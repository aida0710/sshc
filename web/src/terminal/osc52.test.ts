import { describe, expect, it, vi } from "vitest";
import { attachOsc52Clipboard, decodeOsc52 } from "./osc52";

describe("OSC 52 clipboard writes", () => {
  it("decodes bounded UTF-8 clipboard payloads", () => {
    expect(decodeOsc52(`c;${btoa("hello")}`)).toBe("hello");
    expect(decodeOsc52("c;?")) .toBeNull();
    expect(decodeOsc52("p;aGVsbG8=")) .toBeNull();
    expect(decodeOsc52(`c;${btoa("too long")}`, 3)).toBeNull();
  });

  it("swallows requests but writes only after explicit enablement", async () => {
    let handler: ((data: string) => boolean) | undefined;
    let enabled = false;
    const writeText = vi.fn(async () => undefined);
    const dispose = vi.fn();
    const detach = attachOsc52Clipboard({
      parser: { registerOscHandler: vi.fn((_identifier, next) => { handler = next; return { dispose }; }) },
      enabled: () => enabled,
      writeText,
    });

    expect(handler?.(`c;${btoa("blocked")}`)).toBe(true);
    expect(writeText).not.toHaveBeenCalled();
    enabled = true;
    expect(handler?.(`c;${btoa("copied")}`)).toBe(true);
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith("copied"));
    detach();
    expect(dispose).toHaveBeenCalledOnce();
  });
});
