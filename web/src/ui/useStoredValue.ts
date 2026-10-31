import { useState } from "react";
import { forgetStoredValue, readStoredValue, writeStoredValue } from "./browserStorage";

// StoredValue is one screen value this browser remembers (browserStorageKeys.ts). parse reads what is stored,
// or null when nothing is, and turns anything unreadable into fallback. fallback itself is not written, so the
// key holds only what someone chose.
export type StoredValue<T extends string | number> = {
  key: string;
  fallback: T;
  parse: (stored: string | null) => T;
};

// useStoredValue keeps one screen value in this browser: read when the screen first draws, written as it
// changes. Storage that refuses leaves the value working for this page.
export function useStoredValue<T extends string | number>(stored: StoredValue<T>): [T, (value: T) => void] {
  const [value, setValue] = useState(() => {
    const text = readStoredValue(stored.key);
    return stored.parse(text === null || text.trim() === "" ? null : text);
  });
  const change = (next: T) => {
    setValue(next);
    if (next === stored.fallback) forgetStoredValue(stored.key);
    else writeStoredValue(stored.key, String(next));
  };
  return [value, change];
}
