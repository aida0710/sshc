import type { components } from "../api/schema";
import type { Translate } from "../i18n/context";

export type ConnectionProgress = components["schemas"]["TerminalConnectionProgress"];

// connectionStageText は、sshc エンジンが報告した接続の段階を1行の文にする。
// ターミナルのセッションと SFTP のタブが、接続を待つあいだに同じ言い方で出す。
export function connectionStageText(t: Translate, progress: ConnectionProgress): string {
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
