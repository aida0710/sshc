import { useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { Field, control } from "./form";

type PasswordFieldProps = {
  label: string;
  value: string;
  onChange: (value: string) => void;
  hint?: string;
  autoFocus?: boolean;
  disabled?: boolean;
};

export function PasswordField({ label, value, onChange, hint, autoFocus, disabled = false }: PasswordFieldProps): ReactNode {
  const t = useTranslate();
  const [shown, setShown] = useState(false);
  return (
    <div className="flex items-end gap-2">
      <div className="grow">
        <Field label={label} {...(hint === undefined ? {} : { hint })}>
          <input
            type={shown ? "text" : "password"}
            value={value}
            autoFocus={autoFocus ?? false}
            disabled={disabled}
            onChange={(event) => onChange(event.target.value)}
            className={control}
          />
        </Field>
      </div>

      <button
        type="button"
        disabled={disabled}
        onClick={() => setShown(!shown)}
        aria-pressed={shown}
        aria-label={t(shown ? "password.hideNamed" : "password.showNamed", { label })}
        className="whitespace-nowrap rounded border border-control-line px-2 py-1.5 text-xs text-ink-muted hover:bg-select-fill"
      >
        {shown ? t("password.hide") : t("password.show")}
      </button>
    </div>
  );
}
