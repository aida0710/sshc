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
  initialShown?: boolean;
};

type PasswordInputProps = Omit<PasswordFieldProps, "hint"> & {
  className?: string;
  placeholder?: string;
};

export function PasswordInput({
  label,
  value,
  onChange,
  autoFocus,
  disabled = false,
  initialShown = false,
  className = control,
  placeholder,
}: PasswordInputProps): ReactNode {
  const t = useTranslate();
  const [shown, setShown] = useState(initialShown);
  return (
    <div className="flex w-full min-w-0 items-center gap-2">
      <input
        type={shown ? "text" : "password"}
        aria-label={label}
        value={value}
        autoFocus={autoFocus ?? false}
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        disabled={disabled}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        className={`${className} min-w-0 grow`}
      />
      <button
        type="button"
        disabled={disabled}
        onClick={() => setShown(!shown)}
        aria-pressed={shown}
        aria-label={t(shown ? "password.hideNamed" : "password.showNamed", {
          label,
        })}
        className="whitespace-nowrap rounded border border-control-line px-2 py-1.5 text-xs text-ink-muted hover:bg-select-fill"
      >
        {shown ? t("password.hide") : t("password.show")}
      </button>
    </div>
  );
}

export function PasswordField({
  label,
  value,
  onChange,
  hint,
  autoFocus,
  disabled = false,
  initialShown = false,
}: PasswordFieldProps): ReactNode {
  return (
    <Field
      label={label}
      interactiveChildren
      {...(hint === undefined ? {} : { hint })}
    >
      <PasswordInput
        label={label}
        value={value}
        onChange={onChange}
        {...(autoFocus === undefined ? {} : { autoFocus })}
        disabled={disabled}
        initialShown={initialShown}
      />
    </Field>
  );
}
