import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import { Icon } from "../ui/icons";
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
import { SFTPCompareTabsDialog } from "./SFTPCompareTabsDialog";
import type { SFTPLocation } from "./sftpLocation";
import { rememberPanes, rememberSplitRatio, restorePanes, restoreSplitRatio } from "./sftpPaneStorage";
import {
  activeTab, addTab, allTabs, blankTab, canSplit, closeTab, findTab, maxTabsPerPane, moveTab, relocateTab, resortTab, selectTab,
  type PaneSide, type SFTPPane, type SFTPTab, type TabDestination,
} from "./sftpPanes";

// Why closing a tab waits for a confirmation, and what the dialog names.
type CloseTabIntent =
  | { kind: "unsaved"; tabId: string; path: string }
  | { kind: "connected"; tabId: string; alias: string };

function withMembership(tabIds: ReadonlySet<string>, tabId: string, member: boolean): ReadonlySet<string> {
  if (tabIds.has(tabId) === member) return tabIds;
  const next = new Set(tabIds);
  if (member) next.add(tabId);
  else next.delete(tabId);
  return next;
}

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
  const [comparisonLocations, setComparisonLocations] = useState<{ left: SFTPLocation; right: SFTPLocation } | null>(null);
  const [compareTabsOpen, setCompareTabsOpen] = useState(false);
  const [openQueueRequest, setOpenQueueRequest] = useState(0);
  const [hostPickerRequests, setHostPickerRequests] = useState<Record<string, number>>({});
  const [dirtyTabs, setDirtyTabs] = useState<Map<string, string>>(() => new Map());
  const [connectedTabs, setConnectedTabs] = useState<ReadonlySet<string>>(() => new Set());
  const [closeTabIntent, setCloseTabIntent] = useState<CloseTabIntent | null>(null);
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
  const focusedLocation = focusedPane === null ? null : activeTab(focusedPane);
  const compactTabs = focusedPane === null ? null : { ...focusedPane, tabs: allTabs(panes) };
  const comparisonTabs = allTabs(panes).filter((tab) => tab.alias !== "");

  useEffect(() => { rememberPanes(panes); }, [panes]);

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

  const connectionReporter = useCallbackPerTab<boolean>((tabId, connected) => {
    setConnectedTabs((current) => withMembership(current, tabId, connected));
  });

  function forgetTab(tabId: string) {
    restoring.current.delete(tabId);
    blockers.current.delete(tabId);
    blockerReporter.forget(tabId);
    dirtyReporter.forget(tabId);
    connectionReporter.forget(tabId);
    setConnectedTabs((current) => withMembership(current, tabId, false));
    setHostPickerRequests((current) => {
      if (!(tabId in current)) return current;
      const next = { ...current };
      delete next[tabId];
      return next;
    });
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

  // Unsaved edits are always confirmed: they would be lost. A live connection
  // is confirmed unless Shift was held, the quick way for someone who means it.
  function requestCloseTab(tab: SFTPTab, { skipConfirmation }: { skipConfirmation: boolean }) {
    const dirtyPath = dirtyTabs.get(tab.id);
    if (dirtyPath !== undefined) {
      setCloseTabIntent({ kind: "unsaved", tabId: tab.id, path: dirtyPath });
      return;
    }
    if (connectedTabs.has(tab.id) && !skipConfirmation) {
      setCloseTabIntent({ kind: "connected", tabId: tab.id, alias: tab.alias });
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

  // A request from another screen names a host. If every visible pane is
  // local, the selected pane switches to that host before handling the path.
  useEffect(() => {
    const destination = compactViewport ? focusedLocation : leftLocation;
    if (target === null || destination === null || destination.alias !== localHostAlias ||
      (visibleSplit && rightLocation?.alias !== localHostAlias)) return;
    const alias = aliases.includes(target.alias) ? target.alias : "";
    restoring.current.set(destination.id, { alias, path: "" });
    setPanes((current) => relocateTab(current, destination.id, alias, ""));
    if (!compactViewport && leftPane !== null) setFocusedPaneId(leftPane.id);
  }, [target, leftPane, leftLocation, focusedLocation, rightLocation?.alias, visibleSplit, compactViewport, aliases]);

  const compareReady = leftLocation !== null && rightLocation !== null &&
    leftLocation.alias !== "" && rightLocation.alias !== "";

  function selectFileTab(tabId: string) {
    const found = findTab(panes, tabId);
    if (found !== null) chooseTab(found.pane, tabId);
  }

  function requestHostChange(tab: SFTPTab) {
    if (dirtyTabs.has(tab.id)) return;
    selectFileTab(tab.id);
    setHostPickerRequests((current) => ({ ...current, [tab.id]: (current[tab.id] ?? 0) + 1 }));
  }

  function openFileTab() {
    const destination = focusedPane !== null && focusedPane.tabs.length < maxTabsPerPane
      ? focusedPane : panes.find((pane) => pane.tabs.length < maxTabsPerPane);
    if (destination !== undefined && destination !== null) openTab(destination);
  }

  function requestComparison() {
    if (compactViewport) {
      setCompareTabsOpen(true);
    } else if (compareReady && leftLocation !== null && rightLocation !== null) {
      setComparisonLocations({ left: leftLocation, right: rightLocation });
    }
  }

  function comparisonButton(enabled: boolean) {
    return <button type="button" aria-label={t("sftp.compare.heading")} title={t("sftp.compare.heading")}
      disabled={!enabled} onClick={requestComparison}
      className={`flex shrink-0 items-center rounded text-sm text-ink-muted hover:bg-card/50 hover:text-ink disabled:text-ink-faint ${compactViewport ? "min-h-12 w-12 justify-center" : "gap-1.5 px-2"}`}>
      <Icon name="arrowLeftRight" className={compactViewport ? "size-5" : "size-4"} />{compactViewport ? null : t("sftp.compare.action")}
    </button>;
  }

  function renderPane(pane: SFTPPane, index: number) {
    const concealed = compactViewport && pane.id !== focusedPane?.id;
    const other = index === 0 ? rightLocation : leftLocation;
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
          data-sftp-pane={pane.id}
          className="flex min-h-0 min-w-0 flex-col"
          style={{ flexBasis: visibleSplit ? `${index === 0 ? splitRatio : 100 - splitRatio}%` : "100%" }}
          hidden={concealed}
          aria-label={t(index === 0 ? "sftp.firstPane" : "sftp.secondPane")}
          onPointerDown={() => setFocusedPaneId(pane.id)}
          onFocusCapture={() => setFocusedPaneId(pane.id)}
        >
          {compactViewport ? null : <SFTPTabStrip
            pane={pane}
            label={t(index === 0 ? "sftp.primaryTabs" : "sftp.secondaryTabs")}
            closable={closable}
            movable={(tab) => !compactViewport && !dirtyTabs.has(tab.id)}
            trailing={last && visibleSplit ? comparisonButton(compareReady) : null}
            onChangeHost={requestHostChange}
            canChangeHost={(tab) => !dirtyTabs.has(tab.id)}
            onSelect={(tabId) => chooseTab(pane, tabId)}
            onClose={requestCloseTab}
            onAdd={() => openTab(pane)}
            onDragStart={setDraggedTabId}
            onDragEnd={() => setDraggedTabId(null)}
            onMove={moveTabWithKeyboard}
          />}
          <div data-sftp-pane-content={pane.id} className="relative flex min-h-0 min-w-0 flex-1 flex-col">
            {pane.tabs.map((tab) => {
              const selected = tab.id === pane.activeId;
              const restored = restoring.current.get(tab.id);
              const ownsTarget = selected && tab.alias !== localHostAlias &&
                (compactViewport ? pane.id === focusedPane?.id : other?.alias === localHostAlias || focusedPane?.id === pane.id);
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
                    hostPickerRequest={hostPickerRequests[tab.id] ?? 0}
                    {...(hosts === undefined ? {} : { hosts })}
                    {...(hostVPN === undefined ? {} : { hostVPN })}
                    target={ownsTarget ? target : null}
                    initialLocation={restored === undefined || restored.alias === "" ? null : restored}
                    initialSort={tab.sort}
                    showTransfers={false}
                    counterpart={other?.alias && other.path ? { alias: other.alias, path: other.path } : null}
                    onQueueOpen={() => setOpenQueueRequest((current) => current + 1)}
                    {...(selected ? { onNavigationBlockerChange: blockerReporter.forTab(tab.id) } : {})}
                    onDirtyChange={dirtyReporter.forTab(tab.id)}
                    onConnectionChange={connectionReporter.forTab(tab.id)}
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
      {compactViewport && compactTabs !== null ? <SFTPTabStrip
        pane={compactTabs} compact label={t("sftp.mobileTabs")} closable={compactTabs.tabs.length > 1}
        movable={() => false} addDisabled={panes.every((pane) => pane.tabs.length >= maxTabsPerPane)}
        trailing={comparisonButton(comparisonTabs.length > 1)} onChangeHost={requestHostChange}
        canChangeHost={(tab) => !dirtyTabs.has(tab.id)} onSelect={selectFileTab} onClose={requestCloseTab}
        onAdd={openFileTab} onDragStart={setDraggedTabId} onDragEnd={() => setDraggedTabId(null)} onMove={moveTabWithKeyboard}
      /> : null}
      <div className="flex min-h-0 min-w-0 flex-1">
        {panes.map(renderPane)}
      </div>
      <TransferManagerList openRequest={openQueueRequest} onNavigateLocation={onNavigateLocation} />
      {closeTabIntent?.kind === "unsaved" ? (
        <ConfirmDialog
          id="sftp-close-dirty-tab"
          heading={t("sftp.leaveHeading")}
          body={<p className="text-sm text-ink-muted">{t("sftp.leaveBody", { path: closeTabIntent.path })}</p>}
          confirmLabel={t("sftp.leaveDiscard")}
          cancelLabel={t("sftp.leaveStay")}
          onConfirm={() => {
            setCloseTabIntent(null);
            removeTab(closeTabIntent.tabId);
          }}
          onCancel={() => setCloseTabIntent(null)}
        />
      ) : null}
      {closeTabIntent?.kind === "connected" ? (
        <ConfirmDialog
          id="sftp-close-connected-tab"
          heading={t("sftp.closeConnectedHeading", { host: closeTabIntent.alias })}
          body={<>
            <p className="text-sm text-ink-muted">{t("sftp.closeConnectedBody")}</p>
            <p className="mt-2 text-xs text-ink-faint">{t("sftp.closeConnectedShiftHint")}</p>
          </>}
          confirmLabel={t("sftp.closeConnectedConfirm")}
          cancelLabel={t("sftp.closeConnectedCancel")}
          onConfirm={() => {
            setCloseTabIntent(null);
            removeTab(closeTabIntent.tabId);
          }}
          onCancel={() => setCloseTabIntent(null)}
        />
      ) : null}
      {compareTabsOpen ? <SFTPCompareTabsDialog tabs={comparisonTabs} currentTabId={focusedLocation?.id ?? ""}
        onCompare={(left, right) => { setCompareTabsOpen(false); setComparisonLocations({ left, right }); }}
        onDismiss={() => setCompareTabsOpen(false)} /> : null}
      {comparisonLocations !== null ? (
        <SFTPCompareDialog
          left={{ alias: comparisonLocations.left.alias, path: comparisonLocations.left.path || "/" }}
          right={{ alias: comparisonLocations.right.alias, path: comparisonLocations.right.path || "/" }}
          onDismiss={() => setComparisonLocations(null)}
        />
      ) : null}
    </section>
  );
}
