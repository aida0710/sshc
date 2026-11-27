import { clampSplitRatio, evenSplitRatio, minimumShareRatio, type SplitDirection } from "../../ui/SplitResizeHandle";
import { fitsSplitArea, splitMinimum, type SplitArea } from "./geometry";

// Four panes keep terminals usable at desktop widths.
export const maximumSplitViewPanes = 4;
export type PaneEdge = "left" | "right" | "top" | "bottom";
export type SplitViewNode =
  | { sessionId: string }
  | { direction: SplitDirection; ratio: number; first: SplitViewNode; second: SplitViewNode };
// SplitViewBranch は、ペインではなく、2つに分けた部分である。
export type SplitViewBranch = Exclude<SplitViewNode, { sessionId: string }>;
export type SplitPath = ("first" | "second")[];
export type SplitViewState = {
  root: SplitViewNode | null;
  activeSessionId: string | null;
  focusMode: boolean;
  focusRevision: number;
};
export type SplitViewAction =
  | { type: "select"; sessionId: string | null; focus?: boolean }
  // area は分割表示の領域の広さ。渡すと、どのペインにも最小の大きさを残せない分割を断る。
  | { type: "split"; sourceSessionId: string; targetSessionId: string; edge: PaneEdge; area?: SplitArea | undefined }
  | { type: "close"; sessionId: string }
  | { type: "swap"; sourceSessionId: string; targetSessionId: string }
  | { type: "resize"; path: SplitPath; ratio: number }
  | { type: "prune"; sessionIds: ReadonlySet<string> }
  | { type: "focus-mode"; enabled: boolean };

export const emptySplitView: SplitViewState = { root: null, activeSessionId: null, focusMode: false, focusRevision: 0 };

export function paneSessionIds(root: SplitViewNode | null): string[] {
  if (root === null) return [];
  return "sessionId" in root ? [root.sessionId] : [...paneSessionIds(root.first), ...paneSessionIds(root.second)];
}

// edgeDirection は、edge の側に並べる分割の向きである。
export function edgeDirection(edge: PaneEdge): SplitDirection {
  return edge === "left" || edge === "right" ? "horizontal" : "vertical";
}

function isSplitAlong(node: SplitViewNode, direction: SplitDirection): node is SplitViewBranch {
  return !("sessionId" in node) && node.direction === direction;
}

// pathTo は、sessionId のペインまでの道筋である。そのペインが無ければ null。
function pathTo(node: SplitViewNode, sessionId: string): SplitPath | null {
  if ("sessionId" in node) return node.sessionId === sessionId ? [] : null;
  for (const side of ["first", "second"] as const) {
    const below = pathTo(node[side], sessionId);
    if (below !== null) return [side, ...below];
  }
  return null;
}

function nodeAt(root: SplitViewNode, path: SplitPath): SplitViewNode {
  const [side, ...rest] = path;
  return side === undefined || "sessionId" in root ? root : nodeAt(root[side], rest);
}

// replaceAt は、path の位置の部分を replace の結果に替える。path がペインの先を指していれば何も変えない。
function replaceAt(root: SplitViewNode, path: SplitPath, replace: (node: SplitViewNode) => SplitViewNode): SplitViewNode {
  const [side, ...rest] = path;
  if (side === undefined) return replace(root);
  if ("sessionId" in root) return root;
  return { ...root, [side]: replaceAt(root[side], rest, replace) };
}

// evenRun は、direction の向きに続く分割の比率を、並んだペインがどれも同じ大きさになるように割り直す。
// 並びの中に向きの違う分割があれば、それを1つの区画として、最小の大きさに比例した分を渡す。
function evenRun(node: SplitViewNode, direction: SplitDirection): SplitViewNode {
  if (!isSplitAlong(node, direction)) return node;
  return { ...node, ratio: minimumShareRatio(splitMinimum(node)), first: evenRun(node.first, direction), second: evenRun(node.second, direction) };
}

// paneBeside は、target と sourceSessionId のペインを、edge の側に並べた分割である。
function paneBeside(target: SplitViewNode, sourceSessionId: string, edge: PaneEdge): SplitViewBranch {
  const before = edge === "left" || edge === "top";
  const source = { sessionId: sourceSessionId };
  return { direction: edgeDirection(edge), ratio: evenSplitRatio, first: before ? source : target, second: before ? target : source };
}

