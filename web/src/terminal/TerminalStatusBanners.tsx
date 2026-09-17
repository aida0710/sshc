import { useState } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { terminalProblemKey } from "./sessions";
import type { StreamLink } from "./streamLink";

const notice = "shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink";

// The strip of one-line notices between the terminal header and the screen:
// the engine's reconnect progress, the browser's own stream link, and how
// the program ended. Each row only appears while it has something to say.
export function TerminalStatusBanners({ session, problem, link, onLinkNow, onLinkStop, onStopReconnect, onReconnect }: {
  session: TerminalSession;
  // A message from the view itself, such as a refused clipboard write.
  problem: string;
  link: StreamLink;
  onLinkNow: () => void;
  onLinkStop: () => void;
  onStopReconnect?: () => Promise<boolean>;
  // Resolves true once the engine reattached the SSH session; the stream is
  // reopened right away in that case.
  onReconnect?: () => Promise<boolean>;
}) {
  const t = useTranslate();
  const [reconnectBusy, setReconnectBusy] = useState(false);
  const [reconnectFailed, setReconnectFailed] = useState(false);
  const [stopBusy, setStopBusy] = useState(false);

  async function stopReconnecting() {
    if (onStopReconnect === undefined || stopBusy) return;
    setStopBusy(true);
    try {
      await onStopReconnect();
    } finally {
      setStopBusy(false);
    }
  }

  async function reconnectExitedSession() {
    if (onReconnect === undefined || reconnectBusy) return;
    setReconnectBusy(true);
    setReconnectFailed(false);
    try {
      if (await onReconnect()) {
        onLinkNow();
        return;
      }
      setReconnectFailed(true);
    } finally {
      setReconnectBusy(false);
    }
  }

  return (
    <>
      {problem === "" ? null : (
        <p role="status" className={notice}>
          {problem}
        </p>
      )}
      {reconnectFailed ? (
        <p role="status" className={notice}>
          {t("terminal.manualReconnectFailed")}
        </p>
      ) : null}

      {session.state !== "reconnecting" ? null : (
        <div role="status" className={`flex flex-wrap items-center gap-2 ${notice}`}>
          <p className="min-w-0 grow">
            {t("terminal.reconnectingAttempt", {
              attempt: String(session.reconnect?.attempt ?? 1),
              limit: String(session.reconnect?.limit ?? 1),
            })}
          </p>
          {onStopReconnect === undefined ? null : (
            <button
              type="button"
              disabled={stopBusy}
              onClick={() => void stopReconnecting()}
              className="min-h-8 shrink-0 rounded border border-notice-line px-3 py-1 font-medium text-notice-ink hover:bg-select-fill disabled:opacity-50"
            >
              {t("terminal.stopReconnect")}
            </button>
          )}
        </div>
      )}

      {session.problem === "" ? null : (
        <p role="alert" className={notice}>
          {t(terminalProblemKey(session.problem))}
        </p>
      )}

      {link.phase === "live" ? null : (
        <div role="status" className={`flex items-center gap-2 ${notice}`}>
          <p className="min-w-0 grow">
            {link.phase === "connecting"
              ? link.attempt === 1
                ? t("terminal.linkConnecting")
                : t("terminal.linkRetrying", { attempt: String(link.attempt) })
              : link.phase === "waiting"
                ? t("terminal.linkWaiting", { seconds: String(link.seconds), attempt: String(link.attempt) })
                : link.gone
                  ? t("terminal.linkGone")
                  : t("terminal.linkStopped")}
          </p>
          {link.phase === "stopped" && link.gone ? null : (
            <button
              type="button"
              disabled={link.phase === "connecting"}
              onClick={onLinkNow}
              className="shrink-0 rounded border border-notice-line px-2 py-0.5 text-notice-ink disabled:opacity-50"
            >
              {t("terminal.linkNow")}
            </button>
          )}
          {link.phase === "stopped" ? null : (
            <button
              type="button"
              onClick={onLinkStop}
              className="shrink-0 rounded border border-notice-line px-2 py-0.5 text-notice-ink"
            >
              {t("terminal.linkStop")}
            </button>
          )}
        </div>
      )}
      {session.exited === undefined ? null : (
        <div role="status" className="flex shrink-0 flex-wrap items-center gap-2 border-b border-line bg-card px-3 py-1.5 text-xs text-ink-muted">
          <p className="min-w-0 grow">
            {session.exited.signal === ""
              ? t("terminal.exitedWithCode", { code: String(session.exited.code) })
              : t("terminal.exitedWithSignal", { signal: session.exited.signal })}
          </p>
          {session.kind !== "ssh" || session.alias === undefined || onReconnect === undefined ? null : (
            <button
              type="button"
              disabled={reconnectBusy}
              onClick={() => void reconnectExitedSession()}
              className="min-h-8 shrink-0 rounded border border-control-line bg-control px-3 py-1 font-medium text-ink hover:bg-select-fill disabled:opacity-50"
            >
              {t(reconnectBusy ? "terminal.manualReconnecting" : "terminal.manualReconnect")}
            </button>
          )}
        </div>
      )}
    </>
  );
}
