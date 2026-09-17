// Where the browser stands with the WebSocket behind a terminal. This is
// separate from the engine-side session state, which the session reports:
// a connected session can still have a dropped link while the tab was asleep.
export type StreamLink =
  | { phase: "live" }
  | { phase: "connecting"; attempt: number }
  | { phase: "waiting"; attempt: number; seconds: number }
  | { phase: "stopped"; gone: boolean };
