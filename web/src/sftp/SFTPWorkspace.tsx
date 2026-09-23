import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import type { HostEntry } from "../api/config";
import type { NavigationBlocker } from "../routing/useSectionRoute";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { SplitResizeHandle } from "../ui/SplitResizeHandle";
import { useCompactViewport } from "../ui/useMediaQuery";
import { SFTPPanel, type SFTPTarget } from "./SFTPPanel";
import type { RestoredSFTPLocation } from "./useSFTPBrowser";
import { localHostAlias } from "./localHost";
import { SFTPCompareDialog } from "./SFTPCompareDialog";
import { SFTPTabDropTarget } from "./SFTPTabDropTarget";
import { SFTPTabStrip, tabElementId, tabPanelElementId } from "./SFTPTabStrip";
import { TransferManagerList } from "./TransferManagerList";
import { rememberPanes, rememberSplitRatio, restorePanes, restoreSplitRatio } from "./sftpPaneStorage";
import {
  activeTab, addTab, allTabs, blankTab, canSplit, closeTab, findTab, moveTab, relocateTab, resortTab, selectTab,
  type PaneSide, type SFTPPane, type SFTPTab, type TabDestination,
} from "./sftpPanes";

// One stable callback per tab, so a panel's effect that depends on it does not
// re-run on every render of the workspace.
function useCallbackPerTab<T>(handle: (tabId: string, value: T) => void) {
  const callbacks = useRef(new Map<string, (value: T) => void>());
  const latest = useRef(handle);
  latest.current = handle;
  const forTab = useCallback((tabId: string) => {
    const existing = callbacks.current.get(tabId);
    if (existing !== undefined) return existing;
    const created = (value: T) => latest.current(tabId, value);
    callbacks.current.set(tabId, created);
    return created;
  }, []);
  const forget = useCallback((tabId: string) => { callbacks.current.delete(tabId); }, []);
  return { forTab, forget };
}

