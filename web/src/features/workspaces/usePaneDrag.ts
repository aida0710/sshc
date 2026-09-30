import { useEffect, useState, type DragEvent } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { dockEdge } from "./DockPreview";
import { paneIds, type DockEdge, type LayoutState } from "./layout";
import { sessionDragMimeType } from "./live";
import { findPane } from "./panes";

export type DockRequest = {
  sourceSessionId: string;
  targetPaneId: string;
  edge: DockEdge;
  // Set when the target is a terminal shown on its own, which docking turns
  // into a new workspace.
  targetSession?: TerminalSession;
};

type PaneDragOptions = {
  layout: LayoutState | null;
  onSwap: (sourcePaneId: string, targetPaneId: string) => void;
  onDock: (request: DockRequest) => void;
};

// usePaneDrag holds the pane being moved and the edge a drag would dock on.
// A pane is moved either by dragging its handle or, from the keyboard, by
// picking it and then the pane to swap with. A session dragged from the
// session list docks on the edge of the pane it is dropped on.
export function usePaneDrag({ layout, onSwap, onDock }: PaneDragOptions) {
  const [movingPaneId, setMovingPaneId] = useState<string | null>(null);
  const [dockTarget, setDockTarget] = useState<{ paneId: string; edge: DockEdge } | null>(null);

  useEffect(() => {
    if (movingPaneId === null) return;
    if (layout === null || !paneIds(layout.root).includes(movingPaneId)) {
      setMovingPaneId(null);
    }
  }, [layout, movingPaneId]);

  function cancelMove() {
    setMovingPaneId(null);
  }

  function pickToMove(paneId: string) {
    if (movingPaneId === null) {
      setMovingPaneId(paneId);
      return;
    }
    if (movingPaneId !== paneId) onSwap(movingPaneId, paneId);
    setMovingPaneId(null);
  }

  function beginMoveDrag(event: DragEvent<HTMLButtonElement>, paneId: string) {
    event.stopPropagation();
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", paneId);
    const sessionId = layout === null ? undefined : findPane(layout.root, paneId)?.sessionId;
    if (sessionId !== undefined) event.dataTransfer.setData(sessionDragMimeType, sessionId);
    setMovingPaneId(paneId);
  }

  function dragOverPane(event: DragEvent<HTMLDivElement>, paneId: string) {
    const acceptsSession = event.dataTransfer.types?.includes(sessionDragMimeType) === true || movingPaneId !== null;
    if (!acceptsSession) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = "move";
    setDockTarget({ paneId, edge: dockEdge(event) });
  }

  // Leaving for a child of the same pane is not leaving the pane.
  function leavePane(event: DragEvent<HTMLDivElement>, paneId: string) {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
    setDockTarget((current) => current?.paneId === paneId ? null : current);
  }

  function dropOnPane(event: DragEvent<HTMLDivElement>, paneId: string) {
    event.preventDefault();
    event.stopPropagation();
    const movingSessionId = movingPaneId === null || layout === null
      ? ""
      : findPane(layout.root, movingPaneId)?.sessionId ?? "";
    const sourceSessionId = event.dataTransfer.getData(sessionDragMimeType) || movingSessionId;
    const edge = dockTarget?.paneId === paneId ? dockTarget.edge : dockEdge(event);
    if (sourceSessionId !== "") onDock({ sourceSessionId, targetPaneId: paneId, edge });
    setMovingPaneId(null);
    setDockTarget(null);
  }

  // A terminal shown on its own only takes sessions dragged from the list.
  function dragOverSingle(event: DragEvent<HTMLDivElement>, session: TerminalSession) {
    if (event.dataTransfer.types?.includes(sessionDragMimeType) !== true) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setDockTarget({ paneId: session.id, edge: dockEdge(event) });
  }

  function leaveSingle(event: DragEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDockTarget(null);
  }

  function dropOnSingle(event: DragEvent<HTMLDivElement>, session: TerminalSession) {
    event.preventDefault();
    event.stopPropagation();
    const sourceSessionId = event.dataTransfer.getData(sessionDragMimeType);
    if (sourceSessionId !== "") {
      onDock({ sourceSessionId, targetPaneId: session.id, edge: dockEdge(event), targetSession: session });
    }
    setDockTarget(null);
  }

  return {
    movingPaneId,
    dockTarget,
    cancelMove,
    pickToMove,
    beginMoveDrag,
    dragOverPane,
    leavePane,
    dropOnPane,
    dragOverSingle,
    leaveSingle,
    dropOnSingle,
  };
}
