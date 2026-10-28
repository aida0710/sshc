
export type ClipboardAccess = {
  readText(): Promise<string>;
  writeText(text: string): Promise<void>;
  // writeTextAndHTML puts the same text in both forms: a rich editor pastes the
  // HTML, anything else the plain text.
  writeTextAndHTML(text: string, html: string): Promise<void>;
};

// execCommandCopy copies through a hidden text area, which has to take focus
// to be selected. Focus goes back afterwards: without it, a copy made when
// text is selected in the terminal would leave the next keystrokes nowhere.
// html, when given, is put beside the text by the copy event.
function execCommandCopy(text: string, html?: string): boolean {
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
  const fill = (event: ClipboardEvent) => {
    if (html === undefined || event.clipboardData === null) return;
    event.clipboardData.setData("text/plain", text);
    event.clipboardData.setData("text/html", html);
    event.preventDefault();
  };
  document.addEventListener("copy", fill, true);
  try {
    holder.select();
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    document.removeEventListener("copy", fill, true);
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
  async writeTextAndHTML(text: string, html: string) {
    try {
      if (navigator.clipboard?.write !== undefined && typeof ClipboardItem !== "undefined") {
        await navigator.clipboard.write([new ClipboardItem({
          "text/plain": new Blob([text], { type: "text/plain" }),
          "text/html": new Blob([html], { type: "text/html" }),
        })]);
        return;
      }
    } catch {
    }
    if (!execCommandCopy(text, html)) throw new Error("clipboard_refused");
  },
};