// The workspace lays one or two panes side by side. Each pane has its own tab
// strip; a tab dragged onto a pane moves there, and a tab dragged beside the
// only pane opens the second one. The transfer queue is global to the engine
// and is drawn once below the panes.
export function SFTPWorkspace({
  aliases,
  hostVPN,
  hosts,
  target = null,
  onTargetHandled = () => undefined,
  onNavigationBlockerChange,
  onNavigateLocation,
  onOpenTerminal,
}: {
  aliases: string[];
  // hostVPN は、alias ごとに通るVPN経路の名前である。
  hostVPN?: Map<string, string>;
  hosts?: HostEntry[];
  target?: SFTPTarget | null;
  onTargetHandled?: (request: number) => void;
  onNavigationBlockerChange?: (blocker: NavigationBlocker | null) => void;
  onNavigateLocation?: (url: string) => void;
  onOpenTerminal?: (alias: string, path: string) => void | Promise<void>;
}) {
  const t = useTranslate();
  const [panes, setPanes] = useState<SFTPPane[]>(restorePanes);
  const [splitRatio, setSplitRatio] = useState(restoreSplitRatio);
  const [focusedPaneId, setFocusedPaneId] = useState(() => panes[0]?.id ?? "");
  const [draggedTabId, setDraggedTabId] = useState<string | null>(null);
  const [compareOpen, setCompareOpen] = useState(false);
  const [openQueueRequest, setOpenQueueRequest] = useState(0);
  const [dirtyTabs, setDirtyTabs] = useState<Map<string, string>>(() => new Map());
  const [closeTabIntent, setCloseTabIntent] = useState<{ tabId: string; path: string } | null>(null);
  const workspaceRoot = useRef<HTMLElement | null>(null);
  const compactViewport = useCompactViewport(workspaceRoot);
  // Restoring is a one-shot per tab: once a panel has opened its remembered
  // directory, later navigation inside it must not be pulled back.
  const restoring = useRef(new Map<string, RestoredSFTPLocation>(allTabs(panes).map((tab) => [tab.id, { alias: tab.alias, path: tab.path }])));
  const blockers = useRef(new Map<string, NavigationBlocker>());
  const visibleSplit = panes.length > 1 && !compactViewport;
  const leftPane = panes[0] ?? null;
  const rightPane = panes[1] ?? null;
  const focusedPane = panes.find((pane) => pane.id === focusedPaneId) ?? leftPane;

  useEffect(() => { rememberPanes(panes); }, [panes]);

  useEffect(() => {
    if (!compactViewport || leftPane === null) return;
    setFocusedPaneId(leftPane.id);
    setCompareOpen(false);
  }, [compactViewport, leftPane]);

  useEffect(() => {
    if (draggedTabId !== null && findTab(panes, draggedTabId) === null) setDraggedTabId(null);
  }, [draggedTabId, panes]);

  const publishBlockers = useCallback(() => {
    const active = [...blockers.current.values()];
    onNavigationBlockerChange?.(active.length === 0 ? null : (next) => active.every((blocker) => blocker(next)));
  }, [onNavigationBlockerChange]);

  const blockerReporter = useCallbackPerTab<NavigationBlocker | null>((tabId, blocker) => {
    if (blocker === null) blockers.current.delete(tabId);
    else blockers.current.set(tabId, blocker);
    publishBlockers();
  });

  const dirtyReporter = useCallbackPerTab<string | null>((tabId, path) => {
    setDirtyTabs((current) => {
      if (path === null) {
        if (!current.has(tabId)) return current;
        const next = new Map(current);
        next.delete(tabId);
        return next;
      }
      if (current.get(tabId) === path) return current;
      return new Map(current).set(tabId, path);
    });
  });

  function forgetTab(tabId: string) {
    restoring.current.delete(tabId);
    blockers.current.delete(tabId);
    blockerReporter.forget(tabId);
    dirtyReporter.forget(tabId);
    setDirtyTabs((current) => {
      if (!current.has(tabId)) return current;
      const next = new Map(current);
      next.delete(tabId);
      return next;
    });
  }

  function openTab(pane: SFTPPane) {
    const opened = blankTab();
    setPanes((current) => addTab(current, pane.id, opened));
    setFocusedPaneId(pane.id);
  }

  function removeTab(tabId: string) {
    forgetTab(tabId);
    setPanes((current) => closeTab(current, tabId));
  }

  function requestCloseTab(tab: SFTPTab) {
    const dirtyPath = dirtyTabs.get(tab.id);
    if (dirtyPath !== undefined) {
      setCloseTabIntent({ tabId: tab.id, path: dirtyPath });
      return;
    }
    removeTab(tab.id);
  }

  function chooseTab(pane: SFTPPane, tabId: string) {
    setPanes((current) => selectTab(current, tabId));
    setFocusedPaneId(pane.id);
  }

  // A moved tab is unmounted from one pane and mounted in the other, so the
  // new panel reopens the directory the tab was showing. A tab still waiting
  // for Connect since it was restored keeps waiting.
  function placeTab(tabId: string, destination: TabDestination) {
    const found = findTab(panes, tabId);
    if (found === null) return;
    const next = moveTab(panes, tabId, destination);
    if (next === panes) return;
    const live = !restoring.current.has(tabId);
    restoring.current.set(tabId, { alias: found.tab.alias, path: found.tab.path, connect: live });
    setPanes(next);
    setFocusedPaneId(findTab(next, tabId)?.pane.id ?? next[0]?.id ?? "");
  }

  function moveTabWithKeyboard(tabId: string, side: PaneSide) {
    const found = findTab(panes, tabId);
    if (found === null) return;
    const neighbour = side === "left" ? panes[panes.indexOf(found.pane) - 1] : panes[panes.indexOf(found.pane) + 1];
    if (neighbour !== undefined) placeTab(tabId, { paneId: neighbour.id });
    else if (panes.length === 1) placeTab(tabId, { side });
  }

  function dropTab(pane: SFTPPane, side: PaneSide | null) {
    const tabId = draggedTabId;
    setDraggedTabId(null);
    if (tabId === null) return;
    placeTab(tabId, side === null ? { paneId: pane.id } : { side });
  }

  const changeSplitRatio = (ratio: number) => {
    setSplitRatio(ratio);
    rememberSplitRatio(ratio);
  };

  const leftLocation = leftPane === null ? null : activeTab(leftPane);
  const rightLocation = rightPane === null ? null : activeTab(rightPane);

  // A request from another screen names a host. When the visible tabs all show
  // the engine's own disk, the left tab switches to that host first.
  useEffect(() => {
    if (target === null || leftLocation === null || leftLocation.alias !== localHostAlias ||
      (visibleSplit && rightLocation?.alias !== localHostAlias)) return;
    const alias = aliases.includes(target.alias) ? target.alias : "";
    restoring.current.set(leftLocation.id, { alias, path: "" });
    setPanes((current) => relocateTab(current, leftLocation.id, alias, ""));
    if (leftPane !== null) setFocusedPaneId(leftPane.id);
  }, [target, leftPane, leftLocation, rightLocation?.alias, visibleSplit, aliases]);

  const compareEnabled = visibleSplit && leftLocation !== null && rightLocation !== null &&
    leftLocation.alias !== "" && rightLocation.alias !== "" &&
    leftLocation.alias !== localHostAlias && rightLocation.alias !== localHostAlias;

  function renderPane(pane: SFTPPane, index: number) {
    const concealed = index > 0 && !visibleSplit;
    const other = index === 0 ? rightLocation : leftLocation;
    const otherVisible = visibleSplit ? other : null;
    const last = index === panes.length - 1;
    const closable = pane.tabs.length > 1 || panes.length > 1;
    const dropPlacement = draggedTabId === null || compactViewport ? null
      : panes.length > 1 ? (findTab(panes, draggedTabId)?.pane.id === pane.id ? null : "whole" as const)
        : canSplit(panes) ? "halves" as const : null;
    return (
      <Fragment key={pane.id}>
        {index > 0 && visibleSplit ? (
          <SplitResizeHandle direction="horizontal" ratio={splitRatio} label={t("sftp.resizePanes")} onRatioChange={changeSplitRatio} />
        ) : null}
        <div
          className="flex min-h-0 min-w-0 flex-col"
          style={{ flexBasis: visibleSplit ? `${index === 0 ? splitRatio : 100 - splitRatio}%` : "100%" }}
          hidden={concealed}
          aria-label={t(index === 0 ? "sftp.firstPane" : "sftp.secondPane")}
          onPointerDown={() => setFocusedPaneId(pane.id)}
          onFocusCapture={() => setFocusedPaneId(pane.id)}
        >
          <SFTPTabStrip
            pane={pane}
            label={t(index === 0 ? "sftp.primaryTabs" : "sftp.secondaryTabs")}
            closable={closable}
            movable={(tab) => !compactViewport && !dirtyTabs.has(tab.id)}
            trailing={last && visibleSplit ? (
              <button
                type="button"
                aria-label={t("sftp.compare.heading")}
                title={t("sftp.compare.heading")}
                disabled={!compareEnabled}
                onClick={() => setCompareOpen(true)}
                className="flex shrink-0 items-center gap-1.5 rounded px-2.5 text-sm text-ink-muted hover:bg-card/50 hover:text-ink disabled:text-ink-faint"
              >
                <span aria-hidden="true">⇄</span>
                {t("sftp.compare.action")}
              </button>
            ) : null}
            onSelect={(tabId) => chooseTab(pane, tabId)}
            onClose={requestCloseTab}
            onAdd={() => openTab(pane)}
            onDragStart={setDraggedTabId}
            onDragEnd={() => setDraggedTabId(null)}
            onMove={moveTabWithKeyboard}
          />
          <div data-sftp-pane-content={pane.id} className="relative flex min-h-0 min-w-0 flex-1 flex-col pt-2">
            {pane.tabs.map((tab) => {
              const selected = tab.id === pane.activeId;
              const restored = restoring.current.get(tab.id);
              const ownsTarget = selected && tab.alias !== localHostAlias &&
                (compactViewport ? index === 0 : otherVisible?.alias === localHostAlias || focusedPane?.id === pane.id);
              return (
                <div
                  key={tab.id}
                  id={tabPanelElementId(tab.id)}
                  role="tabpanel"
                  aria-labelledby={tabElementId(tab.id)}
                  hidden={!selected}
                  className={selected ? "flex min-h-0 min-w-0 flex-1 flex-col" : ""}
                >
                  <SFTPPanel
                    aliases={aliases}
                    {...(hosts === undefined ? {} : { hosts })}
                    {...(hostVPN === undefined ? {} : { hostVPN })}
                    target={ownsTarget ? target : null}
                    initialLocation={restored === undefined || restored.alias === "" ? null : restored}
                    initialSort={tab.sort}
                    showTransfers={false}
                    counterpart={otherVisible?.alias && otherVisible.path ? { alias: otherVisible.alias, path: otherVisible.path } : null}
                    onQueueOpen={() => setOpenQueueRequest((current) => current + 1)}
                    {...(selected ? { onNavigationBlockerChange: blockerReporter.forTab(tab.id) } : {})}
                    onDirtyChange={dirtyReporter.forTab(tab.id)}
                    {...(onNavigateLocation === undefined ? {} : { onNavigateLocation })}
                    {...(onOpenTerminal === undefined ? {} : { onOpenTerminal })}
                    {...(ownsTarget ? { onTargetHandled } : {})}
                    onLocationChange={(alias, path) => {
                      restoring.current.delete(tab.id);
                      setPanes((current) => relocateTab(current, tab.id, alias, path));
                    }}
                    onSortChange={(sort) => setPanes((current) => resortTab(current, tab.id, sort))}
                  />
                </div>
              );
            })}
            {dropPlacement === null ? null : (
              <SFTPTabDropTarget placement={dropPlacement} onDrop={(side) => dropTab(pane, side)} />
            )}
          </div>
        </div>
      </Fragment>
    );
  }

  return (
    <section ref={workspaceRoot} className="flex h-full min-h-0 min-w-0 flex-col" aria-label={t("sftp.tabs")}>
      <div className="flex min-h-0 min-w-0 flex-1">
        {panes.map(renderPane)}
      </div>
      <TransferManagerList openRequest={openQueueRequest} />
      {closeTabIntent === null ? null : (
        <ConfirmDialog
          id="sftp-close-dirty-tab"
          heading={t("sftp.leaveHeading")}
          body={<p className="text-sm text-ink-muted">{t("sftp.leaveBody", { path: closeTabIntent.path })}</p>}
          confirmLabel={t("sftp.leaveDiscard")}
          cancelLabel={t("sftp.leaveStay")}
          onConfirm={() => {
            const intent = closeTabIntent;
            setCloseTabIntent(null);
            removeTab(intent.tabId);
          }}
          onCancel={() => setCloseTabIntent(null)}
        />
      )}
      {compareOpen && compareEnabled && leftLocation !== null && rightLocation !== null ? (
        <SFTPCompareDialog
          left={{ alias: leftLocation.alias, path: leftLocation.path || "/" }}
          right={{ alias: rightLocation.alias, path: rightLocation.path || "/" }}
          onDismiss={() => setCompareOpen(false)}
        />
      ) : null}
    </section>
  );
}
