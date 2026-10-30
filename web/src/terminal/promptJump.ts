import type { IBuffer, IMarker, IParser } from "@xterm/xterm";
import { matchesShortcut, shortcutsBlocked } from "../keyconfig/bindings";
import { currentBindings } from "../keyconfig/presets";

type JumpTerminal = {
  readonly parser: Pick<IParser, "registerOscHandler">;
  readonly buffer: { readonly active: IBuffer };
  registerMarker(cursorYOffset?: number): IMarker | undefined;
  scrollToLine(line: number): void;
  scrollToBottom(): void;
};

// The previous and next prompt, as Ghostty's jump_to_prompt: long output is
// passed over to where the shell drew its prompt (OSC 133;A, from shell
// integration), counted from the top of what is on the screen.
export type PromptJump = {
  // jump scrolls to the prompt before (-1) or after (1) the top of the view;
  // past the last prompt it goes to the bottom. It returns false when there is
  // no prompt to go by (no shell integration, or a full-screen program on the
  // alternate screen), so the keys can go to the program instead.
  jump(direction: -1 | 1): boolean;
  dispose(): void;
};

export function trackPromptJumps(view: JumpTerminal): PromptJump {
  // A marker follows its line as the buffer scrolls and is disposed once its
  // line leaves the scrollback; disposed ones are dropped as they are met.
  let prompts: IMarker[] = [];
  const watcher = view.parser.registerOscHandler(133, (data) => {
    const [kind] = data.split(";", 1);
    if (kind === "A") {
      const marker = view.registerMarker(0);
      if (marker !== undefined) prompts.push(marker);
    }
    // The command marks (osc133.ts) consume the sequence; this only watches it.
    return false;
  });

  return {
    jump(direction) {
      prompts = prompts.filter((marker) => !marker.isDisposed);
      if (prompts.length === 0 || view.buffer.active.type !== "normal") return false;
      const top = view.buffer.active.viewportY;
      // Prompts are marked in the order the shell drew them, so their lines rise.
      const target = direction < 0
        ? prompts.filter((marker) => marker.line < top).at(-1)
        : prompts.find((marker) => marker.line > top);
      if (target !== undefined) view.scrollToLine(target.line);
      else if (direction > 0) view.scrollToBottom();
      return true;
    },
    dispose() {
      watcher.dispose();
      for (const marker of prompts) marker.dispose();
      prompts = [];
    },
  };
}

// attachPromptJumpKeys moves between prompts on the previousPrompt and
// nextPrompt shortcuts pressed in the terminal.
export function attachPromptJumpKeys(container: HTMLElement, prompts: PromptJump): () => void {
  const pressed = (event: KeyboardEvent) => {
    const direction = matchesShortcut(event, "previousPrompt", currentBindings()) ? -1 : matchesShortcut(event, "nextPrompt", currentBindings()) ? 1 : 0;
    if (direction === 0 || shortcutsBlocked(event) || !prompts.jump(direction)) return;
    event.preventDefault();
    event.stopPropagation();
  };
  // Before xterm.js reads the key from its textarea, which is inside container.
  container.addEventListener("keydown", pressed, true);
  return () => container.removeEventListener("keydown", pressed, true);
}
