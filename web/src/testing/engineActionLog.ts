import type { RemoteEngineActionLog } from "../api/remoteEngines";

// engineActionLog is a RemoteEngineActionLog of the first action on a host
// that has ended and whose whole output is in this one answer. A test names
// only the fields it is about.
export function engineActionLog(overrides: Partial<RemoteEngineActionLog> = {}): RemoteEngineActionLog {
  const output = overrides.output ?? "";
  return {
    generation: 1,
    action: "update",
    done: true,
    succeeded: true,
    cursor: { requested: 0, start: 0, next: output.length, end: output.length, truncated: false },
    output,
    ...overrides,
  };
}
