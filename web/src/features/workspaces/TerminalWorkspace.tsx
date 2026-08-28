import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import type { TerminalSession } from "../../api/integrations";
import { failureCode } from "../../api/client";
import { useTranslate, type Translate } from "../../i18n/context";
import { Button } from "../../ui/surface";
import { BrandMark } from "../../ui/BrandMark";
import { Icon } from "../../ui/icons";
import { MAX_WORKSPACE_PANES, paneIDs, paneSessionIDs, reduceLayout, restoreLayout, storeLayout, type DockEdge, type LayoutAction, type LayoutState, type RuntimeNode, type RuntimePane, type SplitDirection, type StoredNode, type StoredPane } from "./layout";
import { workspaceApi, type SavedWorkspace } from "./api";
import { WorkspaceCommandCenter, type WorkspaceCommandTarget } from "./WorkspaceCommandCenter";
import { consoleDragMimeType, type LiveWorkspaceSummary } from "./live";

export type WorkspaceRestoreRequest = { id: string; sequence: number };

function paneID(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return [...bytes].map((value) => value.toString(16).padStart(2, "0")).join("");
}

function visit(root: StoredNode, callback: (pane: StoredPane) => void) {
  if (root.pane !== undefined) { callback(root.pane); return; }
  visit(root.split.first, callback); visit(root.split.second, callback);
}

function paneForSession(session: TerminalSession, id = paneID()): RuntimePane {
  return {
    id,
    alias: session.kind === "ssh" ? session.alias ?? session.title : "localhost",
    ...(session.kind === "shell" ? { kind: "shell" as const } : {}),
    state: session.state === "connected" ? "connected" : session.state === "exited" ? "failed" : "connecting",
    sessionId: session.id,
    ...(session.problem === "" ? {} : { problem: session.problem }),
  };
}

function findPane(root: RuntimeNode, id: string): RuntimePane | null {
  if (root.pane !== undefined) return root.pane.id === id ? root.pane : null;
  return findPane(root.split.first, id) ?? findPane(root.split.second, id);
}

function findPaneBySession(root: RuntimeNode, sessionId: string): RuntimePane | null {
  if (root.pane !== undefined) return root.pane.sessionId === sessionId ? root.pane : null;
  return findPaneBySession(root.split.first, sessionId) ?? findPaneBySession(root.split.second, sessionId);
}

function dockEdge(event: DragEvent<HTMLElement>): DockEdge {
  const bounds = event.currentTarget.getBoundingClientRect();
  const distances: [DockEdge, number][] = [
    ["left", Math.max(0, event.clientX - bounds.left) / Math.max(1, bounds.width)],
    ["right", Math.max(0, bounds.right - event.clientX) / Math.max(1, bounds.width)],
    ["top", Math.max(0, event.clientY - bounds.top) / Math.max(1, bounds.height)],
    ["bottom", Math.max(0, bounds.bottom - event.clientY) / Math.max(1, bounds.height)],
  ];
  distances.sort((left, right) => left[1] - right[1]);
  return distances[0]?.[0] ?? "right";
}

function dockOverlayClass(edge: DockEdge): string {
  switch (edge) {
    case "left": return "inset-y-0 left-0 w-1/2";
    case "right": return "inset-y-0 right-0 w-1/2";
    case "top": return "inset-x-0 top-0 h-1/2";
    case "bottom": return "inset-x-0 bottom-0 h-1/2";
  }
}

function dockLabel(t: Translate, edge: DockEdge): string {
  switch (edge) {
    case "left": return t("workspace.dock.left");
    case "right": return t("workspace.dock.right");
    case "top": return t("workspace.dock.top");
    case "bottom": return t("workspace.dock.bottom");
  }
}

function compactWorkspaceViewport(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia("(max-width: 767px)").matches;
}

