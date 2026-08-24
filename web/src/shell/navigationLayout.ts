export const navigationVisibleStorageKey = "sshc.navigation.visible";
export const navigationWidthStorageKey = "sshc.navigation.width";

export const defaultNavigationVisible = true;
export const defaultNavigationWidth = 240;
export const minimumNavigationWidth = 192;
export const maximumNavigationWidth = 384;

export function clampNavigationWidth(width: number): number {
  if (!Number.isFinite(width)) return defaultNavigationWidth;
  return Math.min(maximumNavigationWidth, Math.max(minimumNavigationWidth, Math.round(width)));
}

export function detectNavigationVisible(): boolean {
  try {
    const stored = window.localStorage.getItem(navigationVisibleStorageKey);
    if (stored === "true") return true;
    if (stored === "false") return false;
  } catch {
  }
  return defaultNavigationVisible;
}

export function rememberNavigationVisible(visible: boolean): void {
  try {
    window.localStorage.setItem(navigationVisibleStorageKey, String(visible));
  } catch {
  }
}

export function detectNavigationWidth(): number {
  try {
    const stored = window.localStorage.getItem(navigationWidthStorageKey);
    if (stored !== null && stored.trim() !== "") {
      const width = Number(stored);
      if (Number.isFinite(width)) return clampNavigationWidth(width);
    }
  } catch {
  }
  return defaultNavigationWidth;
}

export function rememberNavigationWidth(width: number): void {
  try {
    window.localStorage.setItem(navigationWidthStorageKey, String(clampNavigationWidth(width)));
  } catch {
  }
}
