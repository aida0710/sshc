import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { useTranslate } from "../../i18n/context";
import {
  MAX_WORKSPACE_PANES,
  paneIds,
  paneSessionIds,
  reduceLayout,
  storeLayout,
  type LayoutAction,
  type LayoutState,
  type RuntimeNode,
  type RuntimePane,
} from "./layout";
import { workspaceApi, type SavedWorkspace } from "./api";
import { WorkspaceCommandCenter } from "./WorkspaceCommandCenter";
import type { LiveWorkspaceSummary } from "./live";
import { mobileViewportQuery, useMediaQuery } from "../../ui/useMediaQuery";
import { SplitResizeHandle } from "../../ui/SplitResizeHandle";
import {
  automaticWorkspaceName,
  dockedPairLayout,
  findPane,
  findPaneBySession,
  paneForSession,
  singlePaneLayout,
  withoutClosedSessions,
} from "./panes";
import { newIdentifier } from "../../ui/randomIdentifier";
import { commandTargetsFor } from "./commandTargets";
import { DockPreview } from "./DockPreview";
import { WorkspaceEmptyState } from "./WorkspaceEmptyState";
import { WorkspaceMenu } from "./WorkspaceMenu";
import { DeleteWorkspaceDialog, SaveWorkspaceDialog } from "./SavedWorkspaceDialogs";
import { WorkspacePaneSwitcher } from "./WorkspacePaneSwitcher";
import { WorkspacePaneToolbar } from "./WorkspacePaneToolbar";
import { useSavedWorkspaces } from "./useSavedWorkspaces";
import { usePaneDrag, type DockRequest } from "./usePaneDrag";
import { useLiveWorkspacePersistence, type RestoredLiveWorkspace } from "./useLiveWorkspacePersistence";
import { useWorkspaceRestore, type WorkspaceRestoreRequest } from "./useWorkspaceRestore";
import { describeWorkspaceFailure } from "./workspaceProblem";
import { Notice } from "../../ui/surface";

export type { WorkspaceRestoreRequest } from "./useWorkspaceRestore";
export type WorkspaceRenameRequest = { name: string; sequence: number };

