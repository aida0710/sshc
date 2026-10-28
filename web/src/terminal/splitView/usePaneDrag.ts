import { useEffect, useState, type DragEvent } from "react";
import { sessionDragMimeType } from "../sessionDrag";
import { nearestPaneEdge, paneDropZone } from "./geometry";
import { maximumSplitViewPanes, paneSessionIds, type PaneEdge } from "./layout";
import type { SplitViewController } from "./useSplitView";
import { maximumSplitGroups } from "./splitGroups";

// A pane handle swaps panes in the middle of another pane and moves beside its
// edge near one; sidebar sessions split at the nearest edge.
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
  function pointIn(event: DragEvent) {
    const bounds = event.currentTarget.getBoundingClientRect();
    return { point: { x: event.clientX - bounds.left, y: event.clientY - bounds.top }, size: bounds };
  }
  // dropAt is where a drop at the pointer puts what is dragged: beside an edge, or null to swap panes.
  function dropAt(event: DragEvent): PaneEdge | null {
    const { point, size } = pointIn(event);
    return event.dataTransfer.types.includes(paneDragMimeType) ? paneDropZone(point, size) : nearestPaneEdge(point, size);
  }
  function accepts(event: DragEvent): boolean {
    return !splitView.compact && (event.dataTransfer.types.includes(sessionDragMimeType) || event.dataTransfer.types.includes(paneDragMimeType));
  }
  function dragOver(event: DragEvent, sessionId: string) {
    // A pane dropped on itself stays where it is.
    if (!accepts(event) || sessionId === movingSession) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = "move";
    setTarget({ sessionId, edge: dropAt(event) });
  }
  function drop(event: DragEvent, targetSessionId: string) {
    if (!accepts(event)) return;
    event.preventDefault();
    event.stopPropagation();
    const sourcePane = event.dataTransfer.getData(paneDragMimeType);
    const { point, size } = pointIn(event);
    if (sourcePane === "") {
      splitView.split({ sourceSessionId: event.dataTransfer.getData(sessionDragMimeType), targetSessionId, edge: nearestPaneEdge(point, size) });
    } else {
      const edge = paneDropZone(point, size);
      if (edge === null) splitView.dispatch({ type: "swap", sourceSessionId: sourcePane, targetSessionId });
      else splitView.split({ sourceSessionId: sourcePane, targetSessionId, edge });
    }
    clear();
  }
  return {
    moving: movingSession,
    target,
    atLimit: members.length >= maximumSplitViewPanes,
    atGroupLimit: splitView.activeGroupNumber === null && splitView.groups.length >= maximumSplitGroups,
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
