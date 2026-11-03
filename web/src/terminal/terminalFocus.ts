// isTypingElsewhere is whether the focus sits in a text field outside the terminal (renaming a session
// in the sidebar, a field in a dialog). Attaching to a session finishes a moment after it opens; the
// terminal must not take the focus from such a field then, or the typing would end up in the shell.
// A button or link holding the focus (the one that opened the session) may give it up.
export function isTypingElsewhere(terminalRegion: HTMLElement | null): boolean {
  const active = document.activeElement;
  if (!(active instanceof HTMLElement) || terminalRegion?.contains(active) === true) return false;
  return active.isContentEditable || active.matches("input, textarea, select");
}

// isInNoticeCard is whether the focus sits in a notice card (NoticeFrame: one over the terminal, or the host key
// question of an SFTP tab), which took it to be acted on. The terminal attaching to its stream a moment later must not
// take it back.
export function isInNoticeCard(): boolean {
  const active = document.activeElement;
  return active instanceof Element && active.closest("[data-notice-card]") !== null;
}
