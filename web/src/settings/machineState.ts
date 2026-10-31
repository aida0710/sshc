import { currentSettings, interfaceState, setInterfaceState, type InterfaceStateKey } from "./settingsStore";

// This machine's screen layout and what it has open (the run target, the SFTP tabs, the folded workspaces, the
// transfer list view). The engine keeps it per machine and syncs it, but never reads it: each
// value is the text the screen wrote, which the screen parses and checks again when it reads it.

// readMachineState is the text kept under key, or null when nothing is kept.
export function readMachineState(key: InterfaceStateKey): string | null {
  const value = interfaceState(currentSettings(), key);
  return value === "" ? null : value;
}

// parseMachineStateJSON reads the JSON document in a kept value, treating an empty or malformed one as fallback.
export function parseMachineStateJSON(value: string, fallback: unknown = null): unknown {
  if (value === "") return fallback;
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return fallback;
  }
}

// readMachineStateJSON reads a JSON document kept under key, treating a missing or malformed one as fallback.
export function readMachineStateJSON(key: InterfaceStateKey, fallback: unknown = null): unknown {
  return parseMachineStateJSON(interfaceState(currentSettings(), key), fallback);
}

// keepMachineState keeps value under key; an empty value forgets it. A refused save keeps the value on screen.
export function keepMachineState(key: InterfaceStateKey, value: string): void {
  void setInterfaceState(key, value).catch(() => undefined);
}

export function keepMachineStateJSON(key: InterfaceStateKey, value: unknown): void {
  keepMachineState(key, JSON.stringify(value));
}
