// Browser storage holds copies and conveniences only: the theme and language
// drawn before the engine answers, an update result already shown, the column
// widths and views a drag or a click changes. Private windows, quota limits and enterprise policies can
// all refuse it, and even reading `window.localStorage` can throw, so every
// preference must keep working with nothing stored. These wrappers turn a
// refusal into "nothing stored" so callers never repeat the try/catch. The
// keys are listed, with their naming rule, in browserStorageKeys.ts.

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

export function forgetStoredValue(key: string): void {
  try {
    window.localStorage.removeItem(key);
  } catch {
    // Storage that cannot be reached holds nothing to forget.
  }
}
