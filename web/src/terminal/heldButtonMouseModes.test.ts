import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it } from "vitest";
import { attachHeldButtonMouseModes } from "./heldButtonMouseModes";

// Claude Code が出すマウスのモードの列。あとに届いたものが効くので、1003（any）になる。
const claudeCodeModes = ["\x1b[?1000h", "\x1b[?1002h", "\x1b[?1003h", "\x1b[?1006h"];

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

async function harness() {
  const terminal = new Terminal({ cols: 40, rows: 10, allowProposedApi: true });
  const container = document.createElement("div");
  document.body.append(container);
  const detach = attachHeldButtonMouseModes(terminal, container);
  cleanups.push(() => {
    detach();
    terminal.dispose();
    container.remove();
  });
  // write は、書いたものを xterm.js が読み終えるまで待ち、そのときのマウスのモードを返す。
  const write = async (text: string) => {
    await new Promise<void>((resolve) => terminal.write(text, resolve));
    return terminal.modes.mouseTrackingMode;
  };
  const press = () => container.dispatchEvent(new MouseEvent("mousedown", { buttons: 1, bubbles: true }));
  const release = () => document.dispatchEvent(new MouseEvent("mouseup", { buttons: 0, bubbles: true }));
  for (const mode of claudeCodeModes) await write(mode);
  return { write, press, release };
}

describe("mouse modes sent while a button is held", () => {
  it("keeps reporting every mouse event while a program sends its modes again during a drag", async () => {
    const { write, press, release } = await harness();
    press();
    const modesDuringTheResend: string[] = [];
    for (const mode of claudeCodeModes) modesDuringTheResend.push(await write(mode));
    expect(modesDuringTheResend).toEqual(["any", "any", "any", "any"]);
    release();
    expect(await write("")).toBe("any");
  });

  it("takes a mode that reports fewer events once the button is released", async () => {
    const { write, press, release } = await harness();
    press();
    expect(await write("\x1b[?1000h")).toBe("any");
    release();
    expect(await write("")).toBe("vt200");
  });

  it("takes a mode that reports fewer events at once while no button is held", async () => {
    const { write } = await harness();
    expect(await write("\x1b[?1000h")).toBe("vt200");
    expect(await write("\x1b[?1003h")).toBe("any");
  });

  it("stops the reports at once when the program turns them off during a drag, and leaves them off", async () => {
    const { write, press, release } = await harness();
    press();
    await write("\x1b[?1000h");
    expect(await write("\x1b[?1003l")).toBe("none");
    release();
    expect(await write("")).toBe("none");
  });

  it("reads a move with no button down as the release it did not see", async () => {
    const { write, press } = await harness();
    press();
    await write("\x1b[?1000h");
    document.dispatchEvent(new MouseEvent("mousemove", { buttons: 0, bubbles: true }));
    expect(await write("")).toBe("vt200");
  });

  it("does not hold a mode back after the release", async () => {
    const { write, press, release } = await harness();
    press();
    release();
    expect(await write("\x1b[?1002h")).toBe("drag");
  });
});
