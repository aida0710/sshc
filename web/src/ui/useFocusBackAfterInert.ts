import { useEffect, useRef, type RefObject } from "react";
import { isTextEntry } from "./softKeyboard";

// useFocusBackAfterInert puts the focus back where it was inside container once the container stops being
// inert. Making an element inert takes the focus off whatever inside it had it (the browser moves it to the
// page), and nothing gives it back: when the app hides itself for a moment to recheck the vault on coming back
// to the page, the terminal would otherwise stay without the keyboard until it is clicked.
//
// keyboardOnScreen says a text field brings up an on-screen keyboard when it takes the focus (a phone). There a text
// field is not given the focus back: coming back to the app would raise the keyboard over the terminal, which comes
// up only for a tap. Android leaves the focus in the field when its keyboard is closed with Back, so the field that
// had the focus is often one the person has finished typing in.
export function useFocusBackAfterInert(container: RefObject<HTMLElement | null>, inert: boolean, { keyboardOnScreen }: { keyboardOnScreen: boolean }) {
  const lastFocused = useRef<HTMLElement | null>(null);
  const wasInert = useRef(inert);

  useEffect(() => {
    const remember = (event: FocusEvent) => {
      if (event.target instanceof HTMLElement && container.current?.contains(event.target)) lastFocused.current = event.target;
    };
    document.addEventListener("focusin", remember);
    return () => document.removeEventListener("focusin", remember);
  }, [container]);

  useEffect(() => {
    const cameBack = wasInert.current && !inert;
    wasInert.current = inert;
    if (!cameBack) return;
    const target = lastFocused.current;
    const active = document.activeElement;
    // Something the person chose while it was inert (an overlay's button, another window) keeps the focus.
    if (target === null || !target.isConnected || (active !== null && active !== document.body)) return;
    if (keyboardOnScreen && isTextEntry(target)) return;
    target.focus({ preventScroll: true });
  }, [inert, keyboardOnScreen]);
}
