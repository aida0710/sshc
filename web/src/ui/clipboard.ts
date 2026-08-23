
export type ClipboardAccess = { readText(): Promise<string>; writeText(text: string): Promise<void> };

function execCommandCopy(text: string): boolean {
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
