import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it, vi } from "vitest";
import { attachOsc52Clipboard } from "./osc52";
import { createTerminalOutput } from "./output";

const encoder = new TextEncoder();

function copyRequest(text: string): string {
  return `\u001b]52;c;${btoa(text)}\u0007`;
}

const cleanups: (() => void)[] = [];

function harness() {
  const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
  const output = createTerminalOutput(terminal);
  const writeText = vi.fn(async () => undefined);
  const copied = vi.fn();
  const imageSequences = vi.fn(() => true);
  const imageHandler = terminal.parser.registerOscHandler(1337, imageSequences);
  const detachClipboard = attachOsc52Clipboard({
    parser: terminal.parser,
    enabled: () => !output.isReplaying(),
    writeText,
    copied,
  });
  cleanups.push(() => {
    imageHandler.dispose();
    detachClipboard();
    output.dispose();
    terminal.dispose();
  });
  return {
    output,
    writeText,
    copied,
    imageSequences,
    async expectLine(text: string) {
      await vi.waitFor(() => expect(terminal.buffer.active.getLine(0)?.translateToString(true)).toBe(text));
    },
  };
}

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

describe("terminal output replay", () => {
  for (const [name, terminator] of [["BEL", "\u0007"], ["ST", "\u001b\\"]] as const) {
    for (const size of ["", ";size=1"]) {
      it(`preserves an image ${size ? "with" : "without"} size ending in ${name} across every byte boundary during replay and live output`, async () => {
        const subject = harness();
        const image = `File=inline=1${size}:YQ==`;
        const history = encoder.encode(`履歴\u001b]1337;${image}${terminator}${copyRequest("old text")}`);
        subject.output.beginReplay({ start: 0, next: history.byteLength, end: history.byteLength, truncated: false });
        const live = encoder.encode(`続き😀\u001b]1337;${image}${terminator}${copyRequest("new text")}`);

        // One byte at a time splits both UTF-8 characters and every OSC delimiter.
        for (const bytes of [history, live]) {
          for (const byte of bytes) subject.output.write(new Uint8Array([byte]));
        }
        await subject.expectLine("履歴続き😀");

        expect(subject.imageSequences.mock.calls).toEqual([[image], [image]]);
        expect(subject.writeText).toHaveBeenCalledTimes(1);
        expect(subject.writeText).toHaveBeenCalledWith("new text");
      });
    }
  }

  it("restores old output without copying and accepts the same text from a new live request", async () => {
    const subject = harness();
    const replay = encoder.encode(`history${copyRequest("copied text")}`);
    subject.output.beginReplay({ start: 0, next: replay.byteLength, end: replay.byteLength, truncated: false });

    // Queue live output before xterm has parsed the replay.
    subject.output.write(replay);
    subject.output.write(encoder.encode(`${copyRequest("copied text")}live`));
    await subject.expectLine("historylive");

    expect(subject.writeText).toHaveBeenCalledTimes(1);
    expect(subject.writeText).toHaveBeenCalledWith("copied text");
    expect(subject.copied).toHaveBeenCalledTimes(1);
  });

  it("keeps split UTF-8 and clipboard sequences intact across replay chunks and a live boundary", async () => {
    const subject = harness();
    const replay = encoder.encode(`履歴${copyRequest("old text")}`);
    const live = encoder.encode(`${copyRequest("new text")}続き`);
    const combined = new Uint8Array(replay.byteLength + live.byteLength);
    combined.set(replay);
    combined.set(live, replay.byteLength);
    const start = 100;
    const next = start + replay.byteLength;
    subject.output.beginReplay({ start, next, end: next, truncated: false });

    subject.output.write(combined.subarray(0, 1));
    subject.output.write(combined.subarray(1, replay.byteLength - 1));
    subject.output.write(combined.subarray(replay.byteLength - 1));
    await subject.expectLine("履歴続き");

    expect(subject.writeText).toHaveBeenCalledTimes(1);
    expect(subject.writeText).toHaveBeenCalledWith("new text");
  });

  it("accepts live clipboard requests immediately after an empty replay", async () => {
    const subject = harness();
    subject.output.beginReplay({ start: 50, next: 50, end: 50, truncated: false });

    subject.output.write(encoder.encode(`${copyRequest("live text")}live`));
    await subject.expectLine("live");

    expect(subject.writeText).toHaveBeenCalledWith("live text");
  });

  it("abandons an incomplete escape sequence when retained output is truncated and keeps the existing screen", async () => {
    const subject = harness();
    subject.output.write(encoder.encode(`before\u001b]52;c;${btoa("incomplete")}`));
    const replay = encoder.encode(`retained${copyRequest("old text")}`);
    const start = 100;
    const next = start + replay.byteLength;
    subject.output.beginReplay({ start, next, end: next, truncated: true });

    subject.output.write(replay);
    subject.output.write(encoder.encode(`${copyRequest("new text")}after`));
    await subject.expectLine("beforeretainedafter");

    expect(subject.writeText).toHaveBeenCalledTimes(1);
    expect(subject.writeText).toHaveBeenCalledWith("new text");
  });
});
