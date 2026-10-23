import type { KeyboardEvent as ReactKeyboardEvent } from "react";

// IMEs can report keyCode 229 even after isComposing becomes false.
export const imeKeyCode = 229;

export function isImeComposing(event: KeyboardEvent | ReactKeyboardEvent): boolean {
  const keyboardEvent = "nativeEvent" in event ? event.nativeEvent : event;
  return keyboardEvent.isComposing || keyboardEvent.keyCode === imeKeyCode;
}
