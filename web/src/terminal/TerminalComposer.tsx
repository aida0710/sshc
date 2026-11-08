import { useLayoutEffect, useRef, type KeyboardEvent, type RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { isImeComposing } from "../ui/imeComposition";
import { AlwaysMaskedPasswordInput } from "../ui/PasswordField";
import { matchesMediaQuery, mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { isSoftKeyboardOpen, literalTextEntry } from "../ui/softKeyboard";
import { pressesEnterOnly } from "./composerSend";
import { touchFirstQuery } from "./touchMode";
import { playHaptic } from "../android/haptics";

// The field grows with its text up to this many lines and then scrolls, so the
// terminal above keeps most of the screen.
const maximumComposerLines = 4;

const fieldLook = "min-h-11 min-w-0 grow rounded-[22px] bg-card px-3.5 py-2.5 text-base leading-6 text-ink placeholder:text-ink-faint focus-visible:outline-2 focus-visible:outline-accent";

function keepFocus(event: { preventDefault(): void }) {
  event.preventDefault();
}

// ComposerField is the field that takes the typing: the text box, or the password field while the terminal asks for one.
export type ComposerField = HTMLTextAreaElement | HTMLInputElement;

// TerminalComposer is the phone's text box under the key bar. Japanese is
// composed and corrected here, and only the finished text reaches the terminal.
// A tap on the terminal types here (TerminalView); the key bar's Ctrl and Alt
// hand the next key to the terminal itself.
// Enter starts a new line and the button beside the box sends, unless the
// settings choose enterSends. With nothing typed the button presses Enter, which
// answers a program's menu, and says so with its icon and name.
// The box stays editable while nothing can be sent: a person on a bad connection
// can finish what they are writing, and a disabled box would close the keyboard.
// While the terminal asks for a password, a password field takes the box's place. The keyboard then treats what is
// typed as a password: it does not learn it, suggest from it or show it in a conversion, which hiding the letters of a
// textarea would not stop. The box stays mounted with what was being written in it, for when the question is over.
export function TerminalComposer({
  value,
  secret,
  sendable,
  enterSends,
  placeholder,
  fieldRef,
  onChange,
  onSecretChange,
  onSubmit,
}: {
  value: string;
  // secret is the answer typed so far while the terminal asks for a password, and null otherwise.
  secret: string | null;
  // sendable is false while nothing can be sent. onSubmit still runs, to say why.
  sendable: boolean;
  enterSends: boolean;
  // placeholder says what sending does when it is not sending text (connecting again). Undefined is the usual words.
  placeholder?: string | undefined;
  // fieldRef is given the field that takes the typing while it is shown, and null otherwise.
  fieldRef?: RefObject<ComposerField | null>;
  onChange: (value: string) => void;
  onSecretChange: (secret: string) => void;
  onSubmit: () => void;
}) {
  const t = useTranslate();
  const visible = useMediaQuery(mobileViewportQuery);
  const textField = useRef<HTMLTextAreaElement>(null);
  const secretField = useRef<HTMLInputElement>(null);
  const masked = secret !== null;

  // Show one field and hide the other, in this order. A field that is hidden loses the focus, and the focus must go
  // straight from one to the other: an open keyboard stays open for that (switchDirectInput in TerminalView does the
  // same), and a moment with no field focused may close it. A keyboard closed with Back leaves the focus in the field,
  // and focus() would raise it again: there the hidden field just loses the focus, and a tap brings the keyboard back.
  useLayoutEffect(() => {
    const shown = masked ? secretField.current : textField.current;
    const other = masked ? textField.current : secretField.current;
    if (shown === null || other === null) return;
    shown.hidden = false;
    const wouldRaiseKeyboard = matchesMediaQuery(touchFirstQuery) && !isSoftKeyboardOpen();
    if (document.activeElement === other && !wouldRaiseKeyboard) shown.focus();
    other.hidden = true;
    if (fieldRef === undefined) return;
    fieldRef.current = shown;
    return () => { fieldRef.current = null; };
  }, [fieldRef, masked, visible]);

  // Fit the box to its text. CSS field-sizing is not in every mobile browser yet.
  useLayoutEffect(() => {
    const element = textField.current;
    if (element === null || masked) return;
    const style = getComputedStyle(element);
    const lineHeight = Number.parseFloat(style.lineHeight);
    const padding = Number.parseFloat(style.paddingTop) + Number.parseFloat(style.paddingBottom);
    element.style.height = "auto";
    const tallest = Number.isFinite(lineHeight) ? maximumComposerLines * lineHeight + padding : Number.POSITIVE_INFINITY;
    element.style.height = `${Math.min(element.scrollHeight, tallest)}px`;
  }, [masked, value, visible]);

  if (!visible) return null;

  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    // The Enter that confirms a Japanese conversion must not send.
    if (event.key !== "Enter" || isImeComposing(event)) return;
    // A hardware keyboard keeps a way to do the other thing, as chat apps do:
    // Shift+Enter starts a new line when Enter sends, and Ctrl+Enter or
    // Command+Enter sends when Enter starts a new line.
    const sends = enterSends ? !event.shiftKey : event.ctrlKey || event.metaKey;
    if (!sends) return;
    event.preventDefault();
    onSubmit();
  }

  const pressesEnter = pressesEnterOnly(secret ?? value);
  const sendName = t(pressesEnter ? "terminal.composerPressEnter" : "terminal.composerSend");

  return (
    <form
      aria-label={t("terminal.composer")}
      className="flex shrink-0 items-end gap-2 px-2.5 pb-1.5"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <textarea {...literalTextEntry}
        ref={textField}
        rows={1}
        value={value}
        aria-label={t("terminal.composerField")}
        placeholder={placeholder ?? t("terminal.composerPlaceholder")}
        enterKeyHint={enterSends ? "send" : "enter"}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={keyDown}
        className={`${fieldLook} resize-none`}
      />
      {/* An answer has no line breaks, so Enter sends whatever the settings say: the browser submits the form. */}
      <AlwaysMaskedPasswordInput
        ref={secretField}
        label={t("terminal.composerSecretField")}
        value={secret ?? ""}
        placeholder={t("terminal.composerSecretPlaceholder")}
        onChange={onSecretChange}
        className={fieldLook}
      />
      <button
        type="submit"
        aria-disabled={!sendable}
        aria-label={sendName}
        title={sendName}
        // The keyboard stays open: the field keeps the focus while the button is pressed. The button is felt as it
        // goes down, as the key bar's keys are.
        onPointerDown={(event) => {
          keepFocus(event);
          playHaptic("keyPress");
        }}
        onMouseDown={keepFocus}
        className="flex size-11 shrink-0 items-center justify-center rounded-full bg-accent text-accent-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent aria-disabled:opacity-60"
      >
        <Icon name={pressesEnter ? "cornerDownLeft" : "arrowUp"} className="size-5" />
      </button>
    </form>
  );
}
