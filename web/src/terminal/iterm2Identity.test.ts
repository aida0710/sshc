import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it } from "vitest";

import { attachITerm2Identity, iTerm2XTVersionReply } from "./iterm2Identity";

const terminals: Terminal[] = [];

afterEach(() => {
  for (const terminal of terminals.splice(0)) terminal.dispose();
});

function harness(state: { enabled: boolean; replaying?: boolean }) {
  const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
  terminals.push(terminal);
  const replies: string[] = [];
  const builtInReplies: string[] = [];
  terminal.onData((data) => builtInReplies.push(data));
  attachITerm2Identity({
    parser: terminal.parser,
    enabled: () => state.enabled,
    isReplaying: () => state.replaying ?? false,
    reply: (text) => replies.push(text),
  });
  const write = (output: string) => new Promise<void>((resolve) => terminal.write(output, resolve));
  return { replies, builtInReplies, write };
}

describe("attachITerm2Identity", () => {
  it("answers XTVERSION as iTerm2 3.5 instead of xterm.js when enabled", async () => {
    const subject = harness({ enabled: true });
    await subject.write("\x1b[>0q");
    expect(subject.replies).toEqual([iTerm2XTVersionReply]);
    expect(subject.builtInReplies).toEqual([]);
  });

  it("answers the query written without its parameter", async () => {
    const subject = harness({ enabled: true });
    await subject.write("\x1b[>q");
    expect(subject.replies).toEqual([iTerm2XTVersionReply]);
  });

  it("leaves the answer to xterm.js when disabled", async () => {
    const subject = harness({ enabled: false });
    await subject.write("\x1b[>0q");
    expect(subject.replies).toEqual([]);
    expect(subject.builtInReplies.join("")).toMatch(/^\x1bP>\|xterm\.js\(/);
  });

  it("answers nothing to a query replayed from scrollback", async () => {
    const subject = harness({ enabled: true, replaying: true });
    await subject.write("\x1b[>0q");
    expect(subject.replies).toEqual([]);
    expect(subject.builtInReplies).toEqual([]);
  });
});
