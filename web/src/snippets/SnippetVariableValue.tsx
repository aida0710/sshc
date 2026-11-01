import { useTranslate } from "../i18n/context";
import { PasswordInput } from "../ui/PasswordField";
import type { SnippetVariable } from "./api";
import { literalTextEntry } from "../ui/softKeyboard";

// SnippetVariableValue は、変数1つの値の欄である。真偽値は選択、シークレットはパスワードの欄、
// ほかは文字（整数なら数値）の欄である。空の欄は既定値を使うことを、欄の中の表示で示す。
export function SnippetVariableValue({
  variable,
  value,
  label,
  className,
  onChange,
}: {
  variable: SnippetVariable;
  value: string;
  label: string;
  className: string;
  onChange: (value: string) => void;
}) {
  const t = useTranslate();
  const defaultPlaceholder =
    variable.default === undefined ? "" : t("workspace.useDefaultValue", { value: variable.default });
  if (variable.type === "boolean") {
    return (
      <select aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} className={className}>
        <option value="">
          {variable.default === undefined ? "" : t("workspace.useDefaultOption", { value: variable.default })}
        </option>
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
    );
  }
  if (variable.type === "secret") {
    return <PasswordInput label={label} value={value} placeholder={defaultPlaceholder} onChange={onChange} className={className} />;
  }
  return (
    <input {...literalTextEntry}
      aria-label={label}
      type={variable.type === "integer" ? "number" : "text"}
      value={value}
      placeholder={defaultPlaceholder}
      onChange={(event) => onChange(event.target.value)}
      className={className}
    />
  );
}
