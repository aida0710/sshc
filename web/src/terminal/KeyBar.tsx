import { useTranslate } from "../i18n/context";
import { useEffect, useRef, useState } from "react";
import { Icon } from "../ui/icons";
import { isModifierHeld, modifierDoubleTapMilliseconds, type ModifierLatch, type ModifierName, type Modifiers } from "./modifierLatch";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { keepKeyboardAsIs } from "../ui/softKeyboard";
import type { KeyBarLayout } from "../keybar/keyBarLayout";
import { isModifierKey, type KeyBarKey } from "../keybar/keySequences";
import { KeyFace, spokenKeyName } from "../keybar/KeyFace";
import { keyBarKeyGap } from "../keybar/keyBarStyles";
import { playHaptic } from "../android/haptics";

const keyShape =
  `h-11 min-w-0 rounded-inset font-mono text-[12.5px] active:bg-select-fill ${keyBarKeyGap}`;
// The row always shown: the keys share the width, each at least 39px, and past that the row scrolls sideways. 39px is
// the most that shows all ten keys of the default row on a phone 412px wide (1080 pixels at 420dpi); the 44px of a
// place to press elsewhere would hide a key there, and these are what a person reaches for while a program waits.
// The key cut off at the edge of a narrower phone shows there is more, so the row needs no scrollbar.
const keyRowColumns = "grid min-w-0 flex-1 grid-flow-col auto-cols-[minmax(2.4375rem,1fr)] overflow-x-auto [scrollbar-width:none]";
// 「…」 stays at the end of the row, where a row that scrolls cannot take it out of sight.
const moreKeyWidth = "w-[2.4375rem] shrink-0";
const idleKey = "bg-card text-ink";
const pressedKey = "bg-select-fill text-ink";

// A modifier held for one key is tinted like a pressed key; a locked one takes
// the accent so the two can be told apart at a glance.
const latchColors: Record<ModifierLatch, string> = {
  off: idleKey,
  once: pressedKey,
  locked: "bg-accent text-accent-ink",
};

const modifierNames = { Control: "ctrl", Alt: "alt" } as const satisfies Record<string, ModifierName>;

// A key is felt as it goes down, as on the phone's keyboard, and leaves the keyboard as it is (keepKeyboardAsIs).
function pressKeyDown(event: { preventDefault(): void }) {
  keepKeyboardAsIs(event);
  playHaptic("keyPress");
}

