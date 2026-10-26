// isTypingElsewhere is whether the focus sits in a text field outside the terminal (renaming a session
// in the sidebar, a field in a dialog). Attaching to a session finishes a moment after it opens; the
// terminal must not take the focus from such a field then, or the typing would end up in the shell.
// A button or link holding the focus (the one that opened the session) may give it up.
export function isTypingElsewhere(terminalRegion: HTMLElement | null): boolean {
  const active = document.activeElement;
  if (!(active instanceof HTMLElement) || terminalRegion?.contains(active) === true) return false;
  return active.isContentEditable || active.matches("input, textarea, select");
}
