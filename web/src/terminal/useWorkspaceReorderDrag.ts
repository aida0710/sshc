import { useState, type DragEvent } from "react";
import type { WorkspacePlace, WorkspacePlaceSide } from "./workspaceGrouping";

// workspaceDragMimeType marks the drag of a workspace row. Only the workspaces of the tree take it: the
// session lists, the workspace rows that take sessions and the terminal panes look for the session type.
const workspaceDragMimeType = "application/x-sshc-workspace";

// The upper half of a workspace (its row and its sessions) puts the dragged one before it and the lower
// half after it, so the end of the tree is the lower half of the last workspace.
function sideOf(event: DragEvent): WorkspacePlaceSide {
  const box = event.currentTarget.getBoundingClientRect();
  return event.clientY < box.top + box.height / 2 ? "before" : "after";
}

// useWorkspaceReorderDrag lets a workspace be dragged by its row and dropped on another workspace of the
// tree. dropPlace is where it would land now, for the tree to draw the line; onMove gets the workspace
// and where it was let go.
export function useWorkspaceReorderDrag(onMove: (workspaceId: string, place: WorkspacePlace) => void) {
  const [heldWorkspaceId, setHeldWorkspaceId] = useState<string | null>(null);
  const [dropPlace, setDropPlace] = useState<WorkspacePlace | null>(null);

  function release() {
    setHeldWorkspaceId(null);
    setDropPlace(null);
  }

  function isWorkspaceDrag(event: DragEvent): boolean {
    return heldWorkspaceId !== null && event.dataTransfer.types.includes(workspaceDragMimeType);
  }

  const dragSourceProps = (workspaceId: string) => ({
    draggable: true,
    onDragStart: (event: DragEvent) => {
      event.dataTransfer.setData(workspaceDragMimeType, workspaceId);
      event.dataTransfer.effectAllowed = "move";
      setHeldWorkspaceId(workspaceId);
    },
    onDragEnd: release,
  });

  const dropTargetProps = (targetId: string) => ({
    onDragOver: (event: DragEvent) => {
      if (!isWorkspaceDrag(event)) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = "move";
      const side = sideOf(event);
      setDropPlace((current) => current?.targetId === targetId && current.side === side ? current : { targetId, side });
    },
    onDrop: (event: DragEvent) => {
      const held = heldWorkspaceId;
      if (held === null || !isWorkspaceDrag(event)) return;
      event.preventDefault();
      release();
      if (held !== targetId) onMove(held, { targetId, side: sideOf(event) });
    },
  });

  return { heldWorkspaceId, dropPlace, dragSourceProps, dropTargetProps };
}
