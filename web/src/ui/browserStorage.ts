// Browser storage holds conveniences only: a remembered tab, a column width,
// the last theme. Private windows, quota limits and enterprise policies can
// all refuse it, and even reading `window.localStorage` can throw, so every
// preference must keep working with nothing stored. These wrappers turn a
// refusal into "nothing stored" so callers never repeat the try/catch.

export function readStoredValue(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function writeStoredValue(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Losing the preference is acceptable; it still applies for this page.
  }
}

export function removeStoredValue(key: string): void {
  try {
    window.localStorage.removeItem(key);
  } catch {
    // There is nothing to remove when storage is unavailable.
  }
}

// Reads a JSON document, treating a missing, refused or malformed value as
// `fallback`. Callers still validate the shape of what comes back.
export function readStoredJSON(key: string, fallback: unknown = null): unknown {
  const stored = readStoredValue(key);
  if (stored === null) return fallback;
  try {
    return JSON.parse(stored) as unknown;
  } catch {
    return fallback;
  }
}

export function writeStoredJSON(key: string, value: unknown): void {
  writeStoredValue(key, JSON.stringify(value));
}