export function TerminalWorkspace({
  sessions,
  activeSessionId,
  onActive,
  onOpenAlias,
  onOpenShell,
  onClose,
  renderTerminal,
  restoreRequest = null,
  onRestoreConsumed = () => undefined,
  onRestoringChange = () => undefined,
  renameRequest = null,
  onRenameConsumed = () => undefined,
  onLiveWorkspaceChange = () => undefined,
  sessionsLoaded = true,
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
  onRestoringChange?: (restoring: boolean) => void;
  renameRequest?: WorkspaceRenameRequest | null;
  onRenameConsumed?: (sequence: number) => void;
  onLiveWorkspaceChange?: (workspace: LiveWorkspaceSummary | null) => void;
  sessionsLoaded?: boolean;
}) {
  const t = useTranslate();
  const [layout, setLayout] = useState<LayoutState | null>(null);
  const { saved, loadFailed: savedLoadFailed, reload: loadSaved } = useSavedWorkspaces();
  const [selectedWorkspace, setSelectedWorkspace] = useState("");
  const [commandCenter, setCommandCenter] = useState(false);
  const [focusModePaneId, setFocusModePaneId] = useState<string | null>(null);
  const compactViewport = useMediaQuery(mobileViewportQuery);
  const [problem, setProblem] = useState("");
  const [liveWorkspaceName, setLiveWorkspaceName] = useState("");
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const [deletingWorkspace, setDeletingWorkspace] = useState<SavedWorkspace | null>(null);
  const consumedRename = useRef(0);
  const liveWorkspaceId = useRef(newIdentifier());
  const beginRestore = useCallback((id: string) => {
    setSelectedWorkspace(id);
    setLiveWorkspaceName("");
    setFocusModePaneId(null);
  }, []);
  const restoreWorkspace = useWorkspaceRestore({
    restoreRequest, onRestoreConsumed, onRestoringChange, onOpenAlias, onOpenShell, onClose, onActive,
    onBegin: beginRestore, setLayout, report: setProblem,
  });
  const active = sessions.find((session) => session.id === activeSessionId) ?? null;
  const sessionById = useMemo(() => new Map(sessions.map((session) => [session.id, session])), [sessions]);
  const layoutSessionIds = useMemo(() => layout === null ? [] : paneSessionIds(layout.root), [layout]);
  const showingWorkspace = layout !== null && (activeSessionId === null || layoutSessionIds.includes(activeSessionId));
  const visibleLayout = showingWorkspace ? layout : null;
  const commandTargets = useMemo(() => commandTargetsFor(visibleLayout, active, sessionById), [active, sessionById, visibleLayout]);
  const connectedCommandTargets = commandTargets.filter((target) => target.connected).length;
  const workspacePaneCount = visibleLayout === null ? (active === null ? 0 : 1) : paneIds(visibleLayout.root).length;
  const selectedWorkspaceName = saved.find((item) => item.id === selectedWorkspace)?.name ?? "";
  // A split is called by the name given to it, else by the saved layout it
  // came from, else by its panes.
  const nameLayout = useCallback(
    (target: LayoutState) =>
      liveWorkspaceName.trim() || selectedWorkspaceName.trim() || automaticWorkspaceName(target) || t("workspace.live"),
    [liveWorkspaceName, selectedWorkspaceName, t],
  );
  const workspaceDisplayName = visibleLayout === null ? active?.title ?? t("workspace.live") : nameLayout(visibleLayout);

  useEffect(() => {
    document.title = active === null ? "sshc" : `${active.presentation?.displayTitle ?? active.title} · sshc`;
    return () => { document.title = "sshc"; };
  }, [active]);
  const restoreLive = useCallback((restored: RestoredLiveWorkspace) => {
    setLayout(restored.layout);
    setFocusModePaneId(restored.focusModePaneId);
    setLiveWorkspaceName(restored.name);
    if (restored.focusedSessionId !== undefined) onActive(restored.focusedSessionId);
  }, [onActive]);
  const liveRestoreReady = useLiveWorkspacePersistence({
    sessionsLoaded,
    restoreRequest,
    sessionById,
    layout,
    focusModePaneId,
    name: liveWorkspaceName,
    onRestored: restoreLive,
  });
  useEffect(() => {
    if (layout === null && liveRestoreReady) setLiveWorkspaceName("");
  }, [layout, liveRestoreReady]);
  useEffect(() => {
    if (focusModePaneId === null) return;
    if (layout === null || !paneIds(layout.root).includes(focusModePaneId)) setFocusModePaneId(null);
  }, [focusModePaneId, layout]);
  useEffect(() => {
    if (layout === null) {
      onLiveWorkspaceChange(null);
      return;
    }
    const memberSessionIds = paneSessionIds(layout.root);
    if (memberSessionIds.length < 2) {
      onLiveWorkspaceChange(null);
      return;
    }
    const focusedSessionId = findPane(layout.root, layout.focusedPaneId)?.sessionId ?? memberSessionIds[0] ?? "";
    onLiveWorkspaceChange({
      id: liveWorkspaceId.current,
      name: nameLayout(layout),
      memberSessionIds,
      focusedSessionId,
    });
  }, [layout, nameLayout, onLiveWorkspaceChange]);
  useEffect(() => {
    // The session list keeps every session from the moment its open returns
    // until it is closed (useTerminalSessions), so a pane of this render whose
    // session this render does not list belongs to a closed session. Only
    // those are closed: by the time this runs, the layout may already hold a
    // pane for a session opened later, whose listing has not rendered yet.
    const listed = new Set(sessions.map((session) => session.id));
    const closed = new Set(layoutSessionIds.filter((id) => !listed.has(id)));
    if (closed.size === 0) return;
    setLayout((current) => current === null ? null : withoutClosedSessions(current, closed));
  }, [layoutSessionIds, sessions]);
  useEffect(() => {
    const exit = (event: KeyboardEvent) => { if (event.key === "Escape") setFocusModePaneId(null); };
    window.addEventListener("keydown", exit);
    return () => window.removeEventListener("keydown", exit);
  }, []);
  useEffect(() => {
    if (layout === null || activeSessionId === null || !layoutSessionIds.includes(activeSessionId)) return;
    const pane = findPaneBySession(layout.root, activeSessionId);
    if (pane !== null && pane.id !== layout.focusedPaneId) {
      setLayout((current) => current === null ? current : reduceLayout(current, { type: "focus", paneId: pane.id }));
    }
  }, [activeSessionId, layout, layoutSessionIds]);

  function update(action: LayoutAction) { setLayout((current) => current === null ? current : reduceLayout(current, action)); }

  const paneDrag = usePaneDrag({
    layout,
    onSwap: (sourcePaneId, targetPaneId) => update({ type: "swap-panes", sourcePaneId, targetPaneId }),
    onDock: dockSession,
  });
  const { movingPaneId, dockTarget } = paneDrag;

  function dockSession({ sourceSessionId, targetPaneId, edge, targetSession }: DockRequest) {
    const source = sessionById.get(sourceSessionId);
    if (source === undefined) return;
    if (targetSession !== undefined) {
      if (targetSession.id === source.id) return;
      if (layout !== null) {
        setProblem(t("workspace.oneLiveOnly"));
        return;
      }
      setSelectedWorkspace("");
      setLiveWorkspaceName("");
      setLayout(dockedPairLayout(targetSession, source, edge));
      setProblem("");
      onActive(source.id);
      return;
    }
    if (layout === null || !showingWorkspace) return;
    const existing = findPaneBySession(layout.root, source.id);
    if (existing === null && paneIds(layout.root).length >= MAX_WORKSPACE_PANES) {
      setProblem(t("workspace.maxPanes", { count: MAX_WORKSPACE_PANES }));
      return;
    }
    const pane: RuntimePane = existing ?? paneForSession(source);
    setLayout((current) => current === null ? current : reduceLayout(current, { type: "dock-pane", targetPaneId, edge, pane }));
    setProblem("");
    onActive(source.id);
  }

  function detachPane(paneId: string) {
    if (layout === null) return;
    if (paneIds(layout.root).length <= 2) {
      setLayout(null);
      setFocusModePaneId(null);
      setSelectedWorkspace("");
      setLiveWorkspaceName("");
      return;
    }
    update({ type: "close", paneId });
  }

  async function saveWorkspace(name: string) {
    const effective = visibleLayout ?? (active === null ? null : singlePaneLayout(active));
    if (effective === null) return;
    try {
      const stored = storeLayout(effective);
      const value = selectedWorkspace === ""
        ? await workspaceApi.create({ name, ...stored })
        : await workspaceApi.update(selectedWorkspace, { name, ...stored });
      setSelectedWorkspace(value.id);
      setLiveWorkspaceName(value.name);
      setProblem("");
    } catch (error) {
      setProblem(describeWorkspaceFailure(t, error));
      return;
    }
    await loadSaved();
  }

  async function deleteWorkspace(id: string) {
    try {
      await workspaceApi.remove(id);
      setSelectedWorkspace("");
      setProblem("");
    } catch (error) {
      setProblem(describeWorkspaceFailure(t, error));
      return;
    }
    await loadSaved();
  }

  useEffect(() => {
    if (renameRequest === null || renameRequest.sequence <= consumedRename.current) return;
    consumedRename.current = renameRequest.sequence;
    onRenameConsumed(renameRequest.sequence);
    const name = renameRequest.name.trim();
    if (layout !== null && name !== "") setLiveWorkspaceName(name);
  }, [layout, onRenameConsumed, renameRequest]);

  // A shell pane is named by its session's title; an SSH pane by its alias,
  // which it keeps while its session is being reopened.
  function paneLabel(pane: RuntimePane): string {
    if (pane.kind !== "shell" || pane.sessionId === undefined) return pane.alias;
    return sessionById.get(pane.sessionId)?.title ?? pane.alias;
  }

  function switchToPane(pane: RuntimePane) {
    update({ type: "focus", paneId: pane.id });
    if (pane.sessionId !== undefined) onActive(pane.sessionId);
  }

  function singleTerminal(session: TerminalSession) {
    const docking = dockTarget?.paneId === session.id;
    return (
      <div
        data-single-terminal-drop-target={session.id}
        className={`relative flex h-full min-h-0 flex-col ${docking ? "ring-2 ring-inset ring-live" : ""}`}
        onDragEnter={(event) => paneDrag.dragOverSingle(event, session)}
        onDragOver={(event) => paneDrag.dragOverSingle(event, session)}
        onDragLeave={paneDrag.leaveSingle}
        onDrop={(event) => paneDrag.dropOnSingle(event, session)}
      >
        {renderTerminal(session)}
        {docking ? <DockPreview edge={dockTarget.edge} /> : null}
      </div>
    );
  }

  function renderNode(node: RuntimeNode, path: ("first" | "second")[] = []): ReactNode {
    if (node.pane !== undefined) {
      const pane = node.pane;
      const session = pane.sessionId === undefined ? undefined : sessionById.get(pane.sessionId);
      const multiple = layout !== null && paneIds(layout.root).length > 1;
      const moving = movingPaneId === pane.id;
      const docking = dockTarget?.paneId === pane.id && !moving;
      const border = layout?.focusedPaneId === pane.id ? "border-accent" : "border-line";
      const ring = moving ? "ring-2 ring-accent" : docking ? "ring-2 ring-live" : "";
      return (
        <div
          key={pane.id}
          data-workspace-pane={pane.id}
          data-pane-alias={pane.alias}
          className={`relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden border ${border} ${ring}`}
          onPointerDown={() => {
            update({ type: "focus", paneId: pane.id });
            if (session !== undefined) onActive(session.id);
          }}
          onDragEnter={(event) => paneDrag.dragOverPane(event, pane.id)}
          onDragOver={(event) => paneDrag.dragOverPane(event, pane.id)}
          onDragLeave={(event) => paneDrag.leavePane(event, pane.id)}
          onDrop={(event) => paneDrag.dropOnPane(event, pane.id)}
        >
          {multiple && !compactViewport ? (
            <WorkspacePaneToolbar
              paneLabel={paneLabel(pane)}
              moving={moving}
              inFocusMode={focusModePaneId === pane.id}
              onPickToMove={() => paneDrag.pickToMove(pane.id)}
              onMoveDragStart={(event) => paneDrag.beginMoveDrag(event, pane.id)}
              onCancelMove={paneDrag.cancelMove}
              onToggleFocusMode={() => setFocusModePaneId((current) => current === pane.id ? null : pane.id)}
              onDetach={() => detachPane(pane.id)}
            />
          ) : null}
          <div className="flex min-h-0 flex-1 flex-col">
            {session === undefined ? (
              <div className="grid h-full min-h-40 place-items-center text-sm text-ink-muted">
                {pane.state === "failed" ? t("terminal.openFailed") : t("workspace.reconnecting")}
              </div>
            ) : renderTerminal(session)}
          </div>
          {docking ? <DockPreview edge={dockTarget.edge} /> : null}
        </div>
      );
    }
    const { split } = node;
    return (
      <div className={`flex h-full min-h-0 min-w-0 flex-1 ${split.direction === "horizontal" ? "flex-row" : "flex-col"}`}>
        <div style={{ flexBasis: `${split.ratio}%` }} className="flex min-h-0 min-w-0">
          {renderNode(split.first, [...path, "first"])}
        </div>
        <SplitResizeHandle
          direction={split.direction}
          ratio={split.ratio}
          label={t("workspace.resizeSplit")}
          onRatioChange={(ratio) => update({ type: "resize-split", path, ratio })}
        />
        <div style={{ flexBasis: `${100 - split.ratio}%` }} className="flex min-h-0 min-w-0">
          {renderNode(split.second, [...path, "second"])}
        </div>
      </div>
    );
  }

  const empty = active === null && visibleLayout === null;
  const focusNode = focusModePaneId === null || visibleLayout === null ? null : findPane(visibleLayout.root, focusModePaneId);
  const compactPane = visibleLayout === null ? null : findPane(visibleLayout.root, visibleLayout.focusedPaneId);
  const displayedNode = compactViewport && compactPane !== null
    ? { pane: compactPane } as RuntimeNode
    : focusNode === null ? visibleLayout?.root ?? null : { pane: focusNode } as RuntimeNode;
  const compactPanes = visibleLayout === null
    ? []
    : paneIds(visibleLayout.root)
      .map((id) => findPane(visibleLayout.root, id))
      .filter((pane): pane is RuntimePane => pane !== null);
  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* 管理操作はdesktop向け。1つのターミナルのときとスマホでは出さない。 */}
      {workspacePaneCount !== 1 && !compactViewport ? (
        <WorkspaceMenu
          displayName={workspaceDisplayName}
          paneCount={workspacePaneCount}
          canBroadcast={connectedCommandTargets > 0}
          onBroadcast={() => setCommandCenter(true)}
          inFocusMode={focusModePaneId !== null}
          onExitFocusMode={() => setFocusModePaneId(null)}
          saved={saved}
          savedLoadFailed={savedLoadFailed}
          onReloadSaved={() => void loadSaved()}
          selected={selectedWorkspace}
          onSelect={setSelectedWorkspace}
          canSave={visibleLayout !== null || active !== null}
          onSave={() => setSaveDialogOpen(true)}
          onReopen={(id) => void restoreWorkspace(id)}
          onDelete={(id) => setDeletingWorkspace(saved.find((item) => item.id === id) ?? null)}
        />
      ) : null}
      {commandCenter && commandTargets.length > 0 ? (
        <WorkspaceCommandCenter paneTargets={commandTargets} onClose={() => setCommandCenter(false)} />
      ) : null}
      {saveDialogOpen ? (
        <SaveWorkspaceDialog
          initialName={liveWorkspaceName.trim() || selectedWorkspaceName}
          onSave={(name) => {
            setSaveDialogOpen(false);
            void saveWorkspace(name);
          }}
          onCancel={() => setSaveDialogOpen(false)}
        />
      ) : null}
      {deletingWorkspace === null ? null : (
        <DeleteWorkspaceDialog
          workspace={deletingWorkspace}
          onCancel={() => setDeletingWorkspace(null)}
          onDelete={() => {
            setDeletingWorkspace(null);
            void deleteWorkspace(deletingWorkspace.id);
          }}
        />
      )}
      {problem === "" ? null : <Notice tone="danger" compact>{problem}</Notice>}
      {compactViewport && compactPanes.length > 1 ? (
        <WorkspacePaneSwitcher
          panes={compactPanes}
          focusedPaneId={visibleLayout?.focusedPaneId}
          paneLabel={paneLabel}
          onFocus={switchToPane}
        />
      ) : null}
      <div className="flex min-h-0 flex-1 flex-col">
        {empty ? (
          <WorkspaceEmptyState />
        ) : visibleLayout === null ? (
          active === null ? null : singleTerminal(active)
        ) : displayedNode === null ? null : renderNode(displayedNode)}
      </div>
    </div>
  );
}
