import type { ReactNode } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { VPNProfileChip } from "../vpn/VPNProfileChip";
import { useSessionDestination } from "./sessionDestination";
import { exitedAbnormally, sessionStateText, sessionStatusTone } from "./sessionStatus";
import type { StreamLink } from "./streamLink";
import { terminalDisplayTitle } from "./terminalPresentation";

// TerminalHeader は、ターミナルの上の見出しである。広い画面では40pxの行、スマホではアプリの見出しに代わる
// 上のバーになる（ナビゲーションを開くボタン、名前と行き先、右側の操作）。右側の操作は children で受け取る。
export function TerminalHeader({ session, link, mobile, vpnProfile, runTarget, onOpenNavigation, children }: {
  session: TerminalSession;
  link: StreamLink;
  mobile: boolean;
  // vpnProfile は、この接続が通るVPN経路の名前である。空なら通らない。
  vpnProfile: string;
  // runTarget は、このセッションが動いているエンジンの SSH の別名である。このマシンで動いていれば undefined。
  runTarget: string | undefined;
  onOpenNavigation: (() => void) | undefined;
  children: ReactNode;
}) {
  const t = useTranslate();
  const displayTitle = terminalDisplayTitle(session);
  const destination = useSessionDestination(session);
  const connectionStatus = sessionStateText(t, session);
  // 見出しが状態を言葉で出すのは、接続中のあいだだけである。接続済みは点が言い、再接続中・終了・ブラウザの
  // リンクが落ちているときは見出しの下の行が1度だけ言う。言葉は、どの場合もスクリーンリーダー向けに残す。
  const saysState = session.state === "connecting" && link.phase === "live";
  return (
    <div className={`group/header relative flex shrink-0 items-center ${mobile ? "h-[54px] gap-1.5 pl-0.5 pr-1.5" : "h-10 gap-[9px] bg-term-bg pl-3.5 pr-2.5"}`}>
      {mobile && onOpenNavigation !== undefined ? (
        <button
          type="button"
          aria-label={t("shell.navigationToggle")}
          title={t("shell.navigationToggle")}
          onClick={onOpenNavigation}
          className="flex size-11 shrink-0 items-center justify-center rounded-lg text-ink-muted hover:bg-hover"
        >
          {/* ほかの画面の見出しのメニューのボタンと同じアイコンにする。‹ は「戻る」と読まれた。 */}
          <Icon name="menu" className="size-5" />
        </button>
      ) : null}
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-[9px]">
          <span
            aria-hidden="true"
            className={`size-2 shrink-0 rounded-full ${
              session.state === "exited"
                ? exitedAbnormally(session) ? "bg-danger" : "bg-ink-faint"
                : session.state === "reconnecting" || session.state === "connecting" || link.phase !== "live"
                  ? "bg-notice-ink"
                  : "bg-live"
            }`}
          />
          <p title={displayTitle} className={`min-w-0 truncate font-semibold text-ink ${mobile ? "text-base" : "max-w-48 shrink-0 text-ui"}`}>{displayTitle}</p>
          {mobile ? null : <span title={destination} className="min-w-0 truncate font-mono text-xs text-ink-faint">{destination}</span>}
          {/* スマホには言葉を置く幅が無いが、読み上げには残す。 */}
          <span role="status" className={!saysState || mobile ? "sr-only" : `shrink-0 text-xs ${sessionStatusTone(session)}`}>{connectionStatus}</span>
          {mobile ? null : <VPNProfileChip name={vpnProfile} />}
          {mobile || runTarget === undefined ? null : (
            <span className="shrink-0 rounded-full bg-accent/15 px-2 py-px font-mono text-2xs text-accent">{t("runTarget.runningOn", { alias: runTarget })}</span>
          )}
        </div>
        {mobile ? (
          <p className="mt-px flex min-w-0 items-center gap-1 pl-[17px] text-xs text-ink-faint">
            <span title={destination} className="truncate">{destination}</span>
            {saysState ? <span className={`shrink-0 ${sessionStatusTone(session)}`}>· {connectionStatus}</span> : null}
          </p>
        ) : null}
      </div>
      {children}
    </div>
  );
}
