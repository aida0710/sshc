import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it, vi } from "vitest";
import { attachKittyClipboard } from "./kittyClipboard";

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

const b64 = (text: string) => btoa(text);
const osc = (fields: string, payload?: string) => `\u001b]5522;${fields}${payload === undefined ? "" : `;${payload}`}\u001b\\`;

function harness({ allowed = true, replaying = false } = {}) {
  const view = new Terminal({ allowProposedApi: true });
  const written: Record<string, Blob>[] = [];
  const replies: string[] = [];
  const copied = vi.fn();
  const state = { allowed, replaying, failWith: null as Error | null };
  const clipboard = attachKittyClipboard({
    parser: view.parser,
    writesAllowed: () => state.allowed,
    replaying: () => state.replaying,
    writeClipboard: async (items) => {
      if (state.failWith !== null) throw state.failWith;
      written.push(items);
    },
    reply: (sequence) => replies.push(sequence),
    copied,
    randomBytes: (size) => new Uint8Array(size).fill(7),
  });
  cleanups.push(() => {
    clipboard.dispose();
    view.dispose();
  });
  const write = (data: string) => new Promise<void>((resolve) => view.write(data, resolve));
  return { view, clipboard, written, replies, copied, state, write };
}

describe("writes through the Kitty clipboard protocol", () => {
  it("puts text and an image on the clipboard where OSC 52 is allowed, and says it is done", async () => {
    const { written, replies, copied, write } = harness();

    await write(osc("type=write:id=w1"));
    await write(osc(`type=wdata:mime=${b64("text/plain")}`, b64("hello ")));
    await write(osc(`type=wdata:mime=${b64("text/plain")}`, b64("world")));
    await write(osc(`type=wdata:mime=${b64("image/png")}`, b64("\u0089PNG")));
    await write(osc(`type=walias:mime=${b64("text/plain")}`, b64("text/html")));
    await write(osc("type=wdata"));
    await vi.waitFor(() => expect(replies).toEqual([osc("type=write:status=DONE:id=w1")]));

    expect(Object.keys(written[0] ?? {}).sort()).toEqual(["image/png", "text/html", "text/plain"]);
    expect(await written[0]?.["text/plain"]?.text()).toBe("hello world");
    expect(copied).toHaveBeenCalledOnce();
  });

  it("refuses where OSC 52 is not allowed, for the primary selection, and for broken or unusable data", async () => {
    const { written, replies, state, write } = harness({ allowed: false });
    await write(osc("type=write") + osc(`type=wdata:mime=${b64("text/plain")}`, b64("x")) + osc("type=wdata"));
    state.allowed = true;
    await write(osc("type=write:loc=primary"));
    await write(osc("type=write") + osc(`type=wdata:mime=${b64("text/plain")}`, "not*base64") + osc("type=wdata"));
    await write(osc("type=write") + osc(`type=wdata:mime=${b64("image/jpeg")}`, b64("jpeg")) + osc("type=wdata"));

    expect(replies).toEqual([
      osc("type=write:status=EPERM"),
      osc("type=write:status=ENOSYS"),
      osc("type=write:status=EINVAL"),
      osc("type=write:status=EIO"),
    ]);
    expect(written).toEqual([]);
  });

  it("writes and answers nothing for history being replayed", async () => {
    const { written, replies, write } = harness({ replaying: true });
    await write(osc("type=write") + osc(`type=wdata:mime=${b64("text/plain")}`, b64("old")) + osc("type=wdata"));
    await write("\u001b[?5522$p");

    expect(written).toEqual([]);
    expect(replies).toEqual([]);
  });
});

describe("reads through the Kitty clipboard protocol", () => {
  it("refuses a read that does not come with the password of a paste", async () => {
    const { replies, write } = harness();
    await write(osc("type=read:id=r1", b64("text/plain")));

    expect(replies).toEqual([osc("type=read:status=EPERM:id=r1")]);
  });

  it("tells a program that asked for paste events what was pasted, and lets it read that once", async () => {
    const { clipboard, replies, write } = harness();
    expect(clipboard.takePaste(() => [{ mime: "text/plain", data: "x" }])).toBe(false);
    await write("\u001b[?5522h\u001b[?5522$p");
    expect(replies).toEqual(["\u001b[?5522;1$y"]);
    replies.length = 0;

    expect(clipboard.takePaste(() => [{ mime: "text/plain", data: "pasted" }, { mime: "image/png", data: new Blob(["png"]) }])).toBe(true);
    const password = btoa(String.fromCharCode(...new Uint8Array(18).fill(7)));
    expect(replies).toEqual([
      osc(`type=read:status=OK:pw=${password}`),
      osc(`type=read:status=DATA:mime=${b64(".")}:pw=${password}`, b64("text/plain image/png")),
      osc(`type=read:status=DONE:pw=${password}`),
    ]);
    replies.length = 0;

    const read = osc(`type=read:pw=${password}:name=${b64("Paste event")}`, b64("text/plain"));
    await write(read);
    await vi.waitFor(() => expect(replies).toHaveLength(3));
    expect(replies).toEqual([
      osc("type=read:status=OK"),
      osc(`type=read:status=DATA:mime=${b64("text/plain")}`, b64("pasted")),
      osc("type=read:status=DONE"),
    ]);
    replies.length = 0;
    await write(read);
    expect(replies).toEqual([osc("type=read:status=EPERM")]);
  });

  it("stops paste events when the program turns them off or the terminal is reset", async () => {
    const { clipboard, write } = harness();
    await write("\u001b[?5522h\u001b[?5522l");
    expect(clipboard.takePaste(() => [{ mime: "text/plain", data: "x" }])).toBe(false);
    await write("\u001b[?5522h\u001bc");
    expect(clipboard.takePaste(() => [{ mime: "text/plain", data: "x" }])).toBe(false);
  });
});
