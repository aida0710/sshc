import type { Terminal } from "@xterm/xterm";
import { attachOSC133Commands } from "./osc133";

// Marks each OSC 133 command start in the gutter and reports when a command
// ended.
export function attachCommandMarkers(view: Terminal, onCompleted: () => void): { dispose(): void } {
  const decorations = new Set<{ dispose(): void }>();
  const commands = attachOSC133Commands(view.parser, {
    onCommandStarted: () => {
      const marker = view.registerMarker();
      const decoration = view.registerDecoration({ marker, width: 1, layer: "top" });
      if (decoration === undefined) return;
      decorations.add(decoration);
      decoration.onRender((element) => element.classList.add("sshc-command-marker"));
      decoration.onDispose(() => decorations.delete(decoration));
    },
    onCommandCompleted: onCompleted,
  });
  return {
    dispose() {
      commands.dispose();
      for (const decoration of decorations) decoration.dispose();
    },
  };
}
