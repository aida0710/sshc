import { matchesShortcut, shortcutsBlocked, type ShortcutAction } from "../keyconfig/bindings";
import { currentBindings } from "../keyconfig/presets";

// attachTerminalShortcuts runs the shortcuts that act on one terminal when they are pressed in it. They are read
// before xterm.js takes the key from its textarea, which is inside container, so the program never gets them.
// It returns what detaches them.
export function attachTerminalShortcuts(
  container: HTMLElement,
  handlers: Partial<Record<ShortcutAction, () => void>>,
): () => void {
  const pressed = (event: KeyboardEvent) => {
    if (shortcutsBlocked(event)) return;
    const bindings = currentBindings();
    const action = (Object.keys(handlers) as ShortcutAction[]).find((name) => matchesShortcut(event, name, bindings));
    if (action === undefined) return;
    event.preventDefault();
    event.stopPropagation();
    if (!event.repeat || action.endsWith("FontSize")) handlers[action]?.();
  };
  container.addEventListener("keydown", pressed, true);
  return () => container.removeEventListener("keydown", pressed, true);
}
