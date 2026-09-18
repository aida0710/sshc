import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { failureCode } from "../../api/client";
import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";
import { MAX_WORKSPACE_PANES, paneIDs, paneSessionIDs, reduceLayout, storeLayout, type DockEdge, type LayoutAction, type LayoutState, type RuntimeNode, type RuntimePane, type SplitDirection } from "./layout";
import { workspaceApi, type SavedWorkspace } from "./api";
import { WorkspaceCommandCenter } from "./WorkspaceCommandCenter";
import { consoleDragMimeType, type LiveWorkspaceSummary } from "./live";
import { browserSessionStorage, loadLiveWorkspace, saveLiveWorkspace } from "./livePersistence";
import { InputDialog } from "../../ui/InputDialog";
import { useMediaQuery } from "../../ui/useMediaQuery";
import { automaticWorkspaceName, findPane, findPaneBySession, paneForSession, paneID, restoreLiveNode, singlePaneLayout } from "./panes";
import { commandTargetsFor } from "./commandTargets";
import { DockPreview, dockEdge } from "./DockPreview";
import { WorkspaceEmptyState } from "./WorkspaceEmptyState";
import { WorkspaceMenu } from "./WorkspaceMenu";
import { useWorkspaceRestore, type WorkspaceRestoreRequest } from "./useWorkspaceRestore";

export type { WorkspaceRestoreRequest } from "./useWorkspaceRestore";
export type WorkspaceRenameRequest = { name: string; sequence: number };

