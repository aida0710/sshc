import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { connectionStageText } from "../shell/connectionStageText";

export function connectionProgressText(t: Translate, session: TerminalSession): string {
  const progress = session.progress;
  if (progress === undefined) {
    return session.state === "connected" ? t("terminal.connected") : t("terminal.connecting");
  }
  return connectionStageText(t, progress);
}
