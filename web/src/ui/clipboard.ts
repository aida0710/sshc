
export type ClipboardAccess = { readText(): Promise<string>; writeText(text: string): Promise<void> };

// execCommandCopy copies through a hidden text area, which has to take focus
// to be selected. Focus goes back afterwards: without it, a copy made when
// text is selected in the terminal would leave the next keystrokes nowhere.
function execCommandCopy(text: string): boolean {
  const focused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const textSelection = focused instanceof HTMLInputElement || focused instanceof HTMLTextAreaElement
    ? { element: focused, start: focused.selectionStart, end: focused.selectionEnd, direction: focused.selectionDirection ?? undefined }
    : null;
  const holder = document.createElement("textarea");
  holder.value = text;
  holder.setAttribute("readonly", "");
  holder.setAttribute("aria-hidden", "true");
  holder.style.position = "fixed";
  holder.style.top = "-1000px";
  holder.style.opacity = "0";
  document.body.appendChild(holder);
  try {
    holder.select();
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    holder.remove();
    focused?.focus({ preventScroll: true });
    if (textSelection !== null && textSelection.start !== null && textSelection.end !== null) {
      textSelection.element.setSelectionRange(textSelection.start, textSelection.end, textSelection.direction);
    }
  }
}

export const clipboard: ClipboardAccess = {
  async readText() {
    if (navigator.clipboard === undefined) throw new Error("clipboard_unavailable");
    return navigator.clipboard.readText();
  },
  async writeText(text: string) {
    try {
      if (navigator.clipboard !== undefined) {
        await navigator.clipboard.writeText(text);
        return;
      }
    } catch {
    }
    if (!execCommandCopy(text)) throw new Error("clipboard_refused");
  },
};
