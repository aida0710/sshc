import { useRef } from "react";

// Sessions shown recently but no longer in the split view stay mounted and
// hidden, so going back to one keeps its whole scrollback instead of
// rebuilding it from the engine's replay buffer (256 KiB by default). Each
// keeps its terminal's scrollback in memory and its stream open; their WebGL
// renderers are let go while hidden (attachWebglWhileShown).
export const maximumRetainedTerminals = 3;

// useRetainedSessions returns the sessions to keep hidden, most recently shown
// first. It is worked out while rendering: a session that leaves the panes has
// to stay in the same render, or React unmounts its terminal first.
export function useRetainedSessions(paneIds: readonly string[], existingIds: ReadonlySet<string>): string[] {
  const recent = useRef<readonly string[]>([]);
  const shown = new Set(paneIds);
  const retained = recent.current.filter((id) => !shown.has(id) && existingIds.has(id)).slice(0, maximumRetainedTerminals);
  recent.current = [...paneIds, ...retained];
  return retained;
}
