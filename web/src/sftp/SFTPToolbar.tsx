import { useEffect, useRef, useState, type ReactNode } from "react";
import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";
import { SFTPHostPicker } from "./SFTPHostPicker";
import { VPNProfileChip } from "../vpn/VPNProfileChip";
import type { SFTPBrowserModel } from "./useSFTPBrowser";

// Navigation uses one row; on phones selection or search replaces that row.
// The workspace tab already identifies the source, so its picker stays hidden here.
export function SFTPToolbar({
  browser,
  aliases,
  hosts,
  vpnProfile = "",
  onHostChange,
  onRefresh,
  busy,
  locked = false,
  mobile,
  labels,
  leading,
  actions,
  content,
  hostPickerRequest,
  onParent,
  canParent = false,
}: {
  browser: SFTPBrowserModel;
  aliases: string[];
  hosts?: HostEntry[] | undefined;
  // vpnProfile は、いま選んでいる接続が通るVPN経路の名前である。
  vpnProfile?: string;
  onHostChange: (alias: string) => void;
  // What the refresh button re-reads. A pane showing search results re-runs
  // the search rather than the directory.
  onRefresh?: () => void;
  // Anything the pane is doing that should hold navigation.
  busy: boolean;
  // An unsaved edit: the host and directory must stay where they are.
  locked?: boolean;
  mobile: boolean;
  // Accessible names for the path bar. The engine's disk and an SSH host
  // describe themselves differently to a screen reader.
  labels: { path: string; editPath: string; input: string };
  // Terminal access is shared by local and remote sources.
  leading?: ReactNode;
  // Search and the folder menu remain available at either size.
  actions?: ReactNode;
  content?: ReactNode;
  hostPickerRequest?: number;
  onParent?: () => void;
  canParent?: boolean;
}) {
  const t = useTranslate();
  const { alias, path, connected } = browser;
  const [pathDraft, setPathDraft] = useState(path);
  const [pathEditing, setPathEditing] = useState(false);
  const pathInput = useRef<HTMLInputElement>(null);
  const navigationDisabled = busy || locked || !connected;
  const refresh = onRefresh ?? (() => { void browser.refresh(); });
  const crumbs = browser.crumbs;
  const current = crumbs.at(-1);

  // A new directory replaces the draft and closes an edit that was left open.
  useEffect(() => {
    setPathDraft(path);
    setPathEditing(false);
  }, [alias, path]);

  useEffect(() => {
    if (!pathEditing) return;
    pathInput.current?.focus();
    pathInput.current?.select();
  }, [pathEditing]);

  function submitPath() {
    if (navigationDisabled) return;
    void browser.load(pathDraft.trim());
  }

  function cancelEdit() {
    setPathDraft(path);
    setPathEditing(false);
  }

  const pathField = pathEditing ? (
    <form className="flex min-w-0 flex-1 items-center gap-1" onSubmit={(event) => { event.preventDefault(); submitPath(); }}>
      <input ref={pathInput} aria-label={labels.input} value={pathDraft}
        onChange={(event) => setPathDraft(event.target.value)}
        onKeyDown={(event) => { if (event.key === "Escape") cancelEdit(); }}
        className={`min-w-0 flex-1 rounded border border-control-line bg-control px-2 font-mono outline-none focus:border-accent ${mobile ? "h-12 text-base" : "h-9 text-sm"}`} />
      <Button type="submit" disabled={navigationDisabled}>{t("sftp.go")}</Button>
      <button type="button" aria-label={t("sftp.cancel")} onClick={cancelEdit}
        className="flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover"><Icon name="close" className="size-4" /></button>
    </form>
  ) : mobile ? (
    <button type="button" data-testid="sftp-current-path" data-path={path}
      aria-label={labels.editPath} title={path} disabled={navigationDisabled} onClick={() => setPathEditing(true)}
      className="flex min-h-12 min-w-0 flex-1 items-center gap-1 rounded px-2 text-left active:bg-select-fill disabled:text-ink-faint">
      <span className="min-w-0 flex-1 truncate font-mono text-sm font-medium">{current?.label ?? "/"}</span>
      <Icon name="chevronRight" className="size-3 shrink-0 rotate-90 text-ink-muted" />
    </button>
  ) : (
    <div className="flex min-w-0 flex-1 items-center rounded bg-control/60" data-testid="sftp-current-path" data-path={path}>
      <nav aria-label={labels.path} title={path} onClick={(event) => { if (event.target === event.currentTarget && !navigationDisabled) setPathEditing(true); }}
        className="flex min-w-0 flex-1 items-center overflow-x-auto whitespace-nowrap px-1 font-mono text-xs">
        {crumbs.map((crumb, index) => <span key={crumb.path} className="flex shrink-0 items-center">
          {index > 0 ? <Icon name="chevronRight" className="size-3 text-ink-faint" /> : null}
          {index === crumbs.length - 1 ? <span aria-current="location" className="max-w-40 truncate px-1 py-2 font-medium" title={crumb.path}>{crumb.label}</span>
            : <button type="button" disabled={navigationDisabled} onClick={() => void browser.load(crumb.path)}
              className="max-w-28 truncate rounded px-1 py-2 text-ink-muted hover:bg-hover disabled:text-ink-faint" title={crumb.path}>{crumb.label}</button>}
        </span>)}
      </nav>
      <button type="button" aria-label={labels.editPath} title={labels.editPath} disabled={navigationDisabled} onClick={() => setPathEditing(true)}
        className="flex min-h-9 min-w-9 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover disabled:text-ink-faint"><Icon name="edit" className="size-3.5" /></button>
    </div>
  );

  return (
    <div className="flex min-w-0 shrink-0 items-center gap-1 border-b border-line/50 py-1">
      <SFTPHostPicker aliases={aliases} {...(hosts === undefined ? {} : { hosts })} value={alias}
        disabled={locked} onChange={onHostChange} compact={mobile} includeLocal
        hideTrigger={hostPickerRequest !== undefined} {...(hostPickerRequest === undefined ? {} : { openRequest: hostPickerRequest })} />
      {content ?? <>
        <button type="button" aria-label={t(mobile ? "sftp.parentDirectory" : "sftp.back")} title={t(mobile ? "sftp.parentDirectory" : "sftp.back")}
          disabled={busy || locked || (mobile ? !canParent : !browser.canBack)} onClick={() => { if (mobile) onParent?.(); else void browser.back(); }}
          className={`flex shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover disabled:text-ink-faint ${mobile ? "size-12" : "size-9"}`}>
          <Icon name={mobile ? "arrowUp" : "arrowLeft"} className="size-4" />
        </button>
        {mobile ? null : <button type="button" aria-label={t("sftp.forward")} title={t("sftp.forward")}
          disabled={busy || locked || !browser.canForward} onClick={() => void browser.forward()}
          className="flex size-9 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover disabled:text-ink-faint"><Icon name="arrowRight" className="size-4" /></button>}
        {pathField}
        <VPNProfileChip name={vpnProfile} className="max-w-24 truncate" />
        {mobile || pathEditing ? null : <button type="button" aria-label={t("sftp.refreshDirectory")} title={t("sftp.refreshDirectory")}
          disabled={navigationDisabled} onClick={refresh}
          className="flex size-9 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover disabled:text-ink-faint"><Icon name="sync" className="size-4" /></button>}
        {pathEditing ? null : <>{leading}<div className="flex shrink-0 items-center gap-1">{actions}</div></>}
      </>}
    </div>
  );
}
