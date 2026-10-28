import { useSyncExternalStore } from "react";
import { readStoredValue, writeStoredValue } from "./browserStorage";

// One of a few choices kept in this browser and shared by its tabs. A missing
// or unknown stored value reads as the fallback.
export type StoredChoice<T extends string> = {
  readonly fallback: T;
  load(): T;
  save(choice: T): void;
  subscribe(listener: () => void): () => void;
};

export function createStoredChoice<T extends string>({ key, choices, fallback }: {
  key: string;
  choices: readonly T[];
  fallback: T;
}): StoredChoice<T> {
  // "storage" reaches only the other tabs, so this tab tells itself with its own event.
  const changedEvent = `sshc:stored-choice-changed:${key}`;
  return {
    fallback,
    load: () => {
      const stored = readStoredValue(key);
      return choices.find((choice) => choice === stored) ?? fallback;
    },
    save: (choice) => {
      writeStoredValue(key, choice);
      window.dispatchEvent(new Event(changedEvent));
    },
    subscribe: (listener) => {
      window.addEventListener("storage", listener);
      window.addEventListener(changedEvent, listener);
      return () => {
        window.removeEventListener("storage", listener);
        window.removeEventListener(changedEvent, listener);
      };
    },
  };
}

export function useStoredChoice<T extends string>(choice: StoredChoice<T>): T {
  return useSyncExternalStore(choice.subscribe, choice.load, () => choice.fallback);
}
