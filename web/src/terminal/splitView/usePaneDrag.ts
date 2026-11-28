import { useEffect, useState, type DragEvent } from "react";
import { sessionDragMimeType } from "../sessionDrag";
import { nearestPaneEdge, paneDropZone } from "./geometry";
import { maximumSplitViewPanes, paneSessionIds, type PaneEdge } from "./layout";
import type { SplitViewController } from "./useSplitView";
import { maximumSplitGroups } from "./splitGroups";

// ペインの見出しをつかんで、ほかのペインの中央へ落とすと入れ替え、端の近くへ落とすとその側へ移す。
// サイドバーのセッションの行は、いちばん近い端の側へ分割する。
const paneDragMimeType = "application/x-sshc-split-view-pane";
type DropTarget = { sessionId: string; edge: PaneEdge | null };

// PaneMove は、動かしているペインと、その動かし方である。drag は見出しをつかんでいる間、pick は［ペインを移動］で
// 選んで、入れ替える相手のペインを待っている間。
type PaneMove = { sessionId: string; by: "drag" | "pick" };

export function usePaneDrag(splitView: SplitViewController) {
  const [move, setMove] = useState<PaneMove | null>(null);
  const [target, setTarget] = useState<DropTarget | null>(null);
  const members = paneSessionIds(splitView.state.root);
  const movingSession = move !== null && members.includes(move.sessionId) ? move.sessionId : null;
  const pickedSession = move?.by === "pick" ? movingSession : null;
  function clear() { setMove(null); setTarget(null); }
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
    // picked は、［ペインを移動］で選んで、入れ替える相手を待っているペインである。ドラッグの間は null。
    picked: pickedSession,
    target,
    atLimit: members.length >= maximumSplitViewPanes,
    atGroupLimit: splitView.activeGroupNumber === null && splitView.groups.length >= maximumSplitGroups,
    // lacksRoom は、いま重ねている側に落としても、どれかのペインが最小の大きさを割るので分割しないこと。
    // サイドバーの行は落とすまで何のセッションか分からないので、新しいペインとして見る。
    lacksRoom: target !== null && target.edge !== null && splitView.lacksRoomFor(movingSession, target.sessionId, target.edge),
    clear,
    dragOver,
    drop,
    leave(event: DragEvent) {
      if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget)) setTarget(null);
    },
    begin(event: DragEvent, sessionId: string) {
      event.dataTransfer.setData(paneDragMimeType, sessionId);
      event.dataTransfer.effectAllowed = "move";
      setMove({ sessionId, by: "drag" });
    },
    pick(sessionId: string) {
      if (pickedSession === null) setMove({ sessionId, by: "pick" });
      else {
        splitView.dispatch({ type: "swap", sourceSessionId: pickedSession, targetSessionId: sessionId });
        clear();
      }
    },
  };
}
