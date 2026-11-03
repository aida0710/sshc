import type { ShortcutAction } from "../keyconfig/bindings";

// The shortcut actions that act on one terminal. The command palette runs them too, and it is not inside the
// terminal the way a key press is, so each terminal lists what it does under its session here.
export const terminalActionNames = [
  "terminalSearch", "copy", "paste", "previousPrompt", "nextPrompt",
  "increaseFontSize", "decreaseFontSize", "resetFontSize", "quickCommands",
] as const satisfies readonly ShortcutAction[];

export type TerminalAction = (typeof terminalActionNames)[number];

export type TerminalActions = Record<TerminalAction, () => void>;

const actionsBySession = new Map<string, TerminalActions>();

// registerTerminalActions lists what a terminal does for its session, and returns what takes it off the list.
export function registerTerminalActions(sessionId: string, actions: TerminalActions): () => void {
  actionsBySession.set(sessionId, actions);
  return () => {
    if (actionsBySession.get(sessionId) === actions) actionsBySession.delete(sessionId);
  };
}

export function runTerminalAction(sessionId: string, action: TerminalAction): void {
  actionsBySession.get(sessionId)?.[action]();
}
