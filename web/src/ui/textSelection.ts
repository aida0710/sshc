// The field being typed in, when text is selected in it.
function fieldWithSelection(): HTMLTextAreaElement | HTMLInputElement | null {
  const field = document.activeElement;
  if (!(field instanceof HTMLTextAreaElement || field instanceof HTMLInputElement)) return null;
  return field.selectionStart !== field.selectionEnd ? field : null;
}

// The selection of the page's text, when there is one.
function pageSelection(): Selection | null {
  const selection = document.getSelection();
  return selection === null || selection.isCollapsed ? null : selection;
}

// hasTextSelection tells whether text is selected on the page, in the field being typed in or in the page's text.
export function hasTextSelection(): boolean {
  return fieldWithSelection() !== null || pageSelection() !== null;
}

// dropTextSelection lets go of the text selected on the page, in the field being typed in or in the page's text, and
// says whether there was any. Android's Back does this before it leaves the page, as it closes a menu first: the
// selection holds Android's bar of Copy and Share open.
export function dropTextSelection(): boolean {
  const field = fieldWithSelection();
  if (field !== null) {
    field.setSelectionRange(field.selectionEnd, field.selectionEnd);
    return true;
  }
  const selection = pageSelection();
  if (selection === null) return false;
  selection.removeAllRanges();
  return true;
}
