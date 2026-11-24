import { useEffect, useRef, useState, type ReactNode } from "react";
import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { PathText } from "../ui/PathText";
import { Button } from "../ui/surface";
import { SFTPHostPicker } from "./SFTPHostPicker";
import { SFTPPathShortcuts } from "./SFTPPathShortcuts";
import { VPNProfileChip } from "../vpn/VPNProfileChip";
import type { SFTPBrowserModel } from "./useSFTPBrowser";
import { isImeComposing } from "../ui/imeComposition";
import { useAndroidBackStop } from "../ui/useAndroidBackStop";
import { literalTextEntry } from "../ui/softKeyboard";
import { IconButton } from "../ui/IconButton";

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
  shown = true,
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
  // shown is whether this toolbar's tab is the one on the screen.
  shown?: boolean;
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
  useAndroidBackStop({ open: mobile && shown && pathEditing, close: cancelEdit });

  const pathField = pathEditing ? (
    <form className="flex min-w-0 flex-1 items-center gap-1" onSubmit={(event) => { event.preventDefault(); submitPath(); }}>
      <input {...literalTextEntry} ref={pathInput} aria-label={labels.input} value={pathDraft}
        onChange={(event) => setPathDraft(event.target.value)}
        onKeyDown={(event) => { if (!isImeComposing(event) && event.key === "Escape") cancelEdit(); }}
        className={`min-w-0 flex-1 rounded-md border border-control-line bg-control px-2 font-mono ${mobile ? "h-12 text-base" : "h-9 text-sm"}`} />
      <Button type="submit" disabled={navigationDisabled}>{t("sftp.go")}</Button>
      <IconButton label={t("sftp.cancel")} size="bar" onClick={cancelEdit}><Icon name="close" className="size-4" /></IconButton>
    </form>
  ) : mobile ? (
    <button type="button" data-testid="sftp-current-path" data-path={path}
      aria-label={labels.editPath} title={path} disabled={navigationDisabled} onClick={() => setPathEditing(true)}
      className="flex min-h-12 min-w-0 flex-1 items-center gap-1 rounded-md px-2 text-left active:bg-select-fill">
      <PathText path={path === "" ? "/" : path} className="flex-1 font-mono text-sm font-medium" />
      <Icon name="chevronRight" className="size-3 shrink-0 rotate-90 text-ink-muted" />
    </button>
  ) : (
    <div className="flex min-w-0 flex-1 items-center" data-testid="sftp-current-path" data-path={path}>
      <nav aria-label={labels.path} title={path} onClick={(event) => { if (event.target === event.currentTarget && !navigationDisabled) setPathEditing(true); }}
        className="flex min-w-0 flex-1 items-center overflow-x-auto whitespace-nowrap font-mono text-xs">
        {crumbs.map((crumb, index) => <span key={crumb.path} className="flex shrink-0 items-center">
          {/* A root such as "/" or "C:/" already ends in a slash, so the next name follows it directly. */}
          {index > 0 && !crumbs[index - 1]!.label.endsWith("/") ? <span aria-hidden="true" className="px-0.5 text-ink-faint">/</span> : null}
          {index === crumbs.length - 1 ? <span aria-current="location" className="max-w-40 truncate px-1 py-1.5 font-medium text-ink" title={crumb.path}>{crumb.label}</span>
            : <button type="button" disabled={navigationDisabled} onClick={() => void browser.load(crumb.path)}
              className="sshc-focus-inset max-w-28 truncate rounded-md px-1 py-1.5 text-ink-muted hover:bg-hover hover:text-ink" title={crumb.path}>{crumb.label}</button>}
        </span>)}
      </nav>
      <IconButton label={labels.editPath} size="toolbar" tone="faint" disabled={navigationDisabled} onClick={() => setPathEditing(true)}><Icon name="edit" className="size-3.5" /></IconButton>
    </div>
  );

  const hostPicker = (
    <SFTPHostPicker aliases={aliases} {...(hosts === undefined ? {} : { hosts })} value={alias}
      disabled={locked} onChange={onHostChange} compact={mobile} includeLocal
      hideTrigger={hostPickerRequest !== undefined} {...(hostPickerRequest === undefined ? {} : { openRequest: hostPickerRequest })} />
  );
  const bandClassName = `flex min-w-0 shrink-0 items-center border-b border-line ${mobile ? "gap-1 py-1" : "gap-1.5 bg-well px-3 py-2"}`;

  // Before a host is chosen there is no folder to move in or act on, so the band holds only the host picker.
  // When the empty pane opens the picker itself (hostPickerRequest), the picker has no trigger and no band.
  if (alias === "") {
    return hostPickerRequest === undefined ? <div className={bandClassName}>{hostPicker}</div> : hostPicker;
  }

  return (
    <>
      <div className={bandClassName}>
        {hostPicker}
        {content ?? <>
          <IconButton label={t(mobile ? "sftp.parentDirectory" : "sftp.back")} size={mobile ? "barLarge" : "toolbar"}
            disabled={busy || locked || (mobile ? !canParent : !browser.canBack)} onClick={() => { if (mobile) onParent?.(); else void browser.back(); }}>
            <Icon name={mobile ? "arrowUp" : "arrowLeft"} className={mobile ? "size-4" : "size-[15px]"} />
          </IconButton>
          {mobile ? null : <IconButton label={t("sftp.forward")} size="toolbar"
            disabled={busy || locked || !browser.canForward} onClick={() => void browser.forward()}><Icon name="arrowRight" className="size-[15px]" /></IconButton>}
          {pathField}
          <VPNProfileChip name={vpnProfile} className="max-w-24 truncate" />
          {mobile || pathEditing ? null : <IconButton label={t("sftp.refreshDirectory")} size="toolbar"
            disabled={navigationDisabled} onClick={refresh}><Icon name="sync" className="size-[15px]" /></IconButton>}
          {pathEditing ? null : <>{leading}<div className="flex shrink-0 items-center gap-1">{actions}</div></>}
        </>}
      </div>
      {mobile && pathEditing && content === undefined ? <SFTPPathShortcuts browser={browser} disabled={navigationDisabled} onMove={cancelEdit} /> : null}
    </>
  );
}
