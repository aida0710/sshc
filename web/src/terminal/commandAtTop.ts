import type { IDisposable, Terminal } from "@xterm/xterm";
import { blockAtTop, type CommandBlock, type CommandBlocks } from "./commandBlocks";

// watchCommandAtTop tells which command's output fills the top of the view
// while its prompt is scrolled above it, so the view can keep the command in
// sight. It reports only changes.
export function watchCommandAtTop(view: Terminal, commands: CommandBlocks, changed: (block: CommandBlock | null) => void): IDisposable {
  let current: CommandBlock | null = null;
  const check = () => {
    const buffer = view.buffer.active;
    const next = buffer.type === "normal" ? blockAtTop(commands.blocks(), buffer) : null;
    if (next === current) return;
    current = next;
    changed(next);
  };
  const subscriptions = [view.onScroll(check), view.onRender(check), commands.onChange(check)];
  return {
    dispose() {
      for (const subscription of subscriptions) subscription.dispose();
    },
  };
}
