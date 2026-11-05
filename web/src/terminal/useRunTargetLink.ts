import { useState } from "react";
import type { ApiError } from "../api/client";
import type { RemoteEngineList, RemoteEnginesApi } from "../api/remoteEngines";
import { useDelayedFlag } from "../ui/useDelayedFlag";
import { usePolling } from "../ui/usePolling";

// An engine that is reachable answers within this time. The wait is only shown when it lasts longer, so that nothing
// flashes on every switch.
export const waitNoticeDelayMs = 600;
// While the screen waits, the step the connection is on should change as soon as a person can notice it.
const waitingEnginesPollMs = 1_000;
// Otherwise the list only marks the rows of the switcher. Reading it does not cross SSH, so it costs next to nothing.
const settledEnginesPollMs = 5_000;

// What the screen can say about the engine in use:
// - ready: it answered, or the wait is still too short to mention.
// - loading: this machine's engine has not answered yet.
// - connecting: there is no SSH connection to the run target with its engine's credentials yet.
// - awaitingEngine: that connection is there, and its engine has not answered yet.
// - refused: it answered with a refusal, or could not be reached.
export type RunTargetLinkPhase = "ready" | "loading" | "connecting" | "awaitingEngine" | "refused";

export type RunTargetLink = {
  phase: RunTargetLinkPhase;
  refusal: ApiError | null;
  // connectedEngines are the run targets this machine's engine holds a connection to now.
  connectedEngines: ReadonlySet<string>;
};

const noEngines: ReadonlySet<string> = new Set();

// useRunTargetLink says how the connection to the engine in use is going. `loaded` and `refusal` come from its session
// list; the list of run targets says which step a wait is on, and answers even when the run target does not.
export function useRunTargetLink({ selected, api, enabled, loaded, refusal }: {
  selected: string | null;
  api: Pick<RemoteEnginesApi, "list">;
  // enabled is false while there is no run target to ask about.
  enabled: boolean;
  loaded: boolean;
  refusal: ApiError | null;
}): RunTargetLink {
  const [connectedEngines, setConnectedEngines] = useState(noEngines);
  const waitedLong = useDelayedFlag(!loaded, waitNoticeDelayMs);
  usePolling(async (isCurrent) => {
    const listed = await api.list();
    if (isCurrent()) setConnectedEngines((current) => connectedAliases(listed, current));
  }, { intervalMs: loaded ? settledEnginesPollMs : waitingEnginesPollMs, enabled, immediately: true });
  return {
    phase: linkPhase({ selected, refused: refusal !== null, waitedLong, connected: selected !== null && connectedEngines.has(selected) }),
    refusal,
    connectedEngines,
  };
}

function linkPhase({ selected, refused, waitedLong, connected }: {
  selected: string | null;
  refused: boolean;
  waitedLong: boolean;
  connected: boolean;
}): RunTargetLinkPhase {
  if (refused) return "refused";
  if (!waitedLong) return "ready";
  if (selected === null) return "loading";
  return connected ? "awaitingEngine" : "connecting";
}

// connectedAliases keeps the set it was given when nothing changed, so that a read draws nothing anew.
function connectedAliases(listed: RemoteEngineList, current: ReadonlySet<string>): ReadonlySet<string> {
  const connected = listed.engines.filter((engine) => engine.connected).map((engine) => engine.alias);
  const unchanged = connected.length === current.size && connected.every((alias) => current.has(alias));
  return unchanged ? current : new Set(connected);
}
