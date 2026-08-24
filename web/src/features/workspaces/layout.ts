export type SplitDirection = "horizontal" | "vertical";
export type ReconnectState = "reconnect_required" | "connecting" | "connected" | "failed";

export type StoredPane = {
  id: string;
  alias: string;
};

export type StoredNode =
  | { pane: StoredPane; split?: never }
  | {
      pane?: never;
      split: {
        direction: SplitDirection;
        ratio: number;
        first: StoredNode;
        second: StoredNode;
      };
    };

export type RuntimePane = StoredPane & {
  state: ReconnectState;
  sessionId?: string;
  problem?: string;
};

export type RuntimeNode =
  | { pane: RuntimePane; split?: never }
  | {
      pane?: never;
      split: {
        direction: SplitDirection;
        ratio: number;
        first: RuntimeNode;
        second: RuntimeNode;
      };
    };

export type LayoutState = {
  root: RuntimeNode;
  focusedPaneId: string;
};

export type LayoutAction =
  | { type: "focus"; paneId: string }
  | { type: "split"; paneId: string; direction: SplitDirection; pane: StoredPane }
  | { type: "resize-split"; path: ("first" | "second")[]; ratio: number }
  | { type: "close"; paneId: string }
  | { type: "connection-starting"; paneId: string }
  | { type: "connection-started"; paneId: string; sessionId: string }
  | { type: "connection-failed"; paneId: string; problem: string }
  | { type: "engine-restarted" };

export function restoreLayout(root: StoredNode, focusedPaneId: string): LayoutState {
  const hydrated = hydrateNode(root);
  const panes = paneIDs(hydrated);
  return { root: hydrated, focusedPaneId: panes.includes(focusedPaneId) ? focusedPaneId : (panes[0] ?? "") };
}

export function storeLayout(state: LayoutState): { layout: StoredNode; focusedPaneId: string } {
  return {
    layout: stripRuntime(state.root),
    focusedPaneId: state.focusedPaneId,
  };
}

export function reduceLayout(state: LayoutState, action: LayoutAction): LayoutState {
  switch (action.type) {
    case "focus":
      return paneIDs(state.root).includes(action.paneId) ? { ...state, focusedPaneId: action.paneId } : state;
    case "split": {
      if (paneIDs(state.root).includes(action.pane.id) || action.pane.id === "" || action.pane.alias === "") return state;
      const root = replacePane(state.root, action.paneId, (current) => ({
        split: {
          direction: action.direction,
          ratio: 50,
          first: { pane: current },
          second: { pane: { ...action.pane, state: "reconnect_required" } },
        },
      }));
      if (root === state.root) return state;
      return { root, focusedPaneId: action.pane.id };
    }
    case "resize-split": {
      const ratio = Math.min(90, Math.max(10, Math.round(action.ratio)));
      const root = resizeAt(state.root, action.path, ratio);
      return root === state.root ? state : { ...state, root };
    }
    case "close": {
      const root = removePane(state.root, action.paneId);
      if (root === null || root === state.root) return state;
      const panes = paneIDs(root);
      return {
        root,
        focusedPaneId: state.focusedPaneId === action.paneId ? (panes[0] ?? "") : state.focusedPaneId,
      };
    }
    case "connection-starting":
      return updatePaneState(state, action.paneId, (pane) => ({ id: pane.id, alias: pane.alias, state: "connecting" }));
    case "connection-started":
      if (action.sessionId === "") return state;
      return updatePaneState(state, action.paneId, (pane) => ({
        id: pane.id,
        alias: pane.alias,
        state: "connected",
        sessionId: action.sessionId,
      }));
    case "connection-failed":
      return updatePaneState(state, action.paneId, (pane) => ({
        id: pane.id,
        alias: pane.alias,
        state: "failed",
        problem: action.problem,
      }));
    case "engine-restarted":
      return {
        ...state,
        root: mapRuntimePanes(state.root, (pane) => ({ id: pane.id, alias: pane.alias, state: "reconnect_required" })),
      };
  }
}

export function paneIDs(root: RuntimeNode): string[] {
  const ids: string[] = [];
  visitPanes(root, (pane) => ids.push(pane.id));
  return ids;
}

function updatePaneState(
  state: LayoutState,
  paneId: string,
  update: (pane: RuntimePane) => RuntimePane,
): LayoutState {
  const root = replacePane(state.root, paneId, (pane) => ({ pane: update(pane) }));
  return root === state.root ? state : { ...state, root };
}

function replacePane(
  root: RuntimeNode,
  paneId: string,
  replace: (pane: RuntimePane) => RuntimeNode,
): RuntimeNode {
  if (root.pane !== undefined) return root.pane.id === paneId ? replace(root.pane) : root;
  const first = replacePane(root.split.first, paneId, replace);
  if (first !== root.split.first) return { split: { ...root.split, first } };
  const second = replacePane(root.split.second, paneId, replace);
  return second === root.split.second ? root : { split: { ...root.split, second } };
}

function removePane(root: RuntimeNode, paneId: string): RuntimeNode | null {
  if (root.pane !== undefined) return root.pane.id === paneId ? null : root;
  const first = removePane(root.split.first, paneId);
  if (first === null) return root.split.second;
  if (first !== root.split.first) return { split: { ...root.split, first } };
  const second = removePane(root.split.second, paneId);
  if (second === null) return root.split.first;
  return second === root.split.second ? root : { split: { ...root.split, second } };
}

function resizeAt(root: RuntimeNode, path: ("first" | "second")[], ratio: number): RuntimeNode {
  if (root.split === undefined) return root;
  if (path.length === 0) return root.split.ratio === ratio ? root : { split: { ...root.split, ratio } };
  const [side, ...remaining] = path;
  if (side === undefined) return root;
  const child = resizeAt(root.split[side], remaining, ratio);
  return child === root.split[side] ? root : { split: { ...root.split, [side]: child } };
}

function hydrateNode(root: StoredNode): RuntimeNode {
  if (root.pane !== undefined) return { pane: { ...root.pane, state: "reconnect_required" } };
  return {
    split: {
      direction: root.split.direction,
      ratio: root.split.ratio,
      first: hydrateNode(root.split.first),
      second: hydrateNode(root.split.second),
    },
  };
}

function stripRuntime(root: RuntimeNode): StoredNode {
  if (root.pane !== undefined) return { pane: { id: root.pane.id, alias: root.pane.alias } };
  return {
    split: {
      direction: root.split.direction,
      ratio: root.split.ratio,
      first: stripRuntime(root.split.first),
      second: stripRuntime(root.split.second),
    },
  };
}

function mapRuntimePanes(root: RuntimeNode, map: (pane: RuntimePane) => RuntimePane): RuntimeNode {
  if (root.pane !== undefined) return { pane: map(root.pane) };
  return {
    split: {
      ...root.split,
      first: mapRuntimePanes(root.split.first, map),
      second: mapRuntimePanes(root.split.second, map),
    },
  };
}

function visitPanes(root: RuntimeNode, visit: (pane: RuntimePane) => void): void {
  if (root.pane !== undefined) {
    visit(root.pane);
    return;
  }
  visitPanes(root.split.first, visit);
  visitPanes(root.split.second, visit);
}
