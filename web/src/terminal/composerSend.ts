import { inspectTerminalPaste } from "./pasteGuard";

// ComposerSend is what the phone's text box does with its text when it is sent.
// Text goes through the same path as a paste, so a program that enabled
// bracketed paste (Claude Code, Codex, most shells) gets it as one piece, and
// text a paste would have to confirm (line breaks, control characters) is
// confirmed the same way before anything is sent.
export type ComposerSend =
  // Nothing typed: press Enter, which answers a highlighted choice.
  | { kind: "enter" }
  // Send the text as a paste, then press Enter.
  | { kind: "paste"; text: string }
  // Ask first, as for a risky paste; send and press Enter only if accepted.
  | { kind: "confirm"; text: string };

// bracketedPaste is whether the program in the terminal enabled bracketed
// paste. It then takes the text as one piece and a line break does not run
// anything, so lines typed in the box are sent without asking. Control
// characters are still asked about: one could end the bracketed paste early.
export function planComposerSend(text: string, { bracketedPaste }: { bracketedPaste: boolean }): ComposerSend {
  if (text === "") return { kind: "enter" };
  const { risks } = inspectTerminalPaste(text);
  const asked = bracketedPaste ? risks.filter((risk) => risk !== "line-break") : risks;
  return asked.length > 0 ? { kind: "confirm", text } : { kind: "paste", text };
}
