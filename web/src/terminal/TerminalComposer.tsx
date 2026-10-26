import { useLayoutEffect, useRef, type KeyboardEvent } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { isImeComposing } from "../ui/imeComposition";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";

// The field grows with its text up to this many lines and then scrolls, so the
// terminal above keeps most of the screen.
const maximumComposerLines = 4;

function keepFocus(event: { preventDefault(): void }) {
  event.preventDefault();
}

// TerminalComposer is the phone's text box under the key bar. Japanese is
// composed and corrected here, and only the finished text reaches the terminal.
// Typing into the terminal directly still works; this is an extra way in.
export function TerminalComposer({
  value,
  disabled,
  onChange,
  onSubmit,
}: {
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
  onSubmit: () => void;
}) {
  const t = useTranslate();
  const visible = useMediaQuery(mobileViewportQuery);
  const field = useRef<HTMLTextAreaElement>(null);

  // Fit the field to its text. CSS field-sizing is not in every mobile browser yet.
  useLayoutEffect(() => {
    const element = field.current;
    if (element === null) return;
    const style = getComputedStyle(element);
    const lineHeight = Number.parseFloat(style.lineHeight);
    const padding = Number.parseFloat(style.paddingTop) + Number.parseFloat(style.paddingBottom);
    element.style.height = "auto";
    const tallest = Number.isFinite(lineHeight) ? maximumComposerLines * lineHeight + padding : Number.POSITIVE_INFINITY;
    element.style.height = `${Math.min(element.scrollHeight, tallest)}px`;
  }, [value, visible]);

  if (!visible) return null;

  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    // The Enter that confirms a Japanese conversion must not send.
    if (event.key !== "Enter" || event.shiftKey || isImeComposing(event)) return;
    event.preventDefault();
    if (!disabled) onSubmit();
  }

  return (
    <form
      aria-label={t("terminal.composer")}
      className="flex shrink-0 items-end gap-2 px-2.5 pb-1.5"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <textarea
        ref={field}
        rows={1}
        value={value}
        disabled={disabled}
        aria-label={t("terminal.composerField")}
        placeholder={t("terminal.composerPlaceholder")}
        enterKeyHint="send"
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={keyDown}
        className="min-h-11 grow resize-none rounded-[22px] bg-card px-3.5 py-2.5 text-base leading-6 text-ink placeholder:text-ink-faint focus-visible:outline-2 focus-visible:outline-accent disabled:opacity-60"
      />
      <button
        type="submit"
        disabled={disabled}
        aria-label={t("terminal.composerSend")}
        title={t("terminal.composerSend")}
        // The keyboard stays open: the field keeps the focus while the button is pressed.
        onPointerDown={keepFocus}
        onMouseDown={keepFocus}
        className="flex size-11 shrink-0 items-center justify-center rounded-full bg-accent text-accent-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:opacity-60"
      >
        <Icon name="arrowUp" className="size-5" />
      </button>
    </form>
  );
}