// evenRunAbove は、path の分割から上へ、同じ向きの分割が続く所までの並びを均等に割り直す。
function evenRunAbove(root: SplitViewNode, path: SplitPath, direction: SplitDirection): SplitViewNode {
  let runTop = path.length;
  while (runTop > 0 && isSplitAlong(nodeAt(root, path.slice(0, runTop - 1)), direction)) runTop -= 1;
  return replaceAt(root, path.slice(0, runTop), (run) => evenRun(run, direction));
}

function removePanes(root: SplitViewNode, sessionIds: ReadonlySet<string>): SplitViewNode | null {
  if ("sessionId" in root) return sessionIds.has(root.sessionId) ? null : root;
  const first = removePanes(root.first, sessionIds);
  const second = removePanes(root.second, sessionIds);
  if (first === null) return second;
  if (second === null) return first;
  return first === root.first && second === root.second ? root : { ...root, first, second };
}

function resizeSplit(root: SplitViewNode, path: SplitPath, ratio: number): SplitViewNode {
  return replaceAt(root, path, (node) => "sessionId" in node ? node : { ...node, ratio: clampSplitRatio(ratio) });
}

function closePanes(state: SplitViewState, sessionIds: ReadonlySet<string>): SplitViewState {
  if (state.root === null) return state;
  const root = removePanes(state.root, sessionIds);
  if (root === state.root) return state;
  const remaining = paneSessionIds(root);
  const activeSessionId = remaining.includes(state.activeSessionId ?? "") ? state.activeSessionId : remaining[0] ?? null;
  return { ...state, root, activeSessionId, focusMode: remaining.length > 1 && state.focusMode, focusRevision: state.focusRevision + (activeSessionId === state.activeSessionId ? 0 : 1) };
}

export function reduceSplitView(state: SplitViewState, action: SplitViewAction): SplitViewState {
  const members = paneSessionIds(state.root);
  switch (action.type) {
    case "select": {
      if (action.sessionId === state.activeSessionId && action.focus === false) return state;
      const member = action.sessionId !== null && members.includes(action.sessionId);
      return {
        ...state,
        root: member ? state.root : action.sessionId === null ? null : { sessionId: action.sessionId },
        activeSessionId: action.sessionId,
        focusMode: member && state.focusMode,
        focusRevision: state.focusRevision + (action.focus === false ? 0 : 1),
      };
    }
    case "split": {
      const { sourceSessionId, targetSessionId, edge } = action;
      if (state.root === null || !members.includes(targetSessionId) || sourceSessionId === targetSessionId || sourceSessionId === "") return state;
      const existing = members.includes(sourceSessionId);
      if (!existing && members.length >= maximumSplitViewPanes) return state;
      const withoutSource = existing ? removePanes(state.root, new Set([sourceSessionId])) : state.root;
      if (withoutSource === null) return state;
      const path = pathTo(withoutSource, targetSessionId);
      if (path === null) return state;
      // 足すたびに足した先のペインを半分に割ると、そのペインだけが細くなるので、加わった並びを均等に割り直す。
      const placed = replaceAt(withoutSource, path, (target) => paneBeside(target, sourceSessionId, edge));
      const root = evenRunAbove(placed, path, edgeDirection(edge));
      if (action.area !== undefined && !fitsSplitArea(root, action.area)) return state;
      return { root, activeSessionId: sourceSessionId, focusMode: false, focusRevision: state.focusRevision + 1 };
    }
    case "close":
      return members.length < 2 ? state : closePanes(state, new Set([action.sessionId]));
    case "prune":
      // Keep a single exited terminal available for inspecting its final output.
      return members.length < 2 ? state : closePanes(state, action.sessionIds);
    case "swap": {
      const { sourceSessionId, targetSessionId } = action;
      if (state.root === null || sourceSessionId === targetSessionId || !members.includes(sourceSessionId) || !members.includes(targetSessionId)) return state;
      // Replace both leaves in one traversal so their session identities remain unique.
      function swap(node: SplitViewNode): SplitViewNode {
        if ("sessionId" in node) {
          if (node.sessionId === sourceSessionId) return { sessionId: targetSessionId };
          if (node.sessionId === targetSessionId) return { sessionId: sourceSessionId };
          return node;
        }
        return { ...node, first: swap(node.first), second: swap(node.second) };
      }
      return { ...state, root: swap(state.root) };
    }
    case "resize":
      return state.root === null ? state : { ...state, root: resizeSplit(state.root, action.path, action.ratio) };
    case "focus-mode":
      return { ...state, focusMode: members.length > 1 && action.enabled };
  }
}
