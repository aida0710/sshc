import { useCallback } from "react";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { useLoad } from "../ui/useLoad";
import { sessionMarkerClass } from "./sessionStatus";
import { terminalDisplayTitle, terminalSubtitle } from "./terminalPresentation";

// EndingSessionsList names, inside a confirmation, the running sessions the
// action ends, so they can be checked before agreeing: a count does not say
// which ones they are. Sessions that already ended lose nothing and are left out.
export function EndingSessionsList({ sessions }: { sessions: TerminalSession[] }) {
  const t = useTranslate();
  const running = sessions.filter((session) => session.exited === undefined);
  if (running.length === 0) return null;
  return (
    // A long list scrolls, so it takes focus for the keyboard to scroll it.
    <ul aria-label={t("terminal.endingSessions")} tabIndex={0} className="max-h-40 overflow-y-auto rounded border border-line">
      {running.map((session) => (
        <li key={session.id} className="flex items-center gap-2 border-line px-2.5 py-1.5 text-sm [&+&]:border-t">
          <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${sessionMarkerClass(session, false)}`} />
          <span className="min-w-0 truncate text-ink">{terminalDisplayTitle(session)}</span>
          <span className="ml-auto shrink-0 text-xs text-ink-muted">{terminalSubtitle(session, t)}</span>
        </li>
      ))}
    </ul>
  );
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
