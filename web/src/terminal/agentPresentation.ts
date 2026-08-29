import type { TerminalSession } from "../api/integrations";
import type { Translate } from "../i18n/context";

export function agentName(kind: NonNullable<TerminalSession["agent"]>["kind"]): string {
  switch (kind) {
    case "claude": return "Claude Code";
    case "codex": return "Codex";
    case "opencode": return "OpenCode";
  }
}

export function agentStateText(t: Translate, state: NonNullable<TerminalSession["agent"]>["state"]): string {
  switch (state) {
    case "working": return t("terminal.agentWorking");
    case "attention": return t("terminal.agentAttention");
    case "ready": return t("terminal.agentReady");
    case "unknown": return t("terminal.agentUnknown");
  }
}

export function terminalDisplayTitle(session: TerminalSession): string {
  return session.presentation?.displayTitle ?? session.title;
}

export function terminalSubtitle(session: TerminalSession): string {
  if (session.kind === "ssh") return session.alias ?? session.title;
  return "localhost";
}

export function agentStatusLabel(t: Translate, session: TerminalSession): string {
  if (session.agent === undefined) return "";
  return `${agentName(session.agent.kind)} · ${agentStateText(t, session.agent.state)}`;
}
