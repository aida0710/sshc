// dropTextSelection lets go of the text selected on the page, in the field being typed in or in the page's text, and
// says whether there was any. Android's Back does this before it leaves the page, as it closes a menu first: the
// selection holds Android's bar of Copy and Share open.
export function dropTextSelection(): boolean {
  const field = document.activeElement;
  if ((field instanceof HTMLTextAreaElement || field instanceof HTMLInputElement) && field.selectionStart !== field.selectionEnd) {
    field.setSelectionRange(field.selectionEnd, field.selectionEnd);
    return true;
  }
  const selection = document.getSelection();
  if (selection === null || selection.isCollapsed) return false;
  selection.removeAllRanges();
  return true;
}
