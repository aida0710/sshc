import { useEffect, useRef, type RefObject } from "react";

// useFocusBackAfterInert puts the focus back where it was inside container once the container stops being
// inert. Making an element inert takes the focus off whatever inside it had it (the browser moves it to the
// page), and nothing gives it back: when the app hides itself for a moment to recheck the vault on coming back
// to the page, the terminal would otherwise stay without the keyboard until it is clicked.
export function useFocusBackAfterInert(container: RefObject<HTMLElement | null>, inert: boolean) {
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
    target.focus({ preventScroll: true });
  }, [inert]);
}