// KeyBar is the phone's row of keys above its text box, laid out by the key bar preset in use. The keys after
// 「…」 open above the row.
export function KeyBar({
  layout,
  modifiers,
  directInput,
  onTap,
  onKey,
  onDirectInput,
}: {
  layout: KeyBarLayout;
  modifiers: Modifiers;
  // directInput is whether typing goes straight into the terminal (useDirectInput.ts).
  directInput: boolean;
  onTap: (name: ModifierName) => void;
  onKey: (key: KeyBarKey) => void;
  onDirectInput: () => void;
}) {
  const t = useTranslate();
  const visible = useMediaQuery(mobileViewportQuery);
  const [moreOpen, setMoreOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const moreCloseTimer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(moreCloseTimer.current), []);
  useDismissibleLayer({ open: visible && moreOpen, containerRefs: [root], onDismiss: () => setMoreOpen(false) });
  if (!visible) return null;

  const press = (key: KeyBarKey) => {
    onKey(key);
    setMoreOpen(false);
  };
  // A modifier behind 「…」 keeps the keys open for the double tap window, or the second tap that locks it
  // would have nothing to hit.
  const tapModifierBehindMore = (name: ModifierName) => {
    onTap(name);
    window.clearTimeout(moreCloseTimer.current);
    moreCloseTimer.current = window.setTimeout(() => setMoreOpen(false), modifierDoubleTapMilliseconds);
  };
  // 「…」 says which modifier behind it is held, since the key itself is out of sight.
  const heldBehindMore = layout.moreKeys.map((key) => key.key).find((name): name is "Control" | "Alt" =>
    name !== undefined && isModifierKey(name) && isModifierHeld(modifiers[modifierNames[name]]));
  const keyButton = (key: KeyBarKey, index: number, onModifier: (name: ModifierName) => void) => {
    if (key.key !== undefined && isModifierKey(key.key)) {
      const name = modifierNames[key.key];
      return <ModifierKeyButton key={key.key} barKey={key} latch={modifiers[name]} onTap={() => onModifier(name)} />;
    }
    if (key.key === "DirectInput") {
      return <DirectInputButton key={key.key} on={directInput} onTap={() => { onDirectInput(); setMoreOpen(false); }} />;
    }
    return <BarKeyButton key={key.key ?? `custom-${index}`} barKey={key} onPress={press} />;
  };

  return (
    <div ref={root} aria-label={t("terminal.keyBar")} className="relative shrink-0 px-2.5 pb-2 pt-2.5">
      <div className="flex">
        {/* The row scrolls sideways inside its own box, so the keys behind 「…」 can open above it unclipped. */}
        <div className={keyRowColumns}>
          {layout.keys.map((key, index) => keyButton(key, index, onTap))}
        </div>
        {layout.moreKeys.length === 0 ? null : (
          <button
            type="button"
            data-touch-compact
            aria-label={t("terminal.extraKeys")}
            title={t("terminal.extraKeys")}
            aria-expanded={moreOpen}
            onPointerDown={pressKeyDown}
            onMouseDown={keepKeyboardAsIs}
            onClick={() => setMoreOpen((current) => !current)}
            className={`${keyShape} ${moreKeyWidth} ${moreOpen ? pressedKey : heldBehindMore === undefined ? idleKey : latchColors[modifiers[modifierNames[heldBehindMore]]]}`}
          >
            {heldBehindMore === undefined ? <Icon name="moreHorizontal" className="mx-auto size-4" /> : <KeyFace barKey={{ key: heldBehindMore }} iconClassName="size-4" />}
          </button>
        )}
      </div>
      {moreOpen ? (
        <div className="absolute inset-x-1 bottom-full z-30 mb-1 grid grid-cols-6 gap-y-1 rounded-lg border border-line bg-toolbar p-1 shadow-float">
          {layout.moreKeys.map((key, index) => keyButton(key, index, tapModifierBehindMore))}
        </div>
      ) : null}
    </div>
  );
}

// ModifierKeyButton is Ctrl or Alt. It reads as pressed while held, for one key
// or locked, and is named "Ctrl (locked)" while locked.
function ModifierKeyButton({ barKey, latch, onTap }: { barKey: KeyBarKey; latch: ModifierLatch; onTap: () => void }) {
  const t = useTranslate();
  return (
    <button
      type="button"
      data-touch-compact
      aria-pressed={isModifierHeld(latch)}
      aria-label={latch === "locked" ? t("terminal.modifierLocked", { key: barKey.key === "Control" ? "Ctrl" : "Alt" }) : undefined}
      onPointerDown={pressKeyDown}
      onMouseDown={keepKeyboardAsIs}
      onClick={onTap}
      className={`${keyShape} ${latchColors[latch]}`}
    >
      <KeyFace barKey={barKey} iconClassName="size-[15px]" />
    </button>
  );
}

// DirectInputButton switches where typing goes. It takes the accent while typing goes straight into the
// terminal, as a locked modifier does.
function DirectInputButton({ on, onTap }: { on: boolean; onTap: () => void }) {
  const t = useTranslate();
  return (
    <button
      type="button"
      data-touch-compact
      aria-pressed={on}
      aria-label={t("keyBar.directInput")}
      title={t("keyBar.directInput")}
      onPointerDown={pressKeyDown}
      onMouseDown={keepKeyboardAsIs}
      onClick={onTap}
      className={`${keyShape} ${on ? latchColors.locked : idleKey}`}
    >
      <KeyFace barKey={{ key: "DirectInput" }} iconClassName="size-[15px]" />
    </button>
  );
}

// BarKeyButton is a key that sends something when tapped: a named key or a key the person made.
function BarKeyButton({ barKey, onPress }: { barKey: KeyBarKey; onPress: (key: KeyBarKey) => void }) {
  const t = useTranslate();
  return (
    <button
      type="button"
      data-touch-compact
      aria-label={spokenKeyName(t, barKey)}
      onPointerDown={pressKeyDown}
      onMouseDown={keepKeyboardAsIs}
      onClick={() => onPress(barKey)}
      className={`${keyShape} ${idleKey} truncate px-0.5`}
    >
      <KeyFace barKey={barKey} iconClassName="size-[15px]" />
    </button>
  );
}
