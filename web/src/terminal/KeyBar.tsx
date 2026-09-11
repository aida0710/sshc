import { useTranslate } from "../i18n/context";
import { useRef, useState } from "react";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";

const sequences: Record<string, string> = {
  Esc: "\x1b",
  Tab: "\t",
  "↑": "\x1b[A",
  "↓": "\x1b[B",
  "→": "\x1b[C",
  "←": "\x1b[D",
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
export function encodeKey(label: string, ctrl: boolean, alt: boolean): string {
  const sequence = sequences[label];
  if (sequence !== undefined) return alt ? "\x1b" + sequence : sequence;
  return applyModifiers(label, ctrl, alt);
}

const keys = ["Esc", "Tab", "←", "↑", "↓", "→"];
const extraKeys = ["|", "-", "~", "/"];

const keyShape =
  "min-h-11 min-w-0 rounded border border-control-line font-mono text-sm text-ink active:bg-select-fill focus-visible:outline-2 focus-visible:outline-accent";

function keepFocus(event: { preventDefault(): void }) {
  event.preventDefault();
}

export type Modifiers = { ctrl: boolean; alt: boolean };
export function KeyBar({
  modifiers,
  onToggle,
  onKey,
}: {
  modifiers: Modifiers;
  onToggle: (name: keyof Modifiers) => void;
  onKey: (label: string) => void;
}) {
  const t = useTranslate();
  const visible = useMediaQuery(mobileViewportQuery);
  const [extraOpen, setExtraOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useDismissibleLayer({ open: visible && extraOpen, containerRefs: [root], onDismiss: () => setExtraOpen(false) });
  if (!visible) return null;

  const press = (label: string) => {
    onKey(label);
    setExtraOpen(false);
  };
  return (
    <div
      ref={root}
      aria-label={t("terminal.keyBar")}
      className="relative grid shrink-0 grid-cols-8 border-t border-line bg-toolbar px-1"
    >
      <button
        type="button"
        data-touch-compact
        aria-pressed={modifiers.ctrl}
        onPointerDown={keepFocus}
        onMouseDown={keepFocus}
        onClick={() => onToggle("ctrl")}
        className={`${keyShape} ${modifiers.ctrl ? "bg-select-fill" : "bg-card"}`}
      >
        Ctrl
      </button>
      {keys.map((label) => (
        <button
          key={label}
          type="button"
          data-touch-compact
          onPointerDown={keepFocus}
          onMouseDown={keepFocus}
          onClick={() => press(label)}
          className={`${keyShape} bg-card`}
        >
          {label}
        </button>
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
        className={`${keyShape} ${extraOpen || modifiers.alt ? "bg-select-fill" : "bg-card"}`}
      >
        {modifiers.alt ? "Alt" : "…"}
      </button>
      {extraOpen ? (
        <div className="absolute inset-x-1 bottom-full z-30 mb-1 grid grid-cols-5 gap-1 rounded border border-line bg-toolbar p-1 shadow-lg">
          <button
            type="button"
            aria-pressed={modifiers.alt}
            onPointerDown={keepFocus}
            onMouseDown={keepFocus}
            onClick={() => { onToggle("alt"); setExtraOpen(false); }}
            className={`${keyShape} ${modifiers.alt ? "bg-select-fill" : "bg-card"}`}
          >Alt</button>
          {extraKeys.map((label) => (
            <button key={label} type="button" onPointerDown={keepFocus} onMouseDown={keepFocus} onClick={() => press(label)} className={`${keyShape} bg-card`}>{label}</button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
