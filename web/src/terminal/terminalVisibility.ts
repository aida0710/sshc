// Whether this browser shows a terminal, for visibility reports (mode 2033,
// answered by the engine; see visibilityReports.ts). A terminal is shown while
// the page is not hidden and the terminal is laid out with a size: a pane left
// out of the split, a terminal kept hidden behind the panes and a session not
// on the screen have none. A window without the focus still shows it.

export function isTerminalShown(container: HTMLElement): boolean {
  return !container.ownerDocument.hidden && container.clientWidth > 0 && container.clientHeight > 0;
}

// watchTerminalShown calls onChange with the new value each time
// isTerminalShown changes.
export function watchTerminalShown(container: HTMLElement, onChange: (shown: boolean) => void): () => void {
  let shown = isTerminalShown(container);
  const check = () => {
    const now = isTerminalShown(container);
    if (now === shown) return;
    shown = now;
    onChange(now);
  };
  const resized = new ResizeObserver(check);
  resized.observe(container);
  container.ownerDocument.addEventListener("visibilitychange", check);
  return () => {
    resized.disconnect();
    container.ownerDocument.removeEventListener("visibilitychange", check);
  };
}
