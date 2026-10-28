import { useContext } from "react";
import { createPortal } from "react-dom";
import { useTranslate } from "../i18n/context";
import { PageChromeContext } from "../ui/pageChrome";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { Button } from "../ui/surface";
import { OperatingSystemIcon } from "../ui/OperatingSystemIcon";
import { summarizeConnection, type ConnectionSavedState } from "./connectionSavedState";
import type { RefreshState } from "./pageState";

type ConnectionSummaryProps = {
  state: ConnectionSavedState;
  dirty: boolean;
  refreshState: RefreshState;
  onConnect: () => void;
  connecting: boolean;
  connectAvailable?: boolean;
  // actions は、見出しの「…」に並べる操作（名前の変更・移動・複製・テキストで編集・削除など）。
  actions: ActionMenuItem[];
};

// ConnectionSummary は、接続エディタの見出しである。名前とアドレス、［接続］、ほかの操作の「…」だけを置く。
// 鍵やパスワードの割り当て（片付けが要る割り当ても）は、基本のタブの認証の節で見る。
export function ConnectionSummary({
  state,
  dirty,
  refreshState,
  onConnect,
  connecting,
  connectAvailable = true,
  actions,
}: ConnectionSummaryProps) {
  const t = useTranslate();
  const summary = summarizeConnection(state);
  const blocked = dirty || refreshState !== "idle";
  const reasonId = `connection-actions-${encodeURIComponent(summary.alias)}`;
  // On a phone the top bar holds ［接続］ and the 「…」, as the page's other actions do (see pageChrome).
  const { phoneActionsSlot } = useContext(PageChromeContext);
  const headerActions = (
    <>
      <Button kind="primary" className="shrink-0" disabled={blocked || connecting || !connectAvailable} aria-describedby={blocked ? reasonId : undefined} onClick={onConnect}>
        {connecting ? t("conn.opening") : t("conn.connect")}
      </Button>
      <ActionMenu
        label={t("conn.connectionActions")}
        items={actions}
        triggerClassName="flex size-8 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink"
      />
    </>
  );

  // blockedReason は、接続のボタンを止めている理由である。読み直しに失敗したときは、
  // 上に出る失敗の文と再読み込みのボタンに合わせ、読み直し中とは書かない。
  function blockedReason() {
    if (dirty) return t("conn.summaryDraftBlocksActions");
    if (refreshState === "refreshing") return t("conn.summaryRefreshing");
    return t("conn.summaryReloadFirst");
  }

  return (
    <section data-connection-summary aria-labelledby="connection-summary-heading" className="shrink-0">
      <header className="flex items-center gap-2.5">
        <OperatingSystemIcon os={state.detail.metadata.os || state.detail.metadata.detectedOS} look="accentTile" />
        <div className="min-w-0 flex-1">
          {/* The unsaved badge moves under a long name instead of squeezing it. */}
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <h3 id="connection-summary-heading" className="min-w-0 max-w-full truncate text-base font-semibold text-ink" title={summary.alias}>{summary.alias}</h3>
            {dirty ? <span className="shrink-0 rounded bg-notice px-2 py-0.5 text-xs font-medium text-notice-ink">{t("conn.summaryUnsaved")}</span> : null}
          </div>
          <p className="truncate font-mono text-[11.5px] text-ink-faint" title={summary.endpoint}>{summary.endpoint}</p>
        </div>
        {phoneActionsSlot === null ? headerActions : createPortal(headerActions, phoneActionsSlot)}
      </header>
      {blocked ? <p id={reasonId} className="mt-3 text-xs text-notice-ink">{blockedReason()}</p> : null}
    </section>
  );
}