export function TerminalWorkspace({
  sessions, activeSessionId, onActive, onOpenAlias, onOpenShell, onClose, renderTerminal, restoreRequest = null, onRestoreConsumed = () => undefined,
  renameRequest = null, onRenameConsumed = () => undefined, onLiveWorkspaceChange = () => undefined, sessionsLoaded = true,
}: {
  sessions: TerminalSession[];
  activeSessionId: string | null;
  onActive: (id: string) => void;
  onOpenAlias: (alias: string) => Promise<TerminalSession | null>;
  onOpenShell: () => Promise<TerminalSession | null>;
  onClose: (id: string) => Promise<void>;
  renderTerminal: (session: TerminalSession) => ReactNode;
  restoreRequest?: WorkspaceRestoreRequest | null;
  onRestoreConsumed?: (sequence: number) => void;
  renameRequest?: WorkspaceRenameRequest | null;
  onRenameConsumed?: (sequence: number) => void;
  onLiveWorkspaceChange?: (workspace: LiveWorkspaceSummary | null) => void;
  sessionsLoaded?: boolean;
}) {
  const t = useTranslate();
  const [layout, setLayout] = useState<LayoutState | null>(null);
  const [saved, setSaved] = useState<SavedWorkspace[]>([]);
  const [selectedWorkspace, setSelectedWorkspace] = useState("");
  const [commandCenter, setCommandCenter] = useState(false);
  const [focusModePaneId, setFocusModePaneId] = useState<string | null>(null);
  const [movingPaneId, setMovingPaneId] = useState<string | null>(null);
  const [dockTarget, setDockTarget] = useState<{ paneId: string; edge: DockEdge } | null>(null);
  const compactViewport = useMediaQuery("(max-width: 767px)");
  const [problem, setProblem] = useState("");
  const [liveRestoreReady, setLiveRestoreReady] = useState(false);
  const [liveWorkspaceName, setLiveWorkspaceName] = useState("");
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const consumedRename = useRef(0);
  const liveWorkspaceID = useRef(paneID());
  const liveStorage = useMemo(() => browserSessionStorage(), []);
  const beginRestore = useCallback((id: string) => {
    setSelectedWorkspace(id);
    setLiveWorkspaceName("");
    setFocusModePaneId(null);
  }, []);
  const restoreWorkspace = useWorkspaceRestore({
    restoreRequest, onRestoreConsumed, onOpenAlias, onOpenShell, onClose, onActive,
    onBegin: beginRestore, setLayout, report: setProblem,
  });
  const active = sessions.find((session) => session.id === activeSessionId) ?? null;
  const sessionByID = useMemo(() => new Map(sessions.map((session) => [session.id, session])), [sessions]);
  const layoutSessionIDs = useMemo(() => layout === null ? [] : paneSessionIDs(layout.root), [layout]);
  const showingWorkspace = layout !== null && (activeSessionId === null || layoutSessionIDs.includes(activeSessionId));
  const visibleLayout = showingWorkspace ? layout : null;
  const commandTargets = useMemo(() => commandTargetsFor(visibleLayout, active, sessionByID), [active, sessionByID, visibleLayout]);
  const connectedCommandTargets = commandTargets.filter((target) => target.connected).length;
  const workspacePaneCount = visibleLayout === null ? (active === null ? 0 : 1) : paneIDs(visibleLayout.root).length;
  const workspaceDisplayName = useMemo(() => {
    if (visibleLayout === null) return active?.title ?? t("workspace.live");
    const liveName = liveWorkspaceName.trim();
    if (liveName !== "") return liveName;
    const selectedName = saved.find((item) => item.id === selectedWorkspace)?.name.trim() ?? "";
    if (selectedName !== "") return selectedName;
    return automaticWorkspaceName(visibleLayout) || t("workspace.live");
  }, [active?.title, liveWorkspaceName, saved, selectedWorkspace, t, visibleLayout]);

  useEffect(() => { void workspaceApi.list().then(setSaved).catch(() => undefined); }, []);
  useEffect(() => {
    document.title = active === null ? "sshc" : `${active.presentation?.displayTitle ?? active.title} · sshc`;
    return () => { document.title = "sshc"; };
  }, [active]);
  useEffect(() => {
    if (!sessionsLoaded || liveRestoreReady) return;
    if (restoreRequest !== null) {
      setLiveRestoreReady(true);
      return;
    }
    const restored = loadLiveWorkspace(liveStorage, new Set(sessionByID.keys()));
    if (restored !== null) {
      const root = restoreLiveNode(restored.root, sessionByID);
      if (root !== null && paneIDs(root).length > 1) {
        const focusedPaneId = paneIDs(root).includes(restored.focusedPaneId)
          ? restored.focusedPaneId
          : paneIDs(root)[0] ?? "";
        const next = { root, focusedPaneId };
        setLayout(next);
        setFocusModePaneId(restored.focusModePaneId);
        setLiveWorkspaceName(restored.name);
        const focusedSessionId = findPane(root, focusedPaneId)?.sessionId;
        if (focusedSessionId !== undefined) onActive(focusedSessionId);
      }
    }
    setLiveRestoreReady(true);
  }, [liveRestoreReady, liveStorage, onActive, restoreRequest, sessionByID, sessionsLoaded]);
  useEffect(() => {
    if (!sessionsLoaded || !liveRestoreReady) return;
    saveLiveWorkspace(liveStorage, layout, focusModePaneId, liveWorkspaceName);
  }, [focusModePaneId, layout, liveRestoreReady, liveStorage, liveWorkspaceName, sessionsLoaded]);
  useEffect(() => {
    if (layout === null && liveRestoreReady) setLiveWorkspaceName("");
  }, [layout, liveRestoreReady]);
  useEffect(() => {
    if (movingPaneId === null) return;
    if (layout === null || !paneIDs(layout.root).includes(movingPaneId)) {
      setMovingPaneId(null);
    }
  }, [layout, movingPaneId]);
  useEffect(() => {
    if (focusModePaneId === null) return;
    if (layout === null || !paneIDs(layout.root).includes(focusModePaneId)) setFocusModePaneId(null);
  }, [focusModePaneId, layout]);
  useEffect(() => {
    if (layout === null) {
      onLiveWorkspaceChange(null);
      return;
    }
    const memberSessionIds = paneSessionIDs(layout.root);
    if (memberSessionIds.length < 2) {
      onLiveWorkspaceChange(null);
      return;
    }
    const selectedName = saved.find((item) => item.id === selectedWorkspace)?.name.trim() ?? "";
    const automaticName = automaticWorkspaceName(layout);
    const focusedSessionId = findPane(layout.root, layout.focusedPaneId)?.sessionId ?? memberSessionIds[0] ?? "";
    onLiveWorkspaceChange({
      id: liveWorkspaceID.current,
      name: liveWorkspaceName.trim() || selectedName || automaticName || t("workspace.live"),
      memberSessionIds,
      focusedSessionId,
    });
  }, [layout, liveWorkspaceName, onLiveWorkspaceChange, saved, selectedWorkspace, t]);
  useEffect(() => {
    if (layout === null) return;
    // Opening several saved panes causes independent session refreshes. A short
    // response can therefore omit a session that another response has just
    // created. Reconcile only after the session list has stayed missing long
    // enough to distinguish a real close from that transient view.
    const timer = window.setTimeout(() => {
      const available = new Set(sessions.map((session) => session.id));
      setLayout((current) => {
        if (current === null) return null;
        let next = current;
        for (const paneId of paneIDs(current.root)) {
          const pane = findPane(next.root, paneId);
          if (pane?.sessionId !== undefined && !available.has(pane.sessionId)) {
            next = reduceLayout(next, { type: "close", paneId });
          }
        }
        return paneIDs(next.root).length < 2 ? null : next;
      });
    }, 500);
    return () => window.clearTimeout(timer);
  }, [layout, sessions]);
  useEffect(() => {
    const exit = (event: KeyboardEvent) => { if (event.key === "Escape") setFocusModePaneId(null); };
    window.addEventListener("keydown", exit);
    return () => window.removeEventListener("keydown", exit);
  }, []);
  useEffect(() => {
    if (layout === null || activeSessionId === null || !layoutSessionIDs.includes(activeSessionId)) return;
    const pane = findPaneBySession(layout.root, activeSessionId);
    if (pane !== null && pane.id !== layout.focusedPaneId) {
      setLayout((current) => current === null ? current : reduceLayout(current, { type: "focus", paneId: pane.id }));
    }
  }, [activeSessionId, layout, layoutSessionIDs]);

  function update(action: LayoutAction) { setLayout((current) => current === null ? current : reduceLayout(current, action)); }

  function finishPaneMove(sourcePaneId: string, targetPaneId: string) {
    if (sourcePaneId !== targetPaneId) update({ type: "swap-panes", sourcePaneId, targetPaneId });
    setMovingPaneId(null);
  }

  function choosePaneMove(paneId: string) {
    if (movingPaneId === null) {
      setMovingPaneId(paneId);
      return;
    }
    finishPaneMove(movingPaneId, paneId);
  }

  function beginPaneDrag(event: DragEvent<HTMLButtonElement>, paneId: string) {
    event.stopPropagation();
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", paneId);
    const sessionId = layout === null ? undefined : findPane(layout.root, paneId)?.sessionId;
    if (sessionId !== undefined) event.dataTransfer.setData(consoleDragMimeType, sessionId);
    setMovingPaneId(paneId);
  }

  function dockSession(sourceSessionId: string, targetPaneId: string, edge: DockEdge, targetSession?: TerminalSession) {
    const source = sessionByID.get(sourceSessionId);
    if (source === undefined) return;
    if (targetSession !== undefined) {
      if (targetSession.id === source.id) return;
      if (layout !== null) {
        setProblem(t("workspace.oneLiveOnly"));
        return;
      }
      const targetPaneId = paneID();
      const next = reduceLayout(singlePaneLayout(targetSession, targetPaneId), {
        type: "dock-pane",
        targetPaneId,
        edge,
        pane: paneForSession(source),
      });
      setSelectedWorkspace("");
      setLiveWorkspaceName("");
      setLayout(next);
      setProblem("");
      onActive(source.id);
      return;
    }
    if (layout === null || !showingWorkspace) return;
    const existing = findPaneBySession(layout.root, source.id);
    if (existing === null && paneIDs(layout.root).length >= MAX_WORKSPACE_PANES) {
      setProblem(t("workspace.maxPanes", { count: MAX_WORKSPACE_PANES }));
      return;
    }
    const pane: RuntimePane = existing ?? paneForSession(source);
    setLayout((current) => current === null ? current : reduceLayout(current, { type: "dock-pane", targetPaneId, edge, pane }));
    setProblem("");
    onActive(source.id);
  }

  function dragOverPane(event: DragEvent<HTMLDivElement>, targetPaneId: string) {
    const acceptsConsole = event.dataTransfer.types?.includes(consoleDragMimeType) === true || movingPaneId !== null;
    if (!acceptsConsole) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = "move";
    setDockTarget({ paneId: targetPaneId, edge: dockEdge(event) });
  }

  function dropPane(event: DragEvent<HTMLDivElement>, targetPaneId: string) {
    event.preventDefault();
    event.stopPropagation();
    const sourceSessionId = event.dataTransfer.getData(consoleDragMimeType) ||
      (movingPaneId === null || layout === null ? "" : findPane(layout.root, movingPaneId)?.sessionId ?? "");
    const edge = dockTarget?.paneId === targetPaneId ? dockTarget.edge : dockEdge(event);
    if (sourceSessionId !== "") dockSession(sourceSessionId, targetPaneId, edge);
    setMovingPaneId(null);
    setDockTarget(null);
  }

  function dropOnSingle(event: DragEvent<HTMLDivElement>, target: TerminalSession) {
    event.preventDefault();
    event.stopPropagation();
    const sourceSessionId = event.dataTransfer.getData(consoleDragMimeType);
    if (sourceSessionId !== "") dockSession(sourceSessionId, target.id, dockEdge(event), target);
    setDockTarget(null);
  }

  function detachPane(paneId: string) {
    if (layout === null) return;
    if (paneIDs(layout.root).length <= 2) {
      setLayout(null);
      setFocusModePaneId(null);
      setSelectedWorkspace("");
      setLiveWorkspaceName("");
      return;
    }
    update({ type: "close", paneId });
  }

  function beginResize(event: ReactPointerEvent<HTMLDivElement>, path: ("first" | "second")[], direction: SplitDirection) {
    event.preventDefault();
    event.stopPropagation();
    const container = event.currentTarget.parentElement;
    if (container === null) return;
    const pointerId = event.pointerId;
    event.currentTarget.setPointerCapture(pointerId);
    const move = (next: PointerEvent) => {
      const bounds = container.getBoundingClientRect();
      const extent = direction === "horizontal" ? bounds.width : bounds.height;
      if (extent <= 0) return;
      const offset = direction === "horizontal" ? next.clientX - bounds.left : next.clientY - bounds.top;
      update({ type: "resize-split", path, ratio: offset / extent * 100 });
    };
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      window.removeEventListener("pointercancel", stop);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop, { once: true });
    window.addEventListener("pointercancel", stop, { once: true });
  }

  async function saveWorkspace(name: string) {
    const effective = visibleLayout ?? (active === null ? null : singlePaneLayout(active));
    if (effective === null) return;
    try {
      const stored = storeLayout(effective);
      const value = selectedWorkspace === "" ? await workspaceApi.create({ name, ...stored }) : await workspaceApi.update(selectedWorkspace, { name, ...stored });
      setSelectedWorkspace(value.id); setLiveWorkspaceName(value.name); setSaved(await workspaceApi.list()); setProblem("");
    } catch (error) { setProblem(failureCode(error) || "workspace_failed"); }
  }

  useEffect(() => {
    if (renameRequest === null || renameRequest.sequence <= consumedRename.current) return;
    consumedRename.current = renameRequest.sequence;
    onRenameConsumed(renameRequest.sequence);
    const name = renameRequest.name.trim();
    if (layout !== null && name !== "") setLiveWorkspaceName(name);
  }, [layout, onRenameConsumed, renameRequest]);

  function terminal(session: TerminalSession) { return renderTerminal(session); }

  function singleTerminal(session: TerminalSession) {
    const docking = dockTarget?.paneId === session.id;
    return <div
      data-single-terminal-drop-target={session.id}
      className={`relative flex h-full min-h-0 flex-col ${docking ? "ring-2 ring-inset ring-live" : ""}`}
      onDragEnter={(event) => {
        if (event.dataTransfer.types?.includes(consoleDragMimeType) !== true) return;
        event.preventDefault();
        setDockTarget({ paneId: session.id, edge: dockEdge(event) });
      }}
      onDragOver={(event) => {
        if (event.dataTransfer.types?.includes(consoleDragMimeType) !== true) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = "move";
        setDockTarget({ paneId: session.id, edge: dockEdge(event) });
      }}
      onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDockTarget(null); }}
      onDrop={(event) => dropOnSingle(event, session)}
    >
      {terminal(session)}
      {docking ? <DockPreview edge={dockTarget.edge} /> : null}
    </div>;
  }

  function renderNode(node: RuntimeNode, path: ("first" | "second")[] = []): ReactNode {
    if (node.pane !== undefined) {
      const session = node.pane.sessionId === undefined ? undefined : sessionByID.get(node.pane.sessionId);
      const paneLabel = node.pane.kind === "shell" ? session?.title ?? node.pane.alias : node.pane.alias;
      const multiple = layout !== null && paneIDs(layout.root).length > 1;
      const moving = movingPaneId === node.pane.id;
      const docking = dockTarget?.paneId === node.pane.id && !moving;
      return (
        <div
          key={node.pane.id}
          data-workspace-pane={node.pane.id}
          data-pane-alias={node.pane.alias}
          className={`relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden border ${layout?.focusedPaneId === node.pane.id ? "border-accent" : "border-line"} ${moving ? "ring-2 ring-accent" : docking ? "ring-2 ring-live" : ""}`}
          onPointerDown={() => { update({ type: "focus", paneId: node.pane.id }); if (session !== undefined) onActive(session.id); }}
          onDragEnter={(event) => dragOverPane(event, node.pane.id)}
          onDragOver={(event) => dragOverPane(event, node.pane.id)}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDockTarget((current) => current?.paneId === node.pane.id ? null : current); }}
          onDrop={(event) => dropPane(event, node.pane.id)}
        >
          {multiple && !compactViewport ? <div data-pane-toolbar className="flex h-7 shrink-0 items-center gap-1 border-b border-line bg-toolbar px-1">
            <button type="button" draggable aria-pressed={moving} aria-label={t(moving ? "workspace.movePanePicked" : "workspace.movePane", { alias: paneLabel })} title={t("workspace.movePane", { alias: paneLabel })} className="flex size-6 shrink-0 cursor-grab items-center justify-center rounded text-xs text-ink-muted hover:bg-select-fill active:cursor-grabbing" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); choosePaneMove(node.pane.id); }} onDragStart={(event) => beginPaneDrag(event, node.pane.id)} onDragEnd={() => setMovingPaneId(null)} onKeyDown={(event) => { if (event.key === "Escape") setMovingPaneId(null); }}><Icon name="movePane" className="size-3.5" /></button>
            <span className="min-w-0 grow truncate text-xs font-medium text-ink-muted">{paneLabel}</span>
            <button type="button" aria-pressed={focusModePaneId === node.pane.id} aria-label={t(focusModePaneId === node.pane.id ? "workspace.exitFocusMode" : "workspace.focusMode", { alias: paneLabel })} title={t(focusModePaneId === node.pane.id ? "workspace.exitFocusMode" : "workspace.focusMode", { alias: paneLabel })} className="flex size-6 shrink-0 items-center justify-center rounded text-xs text-ink-muted hover:bg-select-fill" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); setFocusModePaneId((current) => current === node.pane.id ? null : node.pane.id); }}><Icon name="focus" className="size-3.5" /></button>
            <button type="button" aria-label={t("workspace.detachPane")} title={t("workspace.detachPane")} className="flex size-6 shrink-0 items-center justify-center rounded text-sm text-ink-muted hover:bg-select-fill" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); detachPane(node.pane.id); }}><Icon name="close" className="size-3.5" /></button>
          </div> : null}
          <div className="flex min-h-0 flex-1 flex-col">{session === undefined ? <div className="grid h-full min-h-40 place-items-center text-sm text-ink-muted">{node.pane.state === "failed" ? t("terminal.openFailed") : t("workspace.reconnecting")}</div> : terminal(session)}</div>
          {docking ? <DockPreview edge={dockTarget.edge} /> : null}
        </div>
      );
    }
    const row = node.split.direction === "horizontal";
    const resizeStep = (event: ReactKeyboardEvent<HTMLDivElement>) => {
      const decrease = event.key === (row ? "ArrowLeft" : "ArrowUp");
      const increase = event.key === (row ? "ArrowRight" : "ArrowDown");
      if (!decrease && !increase) return;
      event.preventDefault();
      update({ type: "resize-split", path, ratio: node.split.ratio + (decrease ? -5 : 5) });
    };
    return <div className={`flex h-full min-h-0 min-w-0 flex-1 ${row ? "flex-row" : "flex-col"}`}><div style={{ flexBasis: `${node.split.ratio}%` }} className="flex min-h-0 min-w-0">{renderNode(node.split.first, [...path, "first"])}</div><div role="separator" tabIndex={0} aria-label={t("workspace.resizeSplit")} aria-orientation={row ? "vertical" : "horizontal"} aria-valuemin={10} aria-valuemax={90} aria-valuenow={node.split.ratio} onPointerDown={(event) => beginResize(event, path, node.split.direction)} onKeyDown={resizeStep} className={`shrink-0 touch-none bg-line transition-colors hover:bg-accent focus:bg-accent focus:outline-none ${row ? "w-1 cursor-col-resize" : "h-1 cursor-row-resize"}`} /><div style={{ flexBasis: `${100 - node.split.ratio}%` }} className="flex min-h-0 min-w-0">{renderNode(node.split.second, [...path, "second"])}</div></div>;
  }

  const empty = active === null && visibleLayout === null;
  const focusNode = focusModePaneId === null || visibleLayout === null ? null : findPane(visibleLayout.root, focusModePaneId);
  const compactPane = visibleLayout === null ? null : findPane(visibleLayout.root, visibleLayout.focusedPaneId);
  const displayedNode = compactViewport && compactPane !== null
    ? { pane: compactPane } as RuntimeNode
    : focusNode === null ? visibleLayout?.root ?? null : { pane: focusNode } as RuntimeNode;
  const compactPanes = visibleLayout === null ? [] : paneIDs(visibleLayout.root).map((id) => findPane(visibleLayout.root, id)).filter((pane): pane is RuntimePane => pane !== null);
  return (
    <div className="flex h-full min-h-0 flex-col">
      {workspacePaneCount !== 1 ? (
        <WorkspaceMenu
          displayName={workspaceDisplayName}
          paneCount={workspacePaneCount}
          canBroadcast={connectedCommandTargets > 0}
          onBroadcast={() => setCommandCenter(true)}
          inFocusMode={focusModePaneId !== null}
          onExitFocusMode={() => setFocusModePaneId(null)}
          saved={saved}
          selected={selectedWorkspace}
          onSelect={setSelectedWorkspace}
          canSave={visibleLayout !== null || active !== null}
          onSave={() => setSaveDialogOpen(true)}
          onReopen={(id) => void restoreWorkspace(id)}
          onDelete={(id) => void workspaceApi.remove(id).then(async () => { setSelectedWorkspace(""); setSaved(await workspaceApi.list()); })}
        />
      ) : null}
      {commandCenter && commandTargets.length > 0 ? <WorkspaceCommandCenter paneTargets={commandTargets} onClose={() => setCommandCenter(false)} /> : null}
      {saveDialogOpen ? (
        <InputDialog
          id="workspace-save-heading"
          heading={t("workspace.save")}
          label={t("workspace.namePrompt")}
          initialValue={liveWorkspaceName.trim() || (saved.find((item) => item.id === selectedWorkspace)?.name ?? "")}
          submitLabel={t("workspace.saveConfirm")}
          cancelLabel={t("workspace.cancel")}
          validate={(value) => value === "" ? t("workspace.nameRequired") : ""}
          onSubmit={(value) => {
            setSaveDialogOpen(false);
            void saveWorkspace(value);
          }}
          onCancel={() => setSaveDialogOpen(false)}
        />
      ) : null}
      {problem === "" ? null : <p role="alert" className="bg-notice px-3 py-1 text-xs text-notice-ink">{problem}</p>}
      {compactViewport && compactPanes.length > 1 ? (
        <nav aria-label={t("workspace.mobilePaneSwitcher")} className="flex shrink-0 gap-1 overflow-x-auto border-b border-line bg-toolbar px-2 py-1">
          {compactPanes.map((pane) => (
            <button key={pane.id} type="button" aria-current={pane.id === visibleLayout?.focusedPaneId ? "page" : undefined} className={`max-w-40 shrink-0 truncate rounded px-3 py-1.5 text-xs ${pane.id === visibleLayout?.focusedPaneId ? "bg-select-fill text-ink" : "text-ink-muted"}`} onClick={() => { update({ type: "focus", paneId: pane.id }); if (pane.sessionId !== undefined) onActive(pane.sessionId); }}>{pane.kind === "shell" && pane.sessionId !== undefined ? sessionByID.get(pane.sessionId)?.title ?? pane.alias : pane.alias}</button>
          ))}
        </nav>
      ) : null}
      <div className="flex min-h-0 flex-1 flex-col">
        {empty ? (
          <WorkspaceEmptyState />
        ) : visibleLayout === null ? (active === null ? null : singleTerminal(active)) : displayedNode === null ? null : renderNode(displayedNode)}
      </div>
    </div>
  );
}
