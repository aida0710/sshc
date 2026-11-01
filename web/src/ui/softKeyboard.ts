// The phone's on-screen keyboard comes up for the element that has the focus when it takes text.
export function isTextEntry(element: Element | null): element is HTMLElement {
  return element instanceof HTMLElement &&
    (element.matches("input:not([type=checkbox]):not([type=radio]), textarea") || element.isContentEditable);
}

// useAppViewport marks the root while the on-screen keyboard is up.
function isSoftKeyboardOpen(): boolean {
  return document.documentElement.dataset.keyboardOpen === "true";
}

// keepKeyboardAsIs is the pointerdown and mousedown handler of on-screen keys that send their input
// themselves, such as the terminal's Ctrl and arrows. It keeps the focus where it is, so an open keyboard
// stays open for the next letter. While the keyboard is closed it lets go of the focused text field:
// Android brings the keyboard up on any tap while a text field has the focus, and such a key should not.
export function keepKeyboardAsIs(event: { preventDefault(): void }): void {
  event.preventDefault();
  if (isSoftKeyboardOpen()) return;
  const active = document.activeElement;
  if (isTextEntry(active)) active.blur();
}

// literalTextEntry is for a field whose text is typed as it is meant (a path, a command, a key's file name): a phone's
// keyboard must not capitalise its first letter, correct its words or underline them as misspelt. Without it,
// Android is asked for both (an input or a textarea defaults to sentence capitals and autocorrect), so on a Latin
// keyboard "ls" becomes "Ls".
export const literalTextEntry = { autoCapitalize: "off", autoCorrect: "off", spellCheck: false } as const;

// asciiTextEntry is for a field that only ever takes ASCII (a host name, a user name, a connection name): besides
// literalTextEntry, the keyboard opens on its Latin letters. A phone set to Japanese otherwise opens on kana, to be
// switched by hand for every host name.
export const asciiTextEntry = { ...literalTextEntry, inputMode: "url" } as const;
