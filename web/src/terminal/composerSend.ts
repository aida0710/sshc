import type { TerminalSession } from "../api/terminalSessions";
import { inspectTerminalPaste } from "./pasteGuard";
import type { StreamLink } from "./streamLink";

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

// pressesEnterOnly is whether sending this text is a press of Enter: nothing typed.
export function pressesEnterOnly(text: string): boolean {
  return text === "";
}

// bracketedPaste is whether the program in the terminal enabled bracketed
// paste. It then takes the text as one piece and a line break does not run
// anything, so lines typed in the box are sent without asking. Control
// characters are still asked about: one could end the bracketed paste early.
export function planComposerSend(text: string, { bracketedPaste }: { bracketedPaste: boolean }): ComposerSend {
  if (pressesEnterOnly(text)) return { kind: "enter" };
  const { risks } = inspectTerminalPaste(text);
  const asked = bracketedPaste ? risks.filter((risk) => risk !== "line-break") : risks;
  return asked.length > 0 ? { kind: "confirm", text } : { kind: "paste", text };
}

// ComposerSendBlock is why the text box cannot send now, and so what the person has to do; it keeps the text.
// The text is never sent by itself once connected: a reconnect opens another shell than the one it was written for.
export type ComposerSendBlock =
  // The engine or the browser's link is still trying to connect.
  | "wait-for-connection"
  // The connection ended and the person can open it again.
  | "reconnect-first"
  // There is nothing left to connect to.
  | "session-ended";

// composerSendBlock is why the text box cannot send to this session now, or null when it can. Before a session is
// connected the engine drops input, except an answer to an authentication prompt it waits on (Session.Write).
export function composerSendBlock(session: TerminalSession, link: StreamLink): ComposerSendBlock | null {
  if (session.state === "exited") return session.kind === "ssh" ? "reconnect-first" : "session-ended";
  if (link.phase === "stopped") return link.gone ? "session-ended" : "reconnect-first";
  if (link.phase !== "live") return "wait-for-connection";
  return session.state === "connected" || session.awaitingPrompt === true ? null : "wait-for-connection";
}
