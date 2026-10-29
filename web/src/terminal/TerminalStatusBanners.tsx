import { useEffect, useState } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { terminalProblemKey } from "./sessions";
import { endedRightAway, exitedAbnormally } from "./sessionStatus";
import type { StreamLink } from "./streamLink";
import { Button } from "../ui/surface";

// 帯は、ターミナルの上に1行ずつ重なる。共通の Notice のように操作を文の下に置くと、帯ごとに1行増えて
// ターミナルの行が減るので、操作は文の右に並べ、ボタンは知らせの中に置く低いボタン（Button small）にする。
const notice = "shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink";
// スマホの組み方では、指で押せるように帯のボタンを少し高くする。
const bannerButton = "shrink-0 mobile-layout:min-h-8";

function StreamLinkMessage({ link }: { link: Exclude<StreamLink, { phase: "live" }> }) {
  const t = useTranslate();
  if (link.phase === "connecting") {
    return link.attempt === 1 ? t("terminal.linkConnecting") : t("terminal.linkRetrying", { attempt: String(link.attempt) });
  }
  if (link.phase === "stopped") return link.gone ? t("terminal.linkGone") : t("terminal.linkStopped");
  // 秒読みは毎秒変わるので読み上げず、試行の回数が変わったときだけ読み上げる。
  return (
    <>
      <span aria-hidden="true">
        {t("terminal.linkWaiting", { seconds: String(link.seconds), attempt: String(link.attempt) })}
      </span>
      <span className="sr-only">{t("terminal.linkWaitingAnnouncement", { attempt: String(link.attempt) })}</span>
    </>
  );
}

// The countdown to the next reconnect attempt is redrawn this often.
const reconnectCountdownTickMs = 1_000;

// ReconnectMessage says which attempt the engine is on and whether it is
// waiting for the next one (with the seconds left) or connecting now. A
// connection attempt can take until the SSH timeout, so "waiting" and
// "connecting" are told apart rather than shown as one count.
function ReconnectMessage({ reconnect }: { reconnect: TerminalSession["reconnect"] }) {
  const t = useTranslate();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), reconnectCountdownTickMs);
    return () => window.clearInterval(timer);
  }, []);
  const attempt = String(reconnect?.attempt ?? 1);
  const limit = String(reconnect?.limit ?? 1);
  const retryAt = reconnect === undefined ? Number.NaN : Date.parse(reconnect.retryAt);
  const seconds = Math.ceil((retryAt - now) / 1_000);
  if (!Number.isFinite(seconds) || seconds <= 0) return t("terminal.reconnectingNow", { attempt, limit });
  // 秒読みは毎秒変わるので読み上げず、試行の回数が変わったときだけ読み上げる。
  return (
    <>
      <span aria-hidden="true">{t("terminal.reconnectingIn", { attempt, limit, seconds: String(seconds) })}</span>
      <span className="sr-only">{t("terminal.reconnectingAttempt", { attempt, limit })}</span>
    </>
  );
}

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
            <ReconnectMessage reconnect={session.reconnect} />
          </p>
          {onStopReconnect === undefined ? null : (
            <Button small disabled={stopBusy} onClick={() => void stopReconnecting()} className={bannerButton}>
              {t("terminal.stopReconnect")}
            </Button>
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
            <StreamLinkMessage link={link} />
          </p>
          {link.phase === "stopped" && link.gone ? null : (
            <Button small disabled={link.phase === "connecting"} onClick={onLinkNow} className={bannerButton}>
              {t("terminal.linkNow")}
            </Button>
          )}
          {link.phase === "stopped" ? null : (
            <Button small onClick={onLinkStop} className={bannerButton}>
              {t("terminal.linkStop")}
            </Button>
          )}
        </div>
      )}
      {session.exited === undefined ? null : (
        <div role="status" className="flex shrink-0 flex-wrap items-center gap-2 border-b border-line bg-card px-3 py-1.5 text-xs">
          <p className={`min-w-0 grow ${exitedAbnormally(session) ? "text-danger" : "text-ink-muted"}`}>
            {session.exited.connectionLost === true
              ? t("terminal.exitedWithConnectionLost")
              : session.exited.signal !== ""
                ? t("terminal.exitedWithSignal", { signal: session.exited.signal })
                : session.exited.code === 0 && endedRightAway(session)
                  ? t("terminal.exitedWithCodeRightAway")
                  : t("terminal.exitedWithCode", { code: String(session.exited.code) })}
          </p>
          {session.kind !== "ssh" || session.alias === undefined || onReconnect === undefined ? null : (
            <Button small disabled={reconnectBusy} onClick={() => void reconnectExitedSession()} className={bannerButton}>
              {t(reconnectBusy ? "terminal.manualReconnecting" : "terminal.manualReconnect")}
            </Button>
          )}
        </div>
      )}
    </>
  );
}
