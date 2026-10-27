// ModifierLatch is how a modifier key of the phone's key bar is held.
// One tap holds it for the next key only; a quick second tap locks it until
// it is tapped again, for typing several Ctrl keys in a row.
export type ModifierLatch = "off" | "once" | "locked";
export type ModifierName = "ctrl" | "alt";
export type Modifiers = Record<ModifierName, ModifierLatch>;

// The same window as Android's double tap (ViewConfiguration.DOUBLE_TAP_TIMEOUT),
// so the second tap feels as quick as everywhere else on the phone.
export const modifierDoubleTapMilliseconds = 300;

export const releasedModifiers: Modifiers = { ctrl: "off", alt: "off" };

export function tapModifier(current: ModifierLatch, millisecondsSincePreviousTap: number): ModifierLatch {
  switch (current) {
    case "off":
      return "once";
    case "once":
      return millisecondsSincePreviousTap <= modifierDoubleTapMilliseconds ? "locked" : "off";
    case "locked":
      return "off";
  }
}

export function isModifierHeld(latch: ModifierLatch): boolean {
  return latch !== "off";
}

// releaseAfterKey lets go of the modifiers that were held for one key; locked ones stay.
export function releaseAfterKey(modifiers: Modifiers): Modifiers {
  return {
    ctrl: modifiers.ctrl === "once" ? "off" : modifiers.ctrl,
    alt: modifiers.alt === "once" ? "off" : modifiers.alt,
  };
}
