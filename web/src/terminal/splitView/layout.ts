import { clampSplitRatio, type SplitDirection } from "../../ui/SplitResizeHandle";

// Four panes keep terminals usable at desktop widths.
export const maximumSplitViewPanes = 4;
const initialSplitRatio = 50;
export type PaneEdge = "left" | "right" | "top" | "bottom";
export type SplitViewNode =
  | { sessionId: string }
  | { direction: SplitDirection; ratio: number; first: SplitViewNode; second: SplitViewNode };
export type SplitPath = ("first" | "second")[];
export type SplitViewState = {
  root: SplitViewNode | null;
  activeSessionId: string | null;
  focusMode: boolean;
  focusRevision: number;
};
export type SplitViewAction =
  | { type: "select"; sessionId: string | null; focus?: boolean }
  | { type: "split"; sourceSessionId: string; targetSessionId: string; edge: PaneEdge }
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

function replacePane(root: SplitViewNode, sessionId: string, replace: (pane: SplitViewNode) => SplitViewNode): SplitViewNode {
  if ("sessionId" in root) return root.sessionId === sessionId ? replace(root) : root;
  return { ...root, first: replacePane(root.first, sessionId, replace), second: replacePane(root.second, sessionId, replace) };
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
  if ("sessionId" in root) return root;
  const [side, ...rest] = path;
  return side === undefined ? { ...root, ratio: clampSplitRatio(ratio) } : { ...root, [side]: resizeSplit(root[side], rest, ratio) };
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
      const before = edge === "left" || edge === "top";
      const direction = edge === "left" || edge === "right" ? "horizontal" : "vertical";
      const source = { sessionId: sourceSessionId };
      const root = replacePane(withoutSource, targetSessionId, (target) => ({
        direction,
        ratio: initialSplitRatio,
        first: before ? source : target,
        second: before ? target : source,
      }));
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
