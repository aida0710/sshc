import { useRef, useState } from "react";
import { useTranslate, type Translate } from "../i18n/context";
import { control, dialogHeading, Field } from "../ui/form";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { literalTextEntry } from "../ui/softKeyboard";
import { keyBarLabelForbiddenPattern, keyBarTextForbiddenPattern, maxKeyBarLabelLength, maxKeyBarTextLength } from "../rules/keyBar.generated";
import type { KeyBarKey } from "./keySequences";
import { ActionArea } from "../ui/ActionArea";

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
      panelClassName="flex w-full max-w-md flex-col gap-4 p-4 sm:p-5">
      <h2 id="custom-key-heading" className={dialogHeading}>{t("keyBar.customHeading")}</h2>
      <form className="flex flex-col gap-4" onSubmit={(event) => { event.preventDefault(); submit(); }}>
        {/* 誤りは確定のボタンを押したあとだけ出す。補足は誤りのときも残し、誤りの文はその下に出す。 */}
        <Field label={t("keyBar.customLabel")} hint={t("keyBar.customLabelHint", { max: String(maxKeyBarLabelLength) })} error={touched ? problem.label : ""}>
          <input {...literalTextEntry} ref={labelRef} value={label} className={`${control} font-mono`}
            onChange={(event) => setLabel(event.target.value)} />
        </Field>
        <Field label={t("keyBar.customText")} hint={t("keyBar.customTextHint")} error={touched ? problem.text : ""}>
          <textarea {...literalTextEntry} rows={3} value={text} className={`${control} min-h-20 font-mono`}
            onChange={(event) => setText(event.target.value)} />
        </Field>
        <label className="flex items-center gap-2 text-sm text-ink">
          <input type="checkbox" className="size-[15px] shrink-0 accent-accent" checked={enter} onChange={(event) => setEnter(event.target.checked)} />
          {t("keyBar.customEnter")}
        </label>
        <ActionArea placement="inline">
          <Button onClick={onCancel}>{t("keyBar.cancel")}</Button>
          <Button kind="primary" type="submit">{initial === undefined ? t("keyBar.customAdd") : t("keyBar.customSave")}</Button>
        </ActionArea>
      </form>
    </ModalShell>
  );
}
