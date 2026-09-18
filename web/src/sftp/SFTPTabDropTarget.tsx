import { useState, type DragEvent } from "react";
import { useTranslate } from "../i18n/context";
import type { PaneSide } from "./sftpPanes";

// A dragged SFTP tab carries its id under this type, so file drops from the
// entry lists and from the desktop are never mistaken for a tab.
export const sftpTabMimeType = "application/x-sshc-sftp-tab";

export function draggedTabId(event: DragEvent<HTMLElement>): string {
  return typeof event.dataTransfer.getData === "function" ? event.dataTransfer.getData(sftpTabMimeType) : "";
}

export function carriesTab(event: DragEvent<HTMLElement>): boolean {
  return Array.from(event.dataTransfer.types ?? []).includes(sftpTabMimeType);
}

function sideOf(event: DragEvent<HTMLElement>): PaneSide {
  const bounds = event.currentTarget.getBoundingClientRect();
  return event.clientX < bounds.left + bounds.width / 2 ? "left" : "right";
}

// SFTPTabDropTarget covers a pane while a tab is being dragged. A lone pane
// offers its two halves, each opening a new pane on that side; a pane that
// already has a neighbour is one target that takes the tab into itself.
export function SFTPTabDropTarget({
  placement,
  onDrop,
}: {
  placement: "halves" | "whole";
  onDrop: (side: PaneSide | null) => void;
}) {
  const t = useTranslate();
  const [hovered, setHovered] = useState<PaneSide | "whole" | null>(null);

  function over(event: DragEvent<HTMLDivElement>) {
    if (!carriesTab(event)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setHovered(placement === "halves" ? sideOf(event) : "whole");
  }

  function leave(event: DragEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setHovered(null);
  }

  function drop(event: DragEvent<HTMLDivElement>) {
    if (!carriesTab(event)) return;
    event.preventDefault();
    event.stopPropagation();
    setHovered(null);
    onDrop(placement === "halves" ? sideOf(event) : null);
  }

  const shade = hovered === "left" ? "inset-y-0 left-0 w-1/2"
    : hovered === "right" ? "inset-y-0 right-0 w-1/2"
      : hovered === "whole" ? "inset-0" : "";
  const label = hovered === "left" ? t("sftp.dropTabLeft")
    : hovered === "right" ? t("sftp.dropTabRight")
      : t("sftp.dropTabHere");

  return (
    <div
      data-sftp-tab-drop-target={placement}
      className="absolute inset-0 z-20"
      onDragEnter={over}
      onDragOver={over}
      onDragLeave={leave}
      onDrop={drop}
    >
      {hovered === null ? null : (
        <div data-sftp-tab-drop-side={hovered} className={`pointer-events-none absolute grid place-items-center border-2 border-accent bg-accent/20 ${shade}`}>
          <span className="rounded bg-toolbar px-3 py-2 text-xs font-medium shadow">{label}</span>
        </div>
      )}
    </div>
  );
}
