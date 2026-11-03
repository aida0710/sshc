import { useCallback } from "react";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { useLoad } from "../ui/useLoad";
import { SessionNamesList } from "./SessionNamesList";

// EndingSessionsList names, inside a confirmation, the running sessions the
// action ends, so they can be checked before agreeing: a count does not say
// which ones they are. Sessions that already ended lose nothing and are left out.
export function EndingSessionsList({ sessions }: { sessions: TerminalSession[] }) {
  const t = useTranslate();
  const running = sessions.filter((session) => session.exited === undefined);
  if (running.length === 0) return null;
  return <SessionNamesList sessions={running} label={t("terminal.endingSessions")} />;
}

// EngineEndingSessionsList lists the running sessions of the engine behind api,
// read when the confirmation opens. While they load, or when they cannot be
// read (a run target whose Vault is locked), the confirmation's own words stand
// alone.
export function EngineEndingSessionsList({ api }: { api: Pick<TerminalSessionsApi, "terminalSessions"> }) {
  const listed = useLoad(useCallback(() => api.terminalSessions(), [api]));
  if (listed.state !== "loaded") return null;
  return <EndingSessionsList sessions={listed.value.sessions} />;
}
