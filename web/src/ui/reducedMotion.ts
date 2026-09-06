export const reducedMotionQuery = "(prefers-reduced-motion: reduce)";

export function cursorAnimationEnabled(reducedMotion: boolean): boolean {
  return !reducedMotion;
}

export function editorCursorBlinking(reducedMotion: boolean): "blink" | "solid" {
  return reducedMotion ? "solid" : "blink";
}
