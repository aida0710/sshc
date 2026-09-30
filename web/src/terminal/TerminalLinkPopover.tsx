import { useRef } from "react";
import { createPortal } from "react-dom";
import { useTranslate } from "../i18n/context";
import { clipboard } from "../ui/clipboard";
import { isSafeHttpURL, type TerminalLinkMatch } from "./links";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";

export type RemotePathAction = "browse" | "edit" | "download";

export type TerminalLinkSelection = {
  link: TerminalLinkMatch;
  x: number;
  y: number;
};

export function openTerminalURL(target: string): boolean {
  if (!isSafeHttpURL(target)) return false;
  const parsed = new URL(target);
  const opened = window.open(parsed.href, "_blank", "noopener,noreferrer");
  if (opened !== null) opened.opener = null;
  return opened !== null;
}

export function TerminalLinkPopover({
  selection,
  onClose,
  onRemotePath,
}: {
  selection: TerminalLinkSelection;
  onClose: () => void;
  onRemotePath?: (path: string, action: RemotePathAction) => void;
}) {
  const t = useTranslate();
  const panel = useRef<HTMLDivElement>(null);
  const firstAction = useRef<HTMLButtonElement>(null);

  // キーボードやスクリーンリーダーでも操作を選べるように、開いたら最初の操作へ
  // フォーカスを移し、Tab をパネルの中で回す。外へ出すと xterm が Tab をシェルへ
  // 送ってしまう。閉じたら、開く前にフォーカスのあったターミナルへ戻る。
  useDismissibleLayer({
    open: true,
    containerRefs: [panel],
    onDismiss: onClose,
    initialFocusRef: firstAction,
    trapFocus: true,
  });

  function copy() {
    void clipboard.writeText(selection.link.target).finally(onClose);
  }

  function openURL() {
    if (selection.link.kind !== "url") return;
    openTerminalURL(selection.link.target);
    onClose();
  }

  function remote(action: RemotePathAction) {
    if (selection.link.kind !== "remote-path" || onRemotePath === undefined) return;
    onRemotePath(selection.link.target, action);
    onClose();
  }

  const actions: { label: string; run: () => void }[] = [
    ...(selection.link.kind === "url"
      ? [{ label: t("terminal.linkOpenBrowser"), run: openURL }]
      : onRemotePath === undefined
        ? []
        : [
            { label: t("terminal.linkBrowseSFTP"), run: () => remote("browse") },
            { label: t("terminal.linkEditSFTP"), run: () => remote("edit") },
            { label: t("terminal.linkDownloadSFTP"), run: () => remote("download") },
          ]),
    { label: t("terminal.linkCopy"), run: copy },
  ];

  return createPortal(
    <div
      ref={panel}
      role="dialog"
      aria-label={t("terminal.linkActions")}
      style={{ left: Math.min(selection.x, Math.max(8, window.innerWidth - 260)), top: Math.min(selection.y, Math.max(8, window.innerHeight - 220)) }}
      className="fixed z-[80] w-64 rounded-md border border-control-line bg-card p-2 shadow-2xl"
    >
      <p className="truncate px-2 py-1 font-mono text-xs text-ink-muted" title={selection.link.target}>{selection.link.target}</p>
      <div className="mt-1 grid gap-1">
        {actions.map((action, index) => (
          <button
            key={action.label}
            ref={index === 0 ? firstAction : undefined}
            type="button"
            className="rounded px-2 py-1.5 text-left text-sm hover:bg-select-fill"
            onClick={action.run}
          >
            {action.label}
          </button>
        ))}
      </div>
    </div>,
    document.body,
  );
}
