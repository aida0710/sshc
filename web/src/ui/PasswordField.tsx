import { useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import type { DescribableControlProps } from "./describeControl";
import { Field, control } from "./form";

type PasswordFieldProps = {
  label: string;
  value: string;
  onChange: (value: string) => void;
  hint?: string;
  // error は、この値を受け取れない理由である。空なら何も出さない。
  error?: string | undefined;
  autoFocus?: boolean;
  disabled?: boolean;
  initialShown?: boolean;
  // inputClassName replaces the field's look, as the lock screen does for its larger fields.
  inputClassName?: string;
};

// Field が結び付ける hint と誤りの id（DescribableControlProps）は、中の <input> へ渡す。
type PasswordInputProps = Omit<PasswordFieldProps, "hint" | "error" | "inputClassName"> & DescribableControlProps & {
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
  "aria-describedby": describedBy,
  "aria-invalid": invalid,
}: PasswordInputProps): ReactNode {
  const t = useTranslate();
  const [shown, setShown] = useState(initialShown);
  // The show button sits inside the field's right end, as in the design.
  return (
    <div className="relative flex w-full min-w-0 items-center">
      <input
        type={shown ? "text" : "password"}
        aria-label={label}
        aria-describedby={describedBy}
        aria-invalid={invalid}
        value={value}
        autoFocus={autoFocus ?? false}
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        disabled={disabled}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        className={`${className} min-w-0 grow pr-14`}
      />
      <button
        type="button"
        disabled={disabled}
        onClick={() => setShown(!shown)}
        aria-pressed={shown}
        aria-label={t(shown ? "password.hideNamed" : "password.showNamed", {
          label,
        })}
        className="absolute right-1 top-1/2 -translate-y-1/2 whitespace-nowrap rounded px-2 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink"
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
  error,
  autoFocus,
  disabled = false,
  initialShown = false,
  inputClassName,
}: PasswordFieldProps): ReactNode {
  return (
    <Field
      label={label}
      interactiveChildren
      {...(hint === undefined ? {} : { hint })}
      error={error}
    >
      <PasswordInput
        label={label}
        value={value}
        onChange={onChange}
        {...(autoFocus === undefined ? {} : { autoFocus })}
        disabled={disabled}
        initialShown={initialShown}
        {...(inputClassName === undefined ? {} : { className: inputClassName })}
      />
    </Field>
  );
}
