import type { IDisposable, Terminal } from "@xterm/xterm";
import { lastLineOf, type CommandBlock, type CommandBlocks } from "./commandBlocks";
import { terminalScreen } from "./metrics";

// The gutter left of the text marks each command with a line from its prompt
// to the end of its output, coloured by how it ended (index.css). Pointing at
// a line, or tapping it, asks for the command's details.

export type CommandGutter = {
  // refresh draws the marks again, after the setting changed.
  refresh(): void;
  dispose(): void;
};

type CommandGutterOptions = {
  enabled: () => boolean;
  // pointed names the command whose mark is pointed at or tapped, with the mark's place on the page.
  pointed: (block: CommandBlock, mark: DOMRect) => void;
  // left follows the pointer leaving a mark.
  left: () => void;
};

export function attachCommandGutter(view: Terminal, commands: CommandBlocks, options: CommandGutterOptions): CommandGutter {
  const screen = terminalScreen(view);
  const layer = document.createElement("div");
  layer.className = "sshc-command-gutter";
  screen?.appendChild(layer);
  const marks: HTMLElement[] = [];
  const shown = new Map<HTMLElement, CommandBlock>();

  const markAt = (index: number) => {
    let mark = marks[index];
    if (mark === undefined) {
      mark = document.createElement("div");
      mark.className = "sshc-command-gutter-mark";
      const point = () => {
        const block = shown.get(mark!);
        if (block !== undefined) options.pointed(block, mark!.getBoundingClientRect());
      };
      mark.addEventListener("pointerenter", point);
      mark.addEventListener("click", point);
      mark.addEventListener("pointerleave", options.left);
      layer.appendChild(mark);
      marks.push(mark);
    }
    return mark;
  };

  const draw = () => {
    shown.clear();
    const buffer = view.buffer.active;
    let used = 0;
    if (options.enabled() && buffer.type === "normal" && screen !== null && view.rows > 0) {
      const rowHeight = screen.getBoundingClientRect().height / view.rows;
      const top = buffer.viewportY;
      const bottom = top + view.rows - 1;
      for (const block of commands.blocks()) {
        const from = Math.max(block.start.line, top);
        const to = Math.min(lastLineOf(block, buffer), bottom);
        if (from > to) continue;
        const mark = markAt(used);
        used += 1;
        mark.dataset.status = block.status;
        mark.style.top = `${(from - top) * rowHeight}px`;
        mark.style.height = `${(to - from + 1) * rowHeight}px`;
        mark.hidden = false;
        shown.set(mark, block);
      }
    }
    for (let index = used; index < marks.length; index += 1) marks[index]!.hidden = true;
  };

  // Rendering, scrolling and resizing all move the lines; draw once per frame.
  let frame: number | null = null;
  const schedule = () => {
    if (frame !== null) return;
    frame = requestAnimationFrame(() => {
      frame = null;
      draw();
    });
  };
  const subscriptions: IDisposable[] = [view.onRender(schedule), view.onScroll(schedule), view.onResize(schedule), commands.onChange(schedule)];
  schedule();

  return {
    refresh: schedule,
    dispose() {
      if (frame !== null) cancelAnimationFrame(frame);
      for (const subscription of subscriptions) subscription.dispose();
      layer.remove();
    },
  };
}
