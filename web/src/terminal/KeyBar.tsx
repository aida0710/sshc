import { useTranslate } from "../i18n/context";

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

const keys = ["Esc", "Tab", "↑", "↓", "←", "→", "|", "-", "~", "/"];

const keyShape =
  "min-h-11 min-w-11 shrink-0 rounded-md border border-control-line px-3 text-sm text-ink";

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
  return (
    <div
      aria-label={t("terminal.keyBar")}
      className="flex shrink-0 gap-1 overflow-x-auto border-t border-line bg-toolbar p-1 md:hidden"
    >
      <button
        type="button"
        aria-pressed={modifiers.ctrl}
        onPointerDown={keepFocus}
        onMouseDown={keepFocus}
        onClick={() => onToggle("ctrl")}
        className={`${keyShape} ${modifiers.ctrl ? "bg-select-fill" : "bg-card"}`}
      >
        Ctrl
      </button>
      <button
        type="button"
        aria-pressed={modifiers.alt}
        onPointerDown={keepFocus}
        onMouseDown={keepFocus}
        onClick={() => onToggle("alt")}
        className={`${keyShape} ${modifiers.alt ? "bg-select-fill" : "bg-card"}`}
      >
        Alt
      </button>
      {keys.map((label) => (
        <button
          key={label}
          type="button"
          onPointerDown={keepFocus}
          onMouseDown={keepFocus}
          onClick={() => onKey(label)}
          className={`${keyShape} bg-card`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}
