import type { ReactNode } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { VPNProfileChip } from "../vpn/VPNProfileChip";
import { useSessionDestination } from "./sessionDestination";
import { exitedAbnormally, sessionStateText, sessionStatusTone } from "./sessionStatus";
import type { StreamLink } from "./streamLink";
import { terminalDisplayTitle } from "./terminalPresentation";
import { IconButton } from "../ui/IconButton";
import { Tag } from "../ui/Tag";
import type { PaneDragHandlers } from "./splitView/paneControls";

// detailSize は、見出しの名前の後ろに並べる行き先と札の大きさである。残りの幅に全文が入らなければ「…」で縮むが、
// 112px（basis-28）より狭くはしない。112px（それより短いものは全文の幅）も残らなければ、次の行へ折り返して隠れる。
// 112px より狭いと、VPN の札は「VPN経由：」の後ろのルートの名前がほとんど見えず、出しても役に立たない（行き先なら
// 約15字が見える幅）。並んだものは残りの幅を同じだけ分け合うので、次の札が並びに加わると、それまで広がっていた
// 行き先や札は 112px まで縮む。折り返すかどうかは 112px で決まるので、行き先を全文にしてから札を出す形にはできない。
const detailSize = "min-w-0 grow basis-28 max-w-max";

// detailRowHeight は、行き先と札の行の高さである。いちばん高い VPN の札（10px の字の行 15px、上下の余白 4px、枠 2px）に
// 合わせる。低いと札の下の枠が切れ、高いと折り返して隠したはずの次の行の上の端が見える。
const detailRowHeight = "h-[21px] before:h-[21px]";

// TerminalHeader は、ターミナルの上の見出しである。広い画面では40pxの行、スマホではアプリの見出しに代わる
// 上のバーになる（ナビゲーションを開くボタン、名前と行き先、右側の操作）。右側の操作は children で受け取る。
export function TerminalHeader({ session, link, mobile, vpnProfile, runTarget, onOpenNavigation, paneDrag, children }: {
  session: TerminalSession;
  link: StreamLink;
  mobile: boolean;
  // vpnProfile は、この接続が通るVPN経路の名前である。空なら通らない。
  vpnProfile: string;
  // runTarget は、このセッションが動いているエンジンの SSH の別名である。このマシンで動いていれば undefined。
  runTarget: string | undefined;
  onOpenNavigation: (() => void) | undefined;
  // paneDrag は、分割表示のペインを、見出しの名前の部分をつかんで動かす操作である。右のボタンからは動かせない。
  paneDrag: PaneDragHandlers | undefined;
  children: ReactNode;
}) {
  const t = useTranslate();
  const displayTitle = terminalDisplayTitle(session);
  const destination = useSessionDestination(session);
  const connectionStatus = sessionStateText(t, session);
  // 見出しが状態を言葉で出すのは、接続中のあいだだけである。接続済みは点が言い、再接続中・終了・ブラウザの
  // リンクが落ちているときは見出しの下の行が1度だけ言う。言葉は、どの場合もスクリーンリーダー向けに残す。
  const saysState = session.state === "connecting" && link.phase === "live";
  // スマホには言葉を置く幅が無いが、読み上げには残す。
  const stateText = <span role="status" className={!saysState || mobile ? "sr-only" : `shrink-0 text-xs ${sessionStatusTone(session)}`}>{connectionStatus}</span>;
  return (
    <div className={`relative flex shrink-0 items-center ${mobile ? "h-[54px] gap-1.5 pl-0.5 pr-1.5" : "h-10 gap-[9px] bg-term-bg pl-3.5 pr-2.5"}`}>
      {mobile && onOpenNavigation !== undefined ? (
        <IconButton label={t("shell.navigationToggle")} size="bar" onClick={onOpenNavigation}>
          {/* ほかの画面の見出しのメニューのボタンと同じアイコンにする。‹ は「戻る」と読まれた。 */}
          <Icon name="menu" className="size-5" />
        </IconButton>
      ) : null}
      <div className={`min-w-0 flex-1 ${paneDrag === undefined ? "" : "cursor-grab"}`}
        {...(paneDrag === undefined ? {} : { draggable: true, onDragStart: paneDrag.onDragStart, onDragEnd: paneDrag.onDragEnd })}>
        <div className="flex min-w-0 items-center">
          <span
            aria-hidden="true"
            className={`mr-[9px] size-2 shrink-0 rounded-full ${
              session.state === "exited"
                ? exitedAbnormally(session) ? "bg-danger" : "bg-ink-faint"
                : session.state === "reconnecting" || session.state === "connecting" || link.phase !== "live"
                  ? "bg-notice-ink"
                  : "bg-live"
            }`}
          />
          {/* 名前は最後に縮める。行き先と札は、名前が幅を取った残りにだけ並べる。 */}
          <p title={displayTitle} className={`min-w-0 truncate font-semibold text-ink ${mobile ? "text-base" : "text-ui"}`}>{displayTitle}</p>
          {mobile ? stateText : (
            // 行き先と札は1行に入る分だけ出し、入りきらないものは次の行へ折り返して、この行の高さの外で隠す。
            // 行の先頭のものは折り返せないので、幅の無い ::before を先頭に置き、行き先も札と同じく隠れるようにする。
            // 名前との間は ::before の後ろの間なので、名前が幅を使い切ったときは、この間も取らない。
            <div className={`flex min-w-0 flex-1 flex-wrap items-center gap-x-[9px] overflow-hidden before:content-[''] ${detailRowHeight}`}>
              <span title={destination} className={`truncate font-mono text-xs text-ink-faint ${detailSize}`}>{destination}</span>
              {stateText}
              <VPNProfileChip name={vpnProfile} className={detailSize} />
              {runTarget === undefined ? null : (
                <span className={`flex ${detailSize}`}><Tag tone="accent" truncate>{t("runTarget.runningOn", { alias: runTarget })}</Tag></span>
              )}
            </div>
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
