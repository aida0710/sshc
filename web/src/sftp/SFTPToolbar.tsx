import { useEffect, useRef, useState, type ReactNode } from "react";
import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import { clipboard } from "../ui/clipboard";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";
import { SFTPHostPicker } from "./SFTPHostPicker";
import { SFTPNavigationControls } from "./SFTPNavigationControls";
import { VPNRouteChip } from "../vpn/VPNRouteChip";
import type { SFTPBrowserModel } from "./useSFTPBrowser";

// The row above the file list: which host, where in it, and how to move.
// One component for every source, so that the order and size of its
// controls cannot differ between an SSH host and the engine's own disk.
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
  mobileActions,
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
  // Desktop-only controls placed between the host and the navigation.
  leading?: ReactNode;
  // Phone-only controls placed after the refresh button.
  mobileActions?: ReactNode;
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

  async function copyPath() {
    try {
      await clipboard.writeText(path);
      browser.setProblem("");
    } catch {
      browser.setProblem(t("copy.refused"));
    }
  }

  if (mobile) {
    return (
      <div className="flex min-h-11 shrink-0 items-center gap-1 border-b border-line/50 pb-1">
        <SFTPHostPicker aliases={aliases} {...(hosts === undefined ? {} : { hosts })} value={alias} disabled={locked} onChange={onHostChange} compact includeLocal />
        <VPNRouteChip name={vpnProfile} />
        <button type="button" aria-label={t("sftp.back")} disabled={busy || locked || !browser.canBack} onClick={() => void browser.back()} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill disabled:text-ink-faint">←</button>
        {pathEditing ? (
          <form className="flex min-w-0 flex-1 items-center gap-1" onSubmit={(event) => { event.preventDefault(); submitPath(); }}>
            <input ref={pathInput} aria-label={labels.input} value={pathDraft} onChange={(event) => setPathDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Escape") cancelEdit(); }} className="h-11 min-w-0 w-full rounded border border-control-line bg-control px-2 font-mono text-base" />
            <Button type="submit" disabled={navigationDisabled}>{t("sftp.go")}</Button>
          </form>
        ) : (
          <button type="button" data-testid="sftp-current-path" data-path={path} aria-label={labels.editPath} title={path} disabled={navigationDisabled} onClick={() => setPathEditing(true)} className="flex h-11 min-w-0 flex-1 items-center gap-1 rounded px-2 text-left active:bg-select-fill disabled:text-ink-faint">
            <span className="truncate font-mono text-sm font-medium">{current?.label ?? "/"}</span><Icon name="chevronRight" className="size-3 shrink-0 rotate-90 text-ink-muted" />
          </button>
        )}
        {pathEditing ? <button type="button" aria-label={t("sftp.cancel")} onClick={cancelEdit} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted"><Icon name="close" className="size-4" /></button> : <>
          <button type="button" aria-label={t("sftp.refreshDirectory")} disabled={navigationDisabled} onClick={refresh} className="flex size-11 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill disabled:text-ink-faint"><Icon name="sync" className="size-4" /></button>
          {mobileActions}
        </>}
      </div>
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5 border-b border-line/50 pb-1.5 md:pb-1">
      <SFTPHostPicker aliases={aliases} {...(hosts === undefined ? {} : { hosts })} value={alias} disabled={locked} onChange={onHostChange} includeLocal />
      <VPNRouteChip name={vpnProfile} />
      {leading}
      <SFTPNavigationControls busy={busy || locked} canBack={browser.canBack} canForward={browser.canForward}
        canHome={connected} canRoot={connected && !browser.atRoot}
        onBack={() => void browser.back()} onForward={() => void browser.forward()}
        onHome={() => void browser.goHome()} onRoot={() => void browser.goRoot()} />
      <button type="button" aria-label={t("sftp.refreshDirectory")} title={t("sftp.refreshDirectory")} disabled={navigationDisabled} onClick={refresh} className="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint md:size-8"><Icon name="sync" className="size-4" /></button>
      {pathEditing ? (
        <>
          <input
            ref={pathInput}
            aria-label={labels.input}
            value={pathDraft}
            onChange={(event) => setPathDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.preventDefault();
                cancelEdit();
              } else if (event.key === "Enter") {
                event.preventDefault();
                submitPath();
              }
            }}
            className="min-w-44 grow rounded-md border border-control-line/70 bg-control px-2 py-1.5 font-mono text-sm outline-none focus:border-accent md:py-1"
          />
          <Button disabled={navigationDisabled} onClick={submitPath}>{t("sftp.go")}</Button>
        </>
      ) : (
        <div className="flex min-w-44 grow items-center rounded-md bg-control/60 px-1" data-testid="sftp-current-path" data-path={path}>
          <nav aria-label={labels.path} onClick={(event) => { if (event.target === event.currentTarget && !navigationDisabled) setPathEditing(true); }}
            title={labels.editPath} className="flex min-w-0 grow cursor-text items-center overflow-x-auto whitespace-nowrap font-mono text-sm">
            {crumbs.map((crumb, index) => (
              <span key={crumb.path} className="flex min-w-0 items-center">
                {index > 0 ? <Icon name="chevronRight" className="size-3 text-ink-faint" /> : null}
                {index === crumbs.length - 1 ? (
                  <span className="max-w-48 truncate px-1.5 py-1.5 font-medium text-ink md:py-1" title={crumb.path} aria-current="location">{crumb.label}</span>
                ) : (
                  <button type="button" disabled={navigationDisabled} onClick={() => void browser.load(crumb.path)} className="max-w-40 truncate rounded px-1.5 py-1.5 text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint md:py-1" title={crumb.path}>{crumb.label}</button>
                )}
              </span>
            ))}
          </nav>
          <button type="button" aria-label={t("sftp.copyPath")} title={t("sftp.copyPath")}
            disabled={!connected} onClick={() => { void copyPath(); }}
            className="flex size-8 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">
            <Icon name="copy" className="size-3.5" />
          </button>
          <button type="button" aria-label={labels.editPath} title={labels.editPath} disabled={navigationDisabled} onClick={() => setPathEditing(true)} className="flex size-8 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint">
            <Icon name="edit" className="size-3.5" />
          </button>
        </div>
      )}
    </div>
  );
}
