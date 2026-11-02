import { useSyncExternalStore } from "react";

// Direct input is where a phone types: into the terminal itself, a key at a time, or (off) into the text box
// under it, where Japanese is composed and a whole line is sent. The key bar's DirectInput key switches it (the
// user's choice, 2026-11-02). It holds for every session on this screen, so switching sessions keeps the way
// of typing, and starts off each time the app opens: it is a way of working at the moment, not a setting.

let directInput = false;
const listeners = new Set<() => void>();

const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
const snapshot = () => directInput;

export function setDirectInput(on: boolean) {
  if (on === directInput) return;
  directInput = on;
  listeners.forEach((listener) => listener());
}

// useDirectInput is whether a phone types straight into the terminal.
export function useDirectInput(): boolean {
  return useSyncExternalStore(subscribe, snapshot);
}
