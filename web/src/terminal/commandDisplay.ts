import type { Terminal } from "@xterm/xterm";
import type { CommandBlock, CommandBlocks } from "./commandBlocks";
import { watchCommandAtTop } from "./commandAtTop";
import { attachCommandGutter } from "./commandGutter";
import { attachCommandScrollbarMarks, type CommandMarkColours } from "./commandScrollbarMarks";

// What the terminal shows of the commands the shell ran, each part behind its
// own setting: the marks in the gutter, the marks on the scrollbar, and the
// command kept above its output (the view draws that one from topChanged).
export type CommandDisplay = {
  // refresh shows the parts again after a setting or the colours changed.
  refresh(): void;
  dispose(): void;
};

// The scrollbar's marks take the gutter's colours (index.css) from the terminal's container.
function markColours(container: HTMLElement): CommandMarkColours {
  const style = getComputedStyle(container);
  const colour = (name: string) => style.getPropertyValue(name).trim();
  return { running: colour("--ui-notice-ink"), failed: colour("--ui-danger"), succeeded: colour("--ui-ink-faint"), finished: colour("--ui-ink-faint") };
}

export function attachCommandDisplay(view: Terminal, commands: CommandBlocks, options: {
  container: HTMLElement;
  marks: () => boolean;
  scrollbarMarks: () => boolean;
  point: (block: CommandBlock, mark: DOMRect) => void;
  leave: () => void;
  topChanged: (block: CommandBlock | null) => void;
}): CommandDisplay {
  const gutter = attachCommandGutter(view, commands, { enabled: options.marks, pointed: options.point, left: options.leave });
  const scrollbar = attachCommandScrollbarMarks(view, commands, { enabled: options.scrollbarMarks, colours: () => markColours(options.container) });
  const top = watchCommandAtTop(view, commands, options.topChanged);
  return {
    refresh() {
      gutter.refresh();
      scrollbar.refresh();
    },
    dispose() {
      gutter.dispose();
      scrollbar.dispose();
      top.dispose();
    },
  };
}
