import { useEffect, useState, type DragEvent } from "react";
import { sessionDragMimeType } from "../sessionDrag";
import { nearestPaneEdge } from "./geometry";
import { maximumSplitViewPanes, paneSessionIds, type PaneEdge } from "./layout";
import type { SplitViewController } from "./useSplitView";

// Pane handles swap positions; sidebar sessions split at the nearest edge.
const paneDragMimeType = "application/x-sshc-split-view-pane";
type DropTarget = { sessionId: string; edge: PaneEdge | null };

export function usePaneDrag(splitView: SplitViewController) {
  const [moving, setMoving] = useState<string | null>(null);
  const [target, setTarget] = useState<DropTarget | null>(null);
  const members = paneSessionIds(splitView.state.root);
  const movingSession = moving !== null && members.includes(moving) ? moving : null;
  function clear() { setMoving(null); setTarget(null); }
  useEffect(() => {
    window.addEventListener("dragend", clear);
    window.addEventListener("drop", clear);
    return () => { window.removeEventListener("dragend", clear); window.removeEventListener("drop", clear); };
  }, []);
  function edgeAt(event: DragEvent): PaneEdge {
    const bounds = event.currentTarget.getBoundingClientRect();
    return nearestPaneEdge({ x: event.clientX - bounds.left, y: event.clientY - bounds.top }, bounds);
  }
  function accepts(event: DragEvent): boolean {
    return !splitView.compact && (event.dataTransfer.types.includes(sessionDragMimeType) || event.dataTransfer.types.includes(paneDragMimeType));
  }
  function dragOver(event: DragEvent, sessionId: string) {
    if (!accepts(event)) return;
    event.preventDefault();
    event.stopPropagation();
    const swapping = event.dataTransfer.types.includes(paneDragMimeType);
    event.dataTransfer.dropEffect = "move";
    setTarget({ sessionId, edge: swapping ? null : edgeAt(event) });
  }
  function drop(event: DragEvent, targetSessionId: string) {
    if (!accepts(event)) return;
    event.preventDefault();
    event.stopPropagation();
    const sourcePane = event.dataTransfer.getData(paneDragMimeType);
    if (sourcePane !== "") splitView.dispatch({ type: "swap", sourceSessionId: sourcePane, targetSessionId });
    else splitView.split({ sourceSessionId: event.dataTransfer.getData(sessionDragMimeType), targetSessionId, edge: edgeAt(event) });
    clear();
  }
  return {
    moving: movingSession,
    target,
    atLimit: members.length >= maximumSplitViewPanes,
    clear,
    dragOver,
    drop,
    leave(event: DragEvent) {
      if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget)) setTarget(null);
    },
    begin(event: DragEvent, sessionId: string) {
      event.dataTransfer.setData(paneDragMimeType, sessionId);
      event.dataTransfer.effectAllowed = "move";
      setMoving(sessionId);
    },
    pick(sessionId: string) {
      if (movingSession === null) setMoving(sessionId);
      else {
        splitView.dispatch({ type: "swap", sourceSessionId: movingSession, targetSessionId: sessionId });
        clear();
      }
    },
  };
}
