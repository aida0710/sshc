import type { DragEvent } from "react";
import { useTranslate } from "../../i18n/context";
import type { DockEdge } from "./layout";

// The side of the pane the pointer is nearest to, as a fraction of its size.
export function dockEdge(event: DragEvent<HTMLElement>): DockEdge {
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

function overlayClass(edge: DockEdge): string {
  switch (edge) {
    case "left": return "inset-y-0 left-0 w-1/2";
    case "right": return "inset-y-0 right-0 w-1/2";
    case "top": return "inset-x-0 top-0 h-1/2";
    case "bottom": return "inset-x-0 bottom-0 h-1/2";
  }
}

// Shades the half of a pane a dragged console would take.
export function DockPreview({ edge }: { edge: DockEdge }) {
  const t = useTranslate();
  const label = edge === "left" ? t("workspace.dock.left")
    : edge === "right" ? t("workspace.dock.right")
      : edge === "top" ? t("workspace.dock.top")
        : t("workspace.dock.bottom");
  return (
    <div data-dock-preview={edge} className={`pointer-events-none absolute z-10 grid place-items-center border-2 border-accent bg-accent/20 ${overlayClass(edge)}`}>
      <span className="rounded bg-toolbar px-3 py-2 text-xs font-medium shadow">{label}</span>
    </div>
  );
}
