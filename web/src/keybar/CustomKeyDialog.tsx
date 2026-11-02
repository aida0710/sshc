import { useRef, useState } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import { control, dialogHeading, hintText } from "../ui/form";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { literalTextEntry } from "../ui/softKeyboard";
import { keyBarLabelForbiddenPattern, keyBarTextForbiddenPattern, maxKeyBarLabelLength, maxKeyBarTextLength } from "../rules/keyBar.generated";
import type { KeyBarKey } from "./keySequences";

// CustomKey is a key the person makes: what it shows and what it sends.
export type CustomKey = { label: string; text: string; enter?: boolean };

// customKeyProblem is why a key cannot be kept as it is, by the engine's rules (internal/validate/keybar.go),
// or "" when it can.
function customKeyProblem(t: Translate, { label, text }: CustomKey): { label: string; text: string } {
  return {
    label: label.trim() === "" ? t("keyBar.customLabelRequired")
      : [...label].length > maxKeyBarLabelLength ? t("keyBar.customLabelTooLong", { max: String(maxKeyBarLabelLength) })
      : keyBarLabelForbiddenPattern.test(label) ? t("keyBar.customLabelControl") : "",
    text: text === "" ? t("keyBar.customTextRequired")
      : [...text].length > maxKeyBarTextLength ? t("keyBar.customTextTooLong", { max: String(maxKeyBarTextLength) })
      : keyBarTextForbiddenPattern.test(text) ? t("keyBar.customTextControl") : "",
  };
}

// CustomKeyDialog makes a key that types a fixed text, or edits one (initial).
export function CustomKeyDialog({ initial, onSubmit, onCancel }: {
  initial?: KeyBarKey;
  onSubmit: (key: CustomKey) => void;
  onCancel: () => void;
}) {
  const t = useTranslate();
  const [label, setLabel] = useState(initial?.label ?? "");
  const [text, setText] = useState(initial?.text ?? "");
  const [enter, setEnter] = useState(initial?.enter ?? false);
  const [touched, setTouched] = useState(false);
  const labelRef = useRef<HTMLInputElement>(null);
  const problem = customKeyProblem(t, { label, text });

  function submit() {
    setTouched(true);
    if (problem.label !== "" || problem.text !== "") return;
    onSubmit(enter ? { label, text, enter } : { label, text });
  }

  return (
    <ModalShell labelledBy="custom-key-heading" placement="sheet" onDismiss={onCancel} initialFocusRef={labelRef}
      panelClassName="flex w-full max-w-md flex-col gap-4 rounded-lg p-4 sm:p-5">
      <h2 id="custom-key-heading" className={dialogHeading}>{t("keyBar.customHeading")}</h2>
      <form className="flex flex-col gap-4" onSubmit={(event) => { event.preventDefault(); submit(); }}>
        <div className="flex flex-col gap-1">
          <label htmlFor="custom-key-label" className="text-xs font-medium tracking-wide text-ink-muted">{t("keyBar.customLabel")}</label>
          <input {...literalTextEntry} id="custom-key-label" ref={labelRef} value={label} className={`${control} font-mono`}
            aria-invalid={touched && problem.label !== ""} aria-describedby="custom-key-label-hint"
            onChange={(event) => setLabel(event.target.value)} />
          <span id="custom-key-label-hint" role={touched && problem.label !== "" ? "alert" : undefined}
            className={touched && problem.label !== "" ? "text-xs text-danger" : hintText}>
            {touched && problem.label !== "" ? problem.label : t("keyBar.customLabelHint", { max: String(maxKeyBarLabelLength) })}
          </span>
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="custom-key-text" className="text-xs font-medium tracking-wide text-ink-muted">{t("keyBar.customText")}</label>
          <textarea {...literalTextEntry} id="custom-key-text" rows={3} value={text} className={`${control} min-h-20 font-mono`}
            aria-invalid={touched && problem.text !== ""} aria-describedby="custom-key-text-hint"
            onChange={(event) => setText(event.target.value)} />
          <span id="custom-key-text-hint" role={touched && problem.text !== "" ? "alert" : undefined}
            className={touched && problem.text !== "" ? "text-xs text-danger" : hintText}>
            {touched && problem.text !== "" ? problem.text : t("keyBar.customTextHint")}
          </span>
        </div>
        <label className="flex items-center gap-2 text-sm text-ink">
          <input type="checkbox" className="size-[15px] shrink-0 accent-accent" checked={enter} onChange={(event) => setEnter(event.target.checked)} />
          {t("keyBar.customEnter")}
        </label>
        <div className="flex justify-end gap-2">
          <Button onClick={onCancel}>{t("keyBar.cancel")}</Button>
          <Button kind="primary" type="submit">{initial === undefined ? t("keyBar.customAdd") : t("keyBar.customSave")}</Button>
        </div>
      </form>
    </ModalShell>
  );
}
