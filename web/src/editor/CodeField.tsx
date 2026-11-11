import { lazy, Suspense, useRef, type RefObject } from "react";
import type { DescribableControlProps } from "../ui/describeControl";
import { loadMonacoCodeField } from "./loadMonacoEditor";

// CodeLanguage is how a CodeField colours its text: an SSH config (ssh_config(5)), a shell command, or plain lines.
export type CodeLanguage = "ssh-config" | "shell" | "plaintext";

// CodeMarker marks a line of a CodeField with a problem found in it, such as a keyword OpenSSH does not know.
export type CodeMarker = {
  line: number;
  severity: "error" | "warning" | "info";
  message: string;
};

export type CodeFieldProps = {
  // label is the field's accessible name.
  label: string;
  value: string;
  onChange: (value: string) => void;
  // onBlur runs when the text loses focus, for a field saved when it is left.
  onBlur?: () => void;
  language?: CodeLanguage;
  disabled?: boolean;
  placeholder?: string;
  // minLines and maxLines are the lines the field shows: it grows with its text from the one to the other, then
  // scrolls. A field that fills its place (fill) takes the height of its parent instead.
  minLines?: number;
  maxLines?: number;
  fill?: boolean;
  markers?: readonly CodeMarker[];
  // selectedLine selects and shows a line, each time its request changes (a line another page handed over).
  selectedLine?: { line: number; request: number } | null;
} & DescribableControlProps;

// LoadedCodeFieldProps are what the Monaco field takes: also whether the text area it replaces had focus, so the
// person typing as Monaco loads goes on typing in it.
export type LoadedCodeFieldProps = CodeFieldProps & { typingBeforeLoad?: RefObject<boolean> };

const MonacoCodeField = lazy(() => loadMonacoCodeField().then(({ MonacoCodeField: field }) => ({ default: field })));

// CodeField is a text field whose lines mean something (an SSH config file, a Host block, a VPN's file, rules of one
// pattern per line), edited with Monaco as SFTP's editor is. Monaco loads the first time a field is shown; until then
// the field is a plain text area holding the same text, so nothing typed is lost.
export function CodeField(props: CodeFieldProps) {
  const typingBeforeLoad = useRef(false);
  return (
    <Suspense fallback={<PlainCodeField {...props} typingBeforeLoad={typingBeforeLoad} />}>
      <MonacoCodeField {...props} typingBeforeLoad={typingBeforeLoad} />
    </Suspense>
  );
}

// codeFieldFrame is the field's border, as the other form controls draw theirs.
export const codeFieldFrame = "overflow-hidden rounded-md border border-control-line bg-control focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-accent";

// PlainCodeField is the field while Monaco loads.
function PlainCodeField({ label, value, onChange, onBlur, disabled, placeholder, minLines = 3, fill, typingBeforeLoad, ...description }: LoadedCodeFieldProps) {
  return (
    <textarea
      aria-label={label}
      aria-describedby={description["aria-describedby"]}
      aria-invalid={description["aria-invalid"]}
      value={value}
      placeholder={placeholder}
      disabled={disabled}
      rows={minLines}
      spellCheck={false}
      onChange={(event) => onChange(event.target.value)}
      onFocus={() => {
        if (typingBeforeLoad !== undefined) typingBeforeLoad.current = true;
      }}
      onBlur={() => {
        if (typingBeforeLoad !== undefined) typingBeforeLoad.current = false;
        onBlur?.();
      }}
      className={`${codeFieldFrame} block w-full resize-none px-3 py-2 font-mono text-ui leading-[19px] text-ink ${fill ? "h-full min-h-64" : ""}`}
    />
  );
}