export function TerminalWorkspace({
  sessions, activeSessionId, onActive, onOpenAlias, onOpenShell, renderTerminal, restoreRequest = null, onRestoreConsumed = () => undefined,
  onLiveWorkspaceChange = () => undefined,
}: {
  sessions: TerminalSession[];
  activeSessionId: string | null;
  onActive: (id: string) => void;
  onOpenAlias: (alias: string) => Promise<TerminalSession | null>;
  onOpenShell: () => Promise<TerminalSession | null>;
  renderTerminal: (session: TerminalSession) => ReactNode;
  restoreRequest?: WorkspaceRestoreRequest | null;
  onRestoreConsumed?: (sequence: number) => void;
  onLiveWorkspaceChange?: (workspace: LiveWorkspaceSummary | null) => void;
}) {
  const t = useTranslate();
  const [layout, setLayout] = useState<LayoutState | null>(null);
  const [saved, setSaved] = useState<SavedWorkspace[]>([]);
  const [selectedWorkspace, setSelectedWorkspace] = useState("");
  const [commandCenter, setCommandCenter] = useState(false);
  const [focusModePaneId, setFocusModePaneId] = useState<string | null>(null);
  const [movingPaneId, setMovingPaneId] = useState<string | null>(null);
  const [dockTarget, setDockTarget] = useState<{ paneId: string; edge: DockEdge } | null>(null);
  const [compactViewport, setCompactViewport] = useState(compactWorkspaceViewport);
  const [problem, setProblem] = useState("");
  const consumedRestore = useRef(0);
  const liveWorkspaceID = useRef(paneID());
  const active = sessions.find((session) => session.id === activeSessionId) ?? null;
  const sessionByID = useMemo(() => new Map(sessions.map((session) => [session.id, session])), [sessions]);
  const layoutSessionIDs = useMemo(() => layout === null ? [] : paneSessionIDs(layout.root), [layout]);
  const showingWorkspace = layout !== null && (activeSessionId === null || layoutSessionIDs.includes(activeSessionId));
  const visibleLayout = showingWorkspace ? layout : null;
  const commandTargets = useMemo<WorkspaceCommandTarget[]>(() => {
    if (visibleLayout !== null) return paneIDs(visibleLayout.root).map((id, index) => {
      const pane = findPane(visibleLayout.root, id);
      if (pane === null) throw new Error("workspace pane disappeared");
      const session = pane.sessionId === undefined ? undefined : sessionByID.get(pane.sessionId);
      const connected = session?.state === "connected";
      return {
        targetId: pane.id,
        ...(session === undefined ? {} : { sessionId: session.id }),
        alias: pane.alias,
        title: session?.title ?? pane.alias,
        paneNumber: index + 1,
        connected,
        state: session?.state ?? pane.state,
      };
    });
    if (active === null) return [];
    return [{
      targetId: active.id,
      sessionId: active.id,
      alias: active.kind === "ssh" ? active.alias ?? active.title : "localhost",
      title: active.title,
      paneNumber: 1,
      connected: active.state === "connected",
      state: active.state,
    }];
  }, [active, sessionByID, visibleLayout]);
  const connectedCommandTargets = commandTargets.filter((target) => target.connected).length;
  const workspacePaneCount = visibleLayout === null ? (active === null ? 0 : 1) : paneIDs(visibleLayout.root).length;
  const workspaceDisplayName = useMemo(() => {
    if (visibleLayout === null) return active?.title ?? t("workspace.live");
    const selectedName = saved.find((item) => item.id === selectedWorkspace)?.name.trim() ?? "";
    if (selectedName !== "") return selectedName;
    const aliases: string[] = [];
    visit(storeLayout(visibleLayout).layout, (pane) => aliases.push(pane.alias));
    const uniqueAliases = [...new Set(aliases)];
    return uniqueAliases.slice(0, 2).join(" + ") + (uniqueAliases.length > 2 ? ` +${uniqueAliases.length - 2}` : "") || t("workspace.live");
  }, [active?.title, saved, selectedWorkspace, t, visibleLayout]);

  useEffect(() => { void workspaceApi.list().then(setSaved).catch(() => undefined); }, []);
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
    const aliases: string[] = [];
    visit(storeLayout(layout).layout, (pane) => aliases.push(pane.alias));
    const selectedName = saved.find((item) => item.id === selectedWorkspace)?.name.trim() ?? "";
    const uniqueAliases = [...new Set(aliases)];
    const automaticName = uniqueAliases.slice(0, 2).join(" + ") + (uniqueAliases.length > 2 ? ` +${uniqueAliases.length - 2}` : "");
    const focusedSessionId = findPane(layout.root, layout.focusedPaneId)?.sessionId ?? memberSessionIds[0] ?? "";
    onLiveWorkspaceChange({
      id: liveWorkspaceID.current,
      name: selectedName || automaticName || t("workspace.live"),
      memberSessionIds,
      focusedSessionId,
    });
  }, [layout, onLiveWorkspaceChange, saved, selectedWorkspace, t]);
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
    if (typeof window.matchMedia !== "function") return;
    const media = window.matchMedia("(max-width: 767px)");
    const updateViewport = () => setCompactViewport(media.matches);
    updateViewport();
    media.addEventListener("change", updateViewport);
    return () => media.removeEventListener("change", updateViewport);
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
      const targetPane = paneForSession(targetSession, targetPaneId);
      let next = restoreLayout({ pane: {
        id: targetPane.id,
        alias: targetPane.alias,
        ...(targetPane.kind === undefined ? {} : { kind: targetPane.kind }),
      } }, targetPaneId);
      next = reduceLayout(next, { type: "connection-started", paneId: targetPaneId, sessionId: targetSession.id });
      next = reduceLayout(next, {
        type: "dock-pane",
        targetPaneId,
        edge,
        pane: paneForSession(source),
      });
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

  async function saveWorkspace() {
    let effective = visibleLayout;
    if (effective === null && active !== null) {
      const id = paneID();
      const pane = paneForSession(active, id);
      effective = reduceLayout(restoreLayout({ pane: {
        id: pane.id,
        alias: pane.alias,
        ...(pane.kind === undefined ? {} : { kind: pane.kind }),
      } }, id), { type: "connection-started", paneId: id, sessionId: active.id });
    }
    if (effective === null) return;
    const name = window.prompt(t("workspace.namePrompt"), saved.find((item) => item.id === selectedWorkspace)?.name ?? "")?.trim() ?? "";
    if (name === "") return;
    try {
      const stored = storeLayout(effective);
      const value = selectedWorkspace === "" ? await workspaceApi.create({ name, ...stored }) : await workspaceApi.update(selectedWorkspace, { name, ...stored });
      setSelectedWorkspace(value.id); setSaved(await workspaceApi.list()); setProblem("");
    } catch (error) { setProblem(failureCode(error) || "workspace_failed"); }
  }

  const restoreWorkspace = useCallback(async (id: string) => {
    if (id === "") return;
    try {
      const stored = await workspaceApi.restore(id);
      setSelectedWorkspace(id);
      setFocusModePaneId(null);
      let restored = restoreLayout(stored.layout, stored.focusedPaneId);
      const panes: { id: string; alias: string; kind?: "shell" }[] = [];
      visit(stored.layout, (pane) => {
        panes.push(pane);
        restored = reduceLayout(restored, { type: "connection-starting", paneId: pane.id });
      });
      setLayout(restored);
      await Promise.all(panes.map(async (pane) => {
        const session = pane.kind === "shell" ? await onOpenShell() : await onOpenAlias(pane.alias);
        if (session === null) {
          setLayout((current) => current === null ? current : reduceLayout(current, {
            type: "connection-failed", paneId: pane.id, problem: "open_failed",
          }));
          return;
        }
        setLayout((current) => current === null ? current : reduceLayout(current, {
          type: "connection-started", paneId: pane.id, sessionId: session.id,
        }));
        if (pane.id === stored.focusedPaneId) onActive(session.id);
      }));
      setProblem("");
    } catch (error) { setProblem(failureCode(error) || "workspace_failed"); }
  }, [onActive, onOpenAlias, onOpenShell]);

  useEffect(() => {
    if (restoreRequest === null || restoreRequest.sequence <= consumedRestore.current) return;
    consumedRestore.current = restoreRequest.sequence;
    onRestoreConsumed(restoreRequest.sequence);
    void restoreWorkspace(restoreRequest.id);
  }, [onRestoreConsumed, restoreRequest, restoreWorkspace]);

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
      {docking ? <div data-dock-preview={dockTarget.edge} className={`pointer-events-none absolute z-10 grid place-items-center border-2 border-accent bg-accent/20 ${dockOverlayClass(dockTarget.edge)}`}><span className="rounded bg-toolbar px-3 py-2 text-xs font-medium shadow">{dockLabel(t, dockTarget.edge)}</span></div> : null}
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
          {multiple && !compactViewport ? <div data-pane-toolbar className="flex h-8 shrink-0 items-center gap-1 border-b border-line bg-toolbar px-1.5">
            <button type="button" draggable aria-pressed={moving} aria-label={t(moving ? "workspace.movePanePicked" : "workspace.movePane", { alias: paneLabel })} title={t("workspace.movePane", { alias: paneLabel })} className="flex size-6 shrink-0 cursor-grab items-center justify-center rounded text-xs text-ink-muted hover:bg-select-fill active:cursor-grabbing" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); choosePaneMove(node.pane.id); }} onDragStart={(event) => beginPaneDrag(event, node.pane.id)} onDragEnd={() => setMovingPaneId(null)} onKeyDown={(event) => { if (event.key === "Escape") setMovingPaneId(null); }}><Icon name="movePane" className="size-3.5" /></button>
            <span className="min-w-0 grow truncate text-xs font-medium text-ink-muted">{paneLabel}</span>
            <button type="button" aria-pressed={focusModePaneId === node.pane.id} aria-label={t(focusModePaneId === node.pane.id ? "workspace.exitFocusMode" : "workspace.focusMode", { alias: paneLabel })} title={t(focusModePaneId === node.pane.id ? "workspace.exitFocusMode" : "workspace.focusMode", { alias: paneLabel })} className="flex size-6 shrink-0 items-center justify-center rounded text-xs text-ink-muted hover:bg-select-fill" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); setFocusModePaneId((current) => current === node.pane.id ? null : node.pane.id); }}><Icon name="focus" className="size-3.5" /></button>
            <button type="button" aria-label={t("workspace.detachPane")} title={t("workspace.detachPane")} className="flex size-6 shrink-0 items-center justify-center rounded text-sm text-ink-muted hover:bg-select-fill" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); detachPane(node.pane.id); }}><Icon name="close" className="size-3.5" /></button>
          </div> : null}
          <div className="flex min-h-0 flex-1 flex-col">{session === undefined ? <div className="grid h-full min-h-40 place-items-center text-sm text-ink-muted">{node.pane.state === "failed" ? t("terminal.openFailed") : t("workspace.reconnecting")}</div> : terminal(session)}</div>
          {docking ? <div data-dock-preview={dockTarget.edge} className={`pointer-events-none absolute z-10 grid place-items-center border-2 border-accent bg-accent/20 ${dockOverlayClass(dockTarget.edge)}`}><span className="rounded bg-toolbar px-3 py-2 text-xs font-medium shadow">{dockLabel(t, dockTarget.edge)}</span></div> : null}
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
      {workspacePaneCount !== 1 ? <div data-desktop-workspace-controls className="hidden shrink-0 items-center gap-2 border-b border-line bg-toolbar px-3 py-2 md:flex">
        {workspacePaneCount > 0 ? (
          <div className="mr-2 flex min-w-0 items-center gap-2 border-r border-line pr-4">
            <span className="max-w-52 truncate text-xs font-semibold text-ink">{workspaceDisplayName}</span>
            <span className="whitespace-nowrap text-[11px] text-ink-subtle">{t("workspace.groupCount", { count: String(workspacePaneCount) })}</span>
          </div>
        ) : null}
        <Button disabled={connectedCommandTargets === 0} onClick={() => setCommandCenter(true)}>{t("workspace.broadcastCommand")}</Button>
        {focusModePaneId === null ? null : <Button onClick={() => setFocusModePaneId(null)}>{t("workspace.exitFocusMode")}</Button>}
        <details className="group relative ml-auto">
          <summary className="flex cursor-pointer list-none items-center gap-2 rounded-md border border-control-line bg-control px-3 py-1.5 text-xs text-ink marker:hidden hover:bg-select-fill">
            <span aria-hidden="true" className="text-ink-faint group-open:rotate-90">›</span>
            {t("workspace.savedLayouts")}
          </summary>
          <div className="absolute right-0 top-[calc(100%+0.5rem)] z-30 w-80 rounded border border-control-line bg-card p-3 shadow-xl">
            <h2 className="text-sm font-semibold text-ink">{t("workspace.savedLayouts")}</h2>
            <p className="mt-1 text-xs leading-5 text-ink-muted">{t("workspace.savedDescription", { count: MAX_WORKSPACE_PANES })}</p>
            <select aria-label={t("workspace.saved")} value={selectedWorkspace} onChange={(event) => setSelectedWorkspace(event.target.value)} className="mt-3 w-full rounded border border-control-line bg-control px-2 py-1.5 text-xs">
              <option value="">{t("workspace.new")}</option>
              {saved.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
            </select>
            <div className="mt-3 flex flex-wrap gap-2">
              <Button disabled={visibleLayout === null && active === null} onClick={() => void saveWorkspace()}>{t("workspace.save")}</Button>
              <Button disabled={selectedWorkspace === ""} onClick={() => void restoreWorkspace(selectedWorkspace)}>{t("workspace.reopen")}</Button>
              <button disabled={selectedWorkspace === ""} className="px-2 text-xs text-danger disabled:opacity-40" onClick={() => void workspaceApi.remove(selectedWorkspace).then(async () => { setSelectedWorkspace(""); setSaved(await workspaceApi.list()); })}>{t("workspace.delete")}</button>
            </div>
          </div>
        </details>
      </div> : null}
      {commandCenter && commandTargets.length > 0 ? <WorkspaceCommandCenter paneTargets={commandTargets} onClose={() => setCommandCenter(false)} /> : null}
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
          <div className="flex h-full items-center justify-center p-6">
            <section className="w-full max-w-md border-y border-line bg-surface-subtle p-6 text-center" role="status">
              <BrandMark className="mx-auto size-10" />
              <h2 className="mt-4 text-lg font-semibold tracking-tight text-ink">{t("terminal.emptyHeading")}</h2>
              <p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-ink-muted">{t("terminal.emptyHint")}</p>
              <div aria-hidden="true" className="mx-auto mt-4 max-w-xs rounded bg-term-bg px-4 py-3 text-left font-mono text-xs text-ink"><span className="text-live">$</span> sshc host<span className="ml-1 inline-block h-3 w-1.5 translate-y-0.5 bg-ink" /></div>
            </section>
          </div>
        ) : visibleLayout === null ? (active === null ? null : singleTerminal(active)) : displayedNode === null ? null : renderNode(displayedNode)}
      </div>
    </div>
  );
}
