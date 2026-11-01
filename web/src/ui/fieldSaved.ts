import { createContext } from "react";

// FieldSaved lets a page that saves each change at once say "Saved" under the row (FieldRow) the change was made
// in. A row tells noteChange when something in it changes; savedRow is the row whose change was just saved,
// for a moment. Outside such a page there is no FieldSaved, and a row says nothing.
export type FieldSaved = {
  noteChange: (row: string | null) => void;
  savedRow: string | null;
  savedText: string;
};

export const FieldSavedContext = createContext<FieldSaved | null>(null);

// changeEvent says whether an event inside a row is a change of its value: typing, a choice, or a button pressed
// (a picker of colours or images is made of buttons). Focus alone is not, nor a click on a field to type in it.
export function isChangeEvent(event: { type: string; target: EventTarget | null }): boolean {
  if (event.type !== "click") return true;
  return event.target instanceof Element && event.target.closest("button") !== null;
}
