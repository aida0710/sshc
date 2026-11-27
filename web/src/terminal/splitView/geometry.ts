import type { CSSProperties } from "react";
import { fitSplitRatio, type SplitDirection, type SplitMinimum } from "../../ui/SplitResizeHandle";
import type { PaneEdge, SplitPath, SplitViewBranch, SplitViewNode } from "./layout";

// minimumPanePx is the least a pane keeps, padding included, however the
// splits around it are dragged (measured in the desktop layout). Across: the
// header's name cut to its dot and a few letters (52px), its search and "…"
// buttons (2 × 28px), the swap, focus and close buttons (88px) and the
// padding. Down: the 40px header, the terminal's 17px of padding and one
// 15px row. A smaller pane pushes its close button out of sight, or leaves
// xterm no row to draw while the shell writes on at the last size.
// ペインを足すとき（layout.ts の split）と、窓を狭めたとき（splitViewGeometry）にも守る。
export const minimumPanePx = { width: 248, height: 80 };

// minimumExtentPx is the least a part of the split takes along direction: its
// panes side by side add up, panes across it need only the largest.
export function minimumExtentPx(node: SplitViewNode, direction: SplitDirection): number {
  if ("sessionId" in node) return direction === "horizontal" ? minimumPanePx.width : minimumPanePx.height;
  const first = minimumExtentPx(node.first, direction);
  const second = minimumExtentPx(node.second, direction);
  return node.direction === direction ? first + second : Math.max(first, second);
}

// splitMinimum は、分割の2つの半分それぞれが、分割の向きに要る最小の大きさ（px）である。
export function splitMinimum(node: SplitViewBranch): SplitMinimum {
  return { first: minimumExtentPx(node.first, node.direction), second: minimumExtentPx(node.second, node.direction) };
}

// SplitArea は、分割表示の領域の広さ（px）である。
export type SplitArea = { width: number; height: number };

// fitsSplitArea は、area の広さで、root のどのペインにも minimumPanePx を残せるかを返す。
export function fitsSplitArea(root: SplitViewNode, area: SplitArea): boolean {
  return minimumExtentPx(root, "horizontal") <= area.width && minimumExtentPx(root, "vertical") <= area.height;
}

export type PaneBounds = { left: number; top: number; width: number; height: number };
export type PanePlacement = { sessionId: string; style: CSSProperties };
export type DividerPlacement = {
  path: SplitPath;
  style: CSSProperties;
  direction: "horizontal" | "vertical";
  ratio: number;
  minimum: SplitMinimum;
};

function boundsStyle(bounds: PaneBounds): CSSProperties {
  return { left: `${bounds.left}%`, top: `${bounds.top}%`, width: `${bounds.width}%`, height: `${bounds.height}%` };
}

// Flat placements let React keep each terminal mounted as the split tree changes.
// area は分割表示の領域の広さで、まだ測っていなければ null。広さが分かれば、覚えている比率のままでは
// 最小の大きさを割るペイン（窓を狭めたときなど）に、最小の大きさを残す比率で描く。覚えている比率は変えないので、
// 窓を広げ直せば元の比率に戻る。
export function splitViewGeometry(root: SplitViewNode | null, area: SplitArea | null): { panes: PanePlacement[]; dividers: DividerPlacement[] } {
  const panes: PanePlacement[] = [];
  const dividers: DividerPlacement[] = [];
  walkSplitTree(root, area, {
    pane: (sessionId, bounds) => panes.push({ sessionId, style: boundsStyle(bounds) }),
    divider: (node, ratio, bounds, path) => dividers.push({
      path, style: boundsStyle(bounds), direction: node.direction, ratio, minimum: splitMinimum(node),
    }),
  });
  return { panes, dividers };
}

// paneBounds gives each pane's place as percentages of the whole split, for drawing a small map
// of the layout (the sidebar's split list).
export function paneBounds(root: SplitViewNode | null): { sessionId: string; bounds: PaneBounds }[] {
  const placed: { sessionId: string; bounds: PaneBounds }[] = [];
  walkSplitTree(root, null, { pane: (sessionId, bounds) => placed.push({ sessionId, bounds }) });
  return placed;
}

// walkSplitTree は、各ペインと各仕切りの置き場所を、全体に対する割合で上から順に決める。仕切りの比率は、
// area があれば fitSplitRatio で両側に最小の大きさを残す値にし、その比率で下の分割を置く。
function walkSplitTree(root: SplitViewNode | null, area: SplitArea | null, visit: {
  pane: (sessionId: string, bounds: PaneBounds) => void;
  divider?: (node: SplitViewBranch, ratio: number, bounds: PaneBounds, path: SplitPath) => void;
}) {
  function place(node: SplitViewNode, bounds: PaneBounds, path: SplitPath) {
    if ("sessionId" in node) {
      visit.pane(node.sessionId, bounds);
      return;
    }
    const horizontal = node.direction === "horizontal";
    const extentPx = area === null ? 0 : horizontal ? bounds.width / 100 * area.width : bounds.height / 100 * area.height;
    const ratio = fitSplitRatio(node.ratio, extentPx, splitMinimum(node));
    visit.divider?.(node, ratio, bounds, path);
    const first = ratio / 100;
    const second = 1 - first;
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

// A pane dragged by its handle onto the middle of another pane swaps with it;
// onto the part near an edge, it moves beside that edge. The middle is the
// part farther than this share of the pane from every edge.
const paneEdgeZone = 0.25;

// paneDropZone is where a dragged pane goes: beside the nearest edge, or null
// to swap places.
export function paneDropZone(point: { x: number; y: number }, size: { width: number; height: number }): PaneEdge | null {
  const horizontal = point.x / size.width;
  const vertical = point.y / size.height;
  const nearEdge = Math.min(horizontal, 1 - horizontal, vertical, 1 - vertical) < paneEdgeZone;
  return nearEdge ? nearestPaneEdge(point, size) : null;
}

export function nearestPaneEdge(point: { x: number; y: number }, size: { width: number; height: number }): PaneEdge {
  const horizontal = point.x / size.width;
  const vertical = point.y / size.height;
  const distances: [PaneEdge, number][] = [["left", horizontal], ["right", 1 - horizontal], ["top", vertical], ["bottom", 1 - vertical]];
  return distances.reduce((nearest, candidate) => candidate[1] < nearest[1] ? candidate : nearest)[0];
}
