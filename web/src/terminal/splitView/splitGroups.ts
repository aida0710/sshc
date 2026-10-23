import { emptySplitView, paneSessionIds, reduceSplitView, type SplitViewAction, type SplitViewState } from "./layout";

// Eight arrangements keep the sidebar manageable without limiting live sessions.
export const maximumSplitGroups = 8;

export type SplitGroup = { number: number; view: SplitViewState };
export type SplitGroupsState = {
  groups: SplitGroup[];
  activeGroupNumber: number | null;
  nextGroupNumber: number;
  single: SplitViewState;
};
export type SplitGroupsAction = SplitViewAction
  | { type: "select-group"; number: number }
  | { type: "dissolve-group"; number: number };

export const emptySplitGroups: SplitGroupsState = {
  groups: [], activeGroupNumber: null, nextGroupNumber: 1, single: emptySplitView,
};

export function activeSplitView(state: SplitGroupsState): SplitViewState {
  return state.groups.find((group) => group.number === state.activeGroupNumber)?.view ?? state.single;
}

function singleSession(view: SplitViewState): SplitViewState {
  return reduceSplitView(emptySplitView, { type: "select", sessionId: view.activeSessionId });
}

function hasSplitPanes(view: SplitViewState): boolean {
  return view.root !== null && !("sessionId" in view.root);
}

function updateActiveView(state: SplitGroupsState, view: SplitViewState): SplitGroupsState {
  if (view === activeSplitView(state)) return state;
  if (state.activeGroupNumber === null) return { ...state, single: view };
  if (!hasSplitPanes(view)) {
    return { ...state, groups: state.groups.filter((group) => group.number !== state.activeGroupNumber), activeGroupNumber: null, single: view };
  }
  return { ...state, groups: state.groups.map((group) => group.number === state.activeGroupNumber ? { ...group, view } : group) };
}

function splitIntoGroup(state: SplitGroupsState, action: Extract<SplitViewAction, { type: "split" }>): SplitGroupsState {
  const view = activeSplitView(state);
  const nextView = reduceSplitView(view, action);
  if (view === nextView) return state;
  // Transfer membership atomically: a refused split must leave every group intact.
  const groups = state.groups.flatMap((group) => {
    if (group.number === state.activeGroupNumber) return [{ ...group, view: nextView }];
    const remaining = reduceSplitView(group.view, { type: "close", sessionId: action.sourceSessionId });
    return hasSplitPanes(remaining) ? [{ ...group, view: remaining }] : [];
  });
  if (state.activeGroupNumber !== null) return { ...state, groups };
  if (groups.length >= maximumSplitGroups) return state;
  // Never reuse numbers: names and sidebar badges remain stable after dissolution.
  const number = state.nextGroupNumber;
  return { ...state, groups: [...groups, { number, view: nextView }], activeGroupNumber: number, nextGroupNumber: number + 1, single: emptySplitView };
}

function pruneGroups(state: SplitGroupsState, action: Extract<SplitViewAction, { type: "prune" }>): SplitGroupsState {
  let next = state;
  for (const group of state.groups) {
    const view = reduceSplitView(group.view, action);
    if (view === group.view) continue;
    if (group.number === state.activeGroupNumber) {
      next = updateActiveView(next, view);
      continue;
    }
    const groups = next.groups.flatMap((candidate) => {
      if (candidate.number !== group.number) return [candidate];
      return hasSplitPanes(view) ? [{ ...candidate, view }] : [];
    });
    next = { ...next, groups };
  }
  return next;
}

export function reduceSplitGroups(state: SplitGroupsState, action: SplitGroupsAction): SplitGroupsState {
  switch (action.type) {
    case "select": {
      const group = state.groups.find((candidate) => paneSessionIds(candidate.view.root).includes(action.sessionId ?? ""));
      const number = group?.number ?? null;
      const next = number === state.activeGroupNumber ? state : { ...state, activeGroupNumber: number };
      return updateActiveView(next, reduceSplitView(group?.view ?? state.single, action));
    }
    case "select-group": {
      const group = state.groups.find((candidate) => candidate.number === action.number);
      if (group === undefined) return state;
      return updateActiveView({ ...state, activeGroupNumber: group.number }, reduceSplitView(group.view, { type: "select", sessionId: group.view.activeSessionId }));
    }
    case "dissolve-group": {
      if (!state.groups.some((group) => group.number === action.number)) return state;
      const active = state.activeGroupNumber === action.number;
      return { ...state, groups: state.groups.filter((group) => group.number !== action.number), activeGroupNumber: active ? null : state.activeGroupNumber, single: active ? singleSession(activeSplitView(state)) : state.single };
    }
    case "split": return splitIntoGroup(state, action);
    case "prune": return pruneGroups(state, action);
    default: return updateActiveView(state, reduceSplitView(activeSplitView(state), action));
  }
}
