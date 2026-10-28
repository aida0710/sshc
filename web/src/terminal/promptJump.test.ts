import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it } from "vitest";
import { attachPromptJumpKeys, trackPromptJumps } from "./promptJump";

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

// A shell with integration: each prompt is marked with OSC 133;A, and each command prints 20 lines.
async function shellWithPrompts(prompts: number) {
  const view = new Terminal({ cols: 40, rows: 10, scrollback: 1000, allowProposedApi: true });
  const jumps = trackPromptJumps(view);
  cleanups.push(() => {
    jumps.dispose();
    view.dispose();
  });
  const write = (data: string) => new Promise<void>((resolve) => view.write(data, resolve));
  const promptLines: number[] = [];
  for (let index = 0; index < prompts; index += 1) {
    promptLines.push(view.buffer.active.baseY + view.buffer.active.cursorY);
    await write(`\u001b]133;A\u0007$ run ${index}\r\n`);
    for (let line = 0; line < 20; line += 1) await write(`output ${index}.${line}\r\n`);
  }
  return { view, jumps, promptLines, write };
}

describe("moving between prompts", () => {
  it("goes up and down from prompt to prompt, and past the last one to the bottom", async () => {
    const { view, jumps, promptLines } = await shellWithPrompts(3);
    const top = () => view.buffer.active.viewportY;
    expect(top()).toBe(view.buffer.active.baseY);

    expect(jumps.jump(-1)).toBe(true);
    expect(top()).toBe(promptLines[2]);
    jumps.jump(-1);
    expect(top()).toBe(promptLines[1]);
    jumps.jump(-1);
    jumps.jump(-1);
    expect(top()).toBe(promptLines[0]);

    jumps.jump(1);
    expect(top()).toBe(promptLines[1]);
    jumps.jump(1);
    jumps.jump(1);
    expect(top()).toBe(view.buffer.active.baseY);
  });

  it("leaves the keys to the program without shell integration or on the alternate screen", async () => {
    const plain = new Terminal({ cols: 40, rows: 10, allowProposedApi: true });
    const plainJumps = trackPromptJumps(plain);
    cleanups.push(() => {
      plainJumps.dispose();
      plain.dispose();
    });
    expect(plainJumps.jump(-1)).toBe(false);

    const { jumps, write } = await shellWithPrompts(2);
    await write("\u001b[?1049h");
    expect(jumps.jump(-1)).toBe(false);
  });
});

describe("the prompt keys", () => {
  it("move on the shortcut and leave the key to the program when there is no prompt to go by", () => {
    const container = document.createElement("div");
    const textarea = container.appendChild(document.createElement("textarea"));
    const moves: number[] = [];
    let knowsPrompts = true;
    const detach = attachPromptJumpKeys(container, { jump: (direction) => { moves.push(direction); return knowsPrompts; }, dispose: () => undefined });
    cleanups.push(detach);
    const press = (init: KeyboardEventInit) => {
      const event = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init });
      textarea.dispatchEvent(event);
      return event.defaultPrevented;
    };

    expect(press({ key: "ArrowUp", ctrlKey: true, shiftKey: true })).toBe(true);
    expect(press({ key: "ArrowDown", metaKey: true })).toBe(true);
    expect(press({ key: "ArrowUp" })).toBe(false);
    knowsPrompts = false;
    expect(press({ key: "ArrowUp", ctrlKey: true, shiftKey: true })).toBe(false);

    expect(moves).toEqual([-1, 1, -1]);
  });
});
