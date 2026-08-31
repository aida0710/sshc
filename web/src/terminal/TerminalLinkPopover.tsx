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

  useDismissibleLayer({ open: true, containerRefs: [panel], onDismiss: onClose });

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
        {selection.link.kind === "url" ? (
          <button type="button" className="rounded px-2 py-1.5 text-left text-sm hover:bg-select-fill" onClick={openURL}>{t("terminal.linkOpenBrowser")}</button>
        ) : onRemotePath === undefined ? null : (
          <>
            <button type="button" className="rounded px-2 py-1.5 text-left text-sm hover:bg-select-fill" onClick={() => remote("browse")}>{t("terminal.linkBrowseSFTP")}</button>
            <button type="button" className="rounded px-2 py-1.5 text-left text-sm hover:bg-select-fill" onClick={() => remote("edit")}>{t("terminal.linkEditSFTP")}</button>
            <button type="button" className="rounded px-2 py-1.5 text-left text-sm hover:bg-select-fill" onClick={() => remote("download")}>{t("terminal.linkDownloadSFTP")}</button>
          </>
        )}
        <button type="button" className="rounded px-2 py-1.5 text-left text-sm hover:bg-select-fill" onClick={copy}>{t("terminal.linkCopy")}</button>
      </div>
    </div>,
    document.body,
  );
}
