import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { useEffect, useRef, useState } from "react";
import { Icon, type IconName } from "../ui/icons";
import { isModifierHeld, modifierDoubleTapMilliseconds, type ModifierLatch, type ModifierName, type Modifiers } from "./modifierLatch";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";

// sequences は、キーの名前から送る制御シーケンスを引く。矢印キーの名前は KeyboardEvent.key に合わせる。
const sequences: Record<string, string> = {
  Esc: "\x1b",
  Tab: "\t",
  ArrowUp: "\x1b[A",
  ArrowDown: "\x1b[B",
  ArrowRight: "\x1b[C",
  ArrowLeft: "\x1b[D",
};
export function applyModifiers(data: string, ctrl: boolean, alt: boolean): string {
  if (data.length !== 1) return data;

  let body = data;
  if (ctrl) {
    const code = data.toLowerCase().charCodeAt(0);
    if (code >= 97 && code <= 122) body = String.fromCharCode(code - 96);
  }
  return alt ? "\x1b" + body : body;
}
export function encodeKey(name: string, ctrl: boolean, alt: boolean): string {
  const sequence = sequences[name];
  if (sequence !== undefined) return alt ? "\x1b" + sequence : sequence;
  return applyModifiers(name, ctrl, alt);
}

// BarKey は、キーバーに常に出すキーである。name は encodeKey に渡す名前である。矢印キーは、
// フォントによって形や大きさが変わる文字ではなくアイコンで出し、読み上げ用の名前を付ける。
type BarKey =
  | { kind: "text"; name: string }
  | { kind: "arrow"; name: string; icon: IconName; label: MessageKey };

const keys: BarKey[] = [
  { kind: "text", name: "Esc" },
  { kind: "text", name: "Tab" },
  { kind: "arrow", name: "ArrowLeft", icon: "arrowLeft", label: "terminal.keyArrowLeft" },
  { kind: "arrow", name: "ArrowUp", icon: "arrowUp", label: "terminal.keyArrowUp" },
  { kind: "arrow", name: "ArrowDown", icon: "arrowDown", label: "terminal.keyArrowDown" },
  { kind: "arrow", name: "ArrowRight", icon: "arrowRight", label: "terminal.keyArrowRight" },
];
const extraKeys = ["|", "-", "~", "/"];

const keyShape =
  "h-11 min-w-0 rounded-[10px] font-mono text-[12.5px] active:bg-select-fill focus-visible:outline-2 focus-visible:outline-accent";
const idleKey = "bg-card text-ink";
const pressedKey = "bg-select-fill text-ink";

// A modifier held for one key is tinted like a pressed key; a locked one takes
// the accent so the two can be told apart at a glance.
const latchColors: Record<ModifierLatch, string> = {
  off: idleKey,
  once: pressedKey,
  locked: "bg-accent text-accent-ink",
};

function keepFocus(event: { preventDefault(): void }) {
  event.preventDefault();
}

export function KeyBar({
  modifiers,
  onTap,
  onKey,
}: {
  modifiers: Modifiers;
  onTap: (name: ModifierName) => void;
  onKey: (name: string) => void;
}) {
  const t = useTranslate();
  const visible = useMediaQuery(mobileViewportQuery);
  const [extraOpen, setExtraOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const extraCloseTimer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(extraCloseTimer.current), []);
  useDismissibleLayer({ open: visible && extraOpen, containerRefs: [root], onDismiss: () => setExtraOpen(false) });
  if (!visible) return null;

  const press = (name: string) => {
    onKey(name);
    setExtraOpen(false);
  };
  // Alt sits in the popup, so the popup waits out the double tap window
  // before closing; otherwise the second tap that locks Alt has nothing to hit.
  const tapAlt = () => {
    onTap("alt");
    window.clearTimeout(extraCloseTimer.current);
    extraCloseTimer.current = window.setTimeout(() => setExtraOpen(false), modifierDoubleTapMilliseconds);
  };
  return (
    <div
      ref={root}
      aria-label={t("terminal.keyBar")}
      className="relative grid shrink-0 grid-cols-8 gap-1.5 px-2.5 pb-2 pt-2.5"
    >
      {keys.slice(0, 2).map((key) => (
        <BarKeyButton key={key.name} barKey={key} onPress={press} />
      ))}
      <ModifierKeyButton label="Ctrl" latch={modifiers.ctrl} onTap={() => onTap("ctrl")} />
      {keys.slice(2).map((key) => (
        <BarKeyButton key={key.name} barKey={key} onPress={press} />
      ))}
      <button
        type="button"
        data-touch-compact
        aria-label={t("terminal.extraKeys")}
        title={t("terminal.extraKeys")}
        aria-expanded={extraOpen}
        onPointerDown={keepFocus}
        onMouseDown={keepFocus}
        onClick={() => setExtraOpen((current) => !current)}
        className={`${keyShape} ${extraOpen ? pressedKey : latchColors[modifiers.alt]}`}
      >
        {isModifierHeld(modifiers.alt) ? "Alt" : <Icon name="moreHorizontal" className="mx-auto size-4" />}
      </button>
      {extraOpen ? (
        <div className="absolute inset-x-1 bottom-full z-30 mb-1 grid grid-cols-5 gap-1 rounded border border-line bg-toolbar p-1 shadow-lg">
          <ModifierKeyButton label="Alt" latch={modifiers.alt} onTap={tapAlt} />
          {extraKeys.map((label) => (
            <button key={label} type="button" onPointerDown={keepFocus} onMouseDown={keepFocus} onClick={() => press(label)} className={`${keyShape} ${idleKey}`}>{label}</button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

// ModifierKeyButton is Ctrl or Alt. It reads as pressed while held, for one key
// or locked, and is named "Ctrl (locked)" while locked.
function ModifierKeyButton({ label, latch, onTap }: { label: string; latch: ModifierLatch; onTap: () => void }) {
  const t = useTranslate();
  return (
    <button
      type="button"
      data-touch-compact
      aria-pressed={isModifierHeld(latch)}
      aria-label={latch === "locked" ? t("terminal.modifierLocked", { key: label }) : undefined}
      onPointerDown={keepFocus}
      onMouseDown={keepFocus}
      onClick={onTap}
      className={`${keyShape} ${latchColors[latch]}`}
    >
      {label}
    </button>
  );
}

// BarKeyButton is one key of the bar that sends a sequence (not Ctrl, not the extra keys).
function BarKeyButton({ barKey, onPress }: { barKey: BarKey; onPress: (name: string) => void }) {
  const t = useTranslate();
  return (
    <button
      type="button"
      data-touch-compact
      aria-label={barKey.kind === "arrow" ? t(barKey.label) : undefined}
      onPointerDown={keepFocus}
      onMouseDown={keepFocus}
      onClick={() => onPress(barKey.name)}
      className={`${keyShape} ${idleKey}`}
    >
      {barKey.kind === "arrow" ? <Icon name={barKey.icon} className="mx-auto size-[15px]" /> : barKey.name}
    </button>
  );
}
