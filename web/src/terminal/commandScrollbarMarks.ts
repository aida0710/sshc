import type { IDecoration, Terminal } from "@xterm/xterm";
import type { CommandBlock, CommandBlocks, CommandStatus } from "./commandBlocks";

// The scrollbar shows where in the history each command is, coloured by how
// it ended, so a failure far back is easy to find. xterm.js draws a
// decoration's overview ruler mark in the scrollbar, as for search matches.

export type CommandScrollbarMarks = {
  // refresh marks the commands again, after the setting or the colours changed.
  refresh(): void;
  dispose(): void;
};

export type CommandMarkColours = Record<CommandStatus, string>;

export function attachCommandScrollbarMarks(view: Terminal, commands: CommandBlocks, { enabled, colours }: {
  enabled: () => boolean;
  colours: () => CommandMarkColours;
}): CommandScrollbarMarks {
  // Each mark keeps the status it was drawn with; a command that finished is marked again.
  const marks = new Map<CommandBlock, { decoration: IDecoration; status: CommandStatus }>();
  const clear = () => {
    for (const { decoration } of marks.values()) decoration.dispose();
    marks.clear();
  };
  const update = () => {
    if (!enabled()) {
      clear();
      return;
    }
    const palette = colours();
    for (const block of commands.blocks()) {
      const drawn = marks.get(block);
      if (drawn?.status === block.status && !drawn.decoration.isDisposed) continue;
      drawn?.decoration.dispose();
      const decoration = view.registerDecoration({ marker: block.start, overviewRulerOptions: { color: palette[block.status], position: "full" } });
      if (decoration === undefined) {
        marks.delete(block);
        continue;
      }
      marks.set(block, { decoration, status: block.status });
    }
    for (const [block, { decoration }] of marks) {
      if (block.start.isDisposed) {
        decoration.dispose();
        marks.delete(block);
      }
    }
  };
  const subscription = commands.onChange(update);
  update();
  return {
    refresh() {
      clear();
      update();
    },
    dispose() {
      subscription.dispose();
      clear();
    },
  };
}
