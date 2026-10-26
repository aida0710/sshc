import type { CSSProperties } from "react";
import type { PaneEdge, SplitPath, SplitViewNode } from "./layout";

export type PaneBounds = { left: number; top: number; width: number; height: number };
export type PanePlacement = { sessionId: string; style: CSSProperties };
export type DividerPlacement = {
  path: SplitPath;
  style: CSSProperties;
  direction: "horizontal" | "vertical";
  ratio: number;
};

function boundsStyle(bounds: PaneBounds): CSSProperties {
  return { left: `${bounds.left}%`, top: `${bounds.top}%`, width: `${bounds.width}%`, height: `${bounds.height}%` };
}

// Flat placements let React keep each terminal mounted as the split tree changes.
export function splitViewGeometry(root: SplitViewNode | null): { panes: PanePlacement[]; dividers: DividerPlacement[] } {
  const panes: PanePlacement[] = [];
  const dividers: DividerPlacement[] = [];
  walkSplitTree(root, {
    pane: (sessionId, bounds) => panes.push({ sessionId, style: boundsStyle(bounds) }),
    divider: (node, bounds, path) => dividers.push({ path, style: boundsStyle(bounds), direction: node.direction, ratio: node.ratio }),
  });
  return { panes, dividers };
}

// paneBounds gives each pane's place as percentages of the whole split, for drawing a small map
// of the layout (the sidebar's split list).
export function paneBounds(root: SplitViewNode | null): { sessionId: string; bounds: PaneBounds }[] {
  const placed: { sessionId: string; bounds: PaneBounds }[] = [];
  walkSplitTree(root, { pane: (sessionId, bounds) => placed.push({ sessionId, bounds }) });
  return placed;
}

function walkSplitTree(root: SplitViewNode | null, visit: {
  pane: (sessionId: string, bounds: PaneBounds) => void;
  divider?: (node: Exclude<SplitViewNode, { sessionId: string }>, bounds: PaneBounds, path: SplitPath) => void;
}) {
  function place(node: SplitViewNode, bounds: PaneBounds, path: SplitPath) {
    if ("sessionId" in node) {
      visit.pane(node.sessionId, bounds);
      return;
    }
    visit.divider?.(node, bounds, path);
    const first = node.ratio / 100;
    const second = 1 - first;
    const horizontal = node.direction === "horizontal";
    place(node.first, { ...bounds, width: bounds.width * (horizontal ? first : 1), height: bounds.height * (horizontal ? 1 : first) }, [...path, "first"]);
    place(node.second, {
      left: bounds.left + (horizontal ? bounds.width * first : 0),
      top: bounds.top + (horizontal ? 0 : bounds.height * first),
      width: bounds.width * (horizontal ? second : 1),
      height: bounds.height * (horizontal ? 1 : second),
    }, [...path, "second"]);
  }
  if (root !== null) place(root, { left: 0, top: 0, width: 100, height: 100 }, []);
}

export function nearestPaneEdge(point: { x: number; y: number }, size: { width: number; height: number }): PaneEdge {
  const horizontal = point.x / size.width;
  const vertical = point.y / size.height;
  const distances: [PaneEdge, number][] = [["left", horizontal], ["right", 1 - horizontal], ["top", vertical], ["bottom", 1 - vertical]];
  return distances.reduce((nearest, candidate) => candidate[1] < nearest[1] ? candidate : nearest)[0];
}
