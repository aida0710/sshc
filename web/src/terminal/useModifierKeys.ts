import { useCallback, useRef, useState } from "react";
import { isModifierHeld, releaseAfterKey, releasedModifiers, tapModifier, type ModifierName, type Modifiers } from "./modifierLatch";

// useModifierKeys holds the key bar's Ctrl and Alt for one terminal.
// takeModifiers is called from the terminal's input callbacks, which outlive a
// render, so it reads the latest state through a ref instead of a captured value.
export function useModifierKeys() {
  const [modifiers, setModifiers] = useState<Modifiers>(releasedModifiers);
  const latest = useRef(modifiers);
  latest.current = modifiers;
  const previousTap = useRef<Record<ModifierName, number>>({ ctrl: Number.NEGATIVE_INFINITY, alt: Number.NEGATIVE_INFINITY });

  const tap = useCallback((name: ModifierName) => {
    const now = performance.now();
    const elapsed = now - previousTap.current[name];
    previousTap.current[name] = now;
    setModifiers((current) => ({ ...current, [name]: tapModifier(current[name], elapsed) }));
  }, []);

  // takeModifiers answers which modifiers apply to the key being sent and lets
  // go of those that were held for that key only.
  const takeModifiers = useCallback(() => {
    const { ctrl, alt } = latest.current;
    if (ctrl === "once" || alt === "once") setModifiers(releaseAfterKey);
    return { ctrl: isModifierHeld(ctrl), alt: isModifierHeld(alt) };
  }, []);

  return { modifiers, tap, takeModifiers };
}
