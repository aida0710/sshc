import type { TerminalSession } from "../api/integrations";
import type { Translate } from "../i18n/context";

export function connectionProgressText(t: Translate, session: TerminalSession): string {
  const progress = session.progress;
  if (progress === undefined) {
    return session.state === "connected" ? t("terminal.connected") : t("terminal.connecting");
  }
  const target = progress.alias || `${progress.user}@${progress.hostName}`;
  const position = `${progress.hop}/${progress.hops}`;
  switch (progress.phase) {
    case "dialing":
      return t("terminal.progressDialing", { target, position });
    case "host_key":
      return t("terminal.progressHostKey", { target, position });
    case "authenticating":
      return t("terminal.progressAuthenticating", { target, position });
    case "authenticated":
      return t("terminal.progressAuthenticated", { target, position });
    case "opening_session":
      return t("terminal.progressOpeningSession", { target, position });
  }
}
