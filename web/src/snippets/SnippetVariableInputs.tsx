import { useTranslate } from "../i18n/context";
import { PasswordInput } from "../ui/PasswordField";
import type { SnippetVariable } from "./api";

const variableInputClass =
  "mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm text-ink";

// SnippetVariableInputs は、スニペットの変数ごとに、型に合った入力欄を並べる。
// 真偽値は選択、シークレットはパスワードの欄、ほかは文字（整数なら数値）の欄である。
// 空の欄は既定値を使うことを、欄の中の表示で示す。
export function SnippetVariableInputs({
  variables,
  inputs,
  onChange,
}: {
  variables: SnippetVariable[];
  inputs: Record<string, string>;
  onChange: (variable: SnippetVariable, value: string) => void;
}) {
  const t = useTranslate();
  return (
    <>
      {variables.map((variable) => {
        const value = inputs[variable.name] ?? "";
        const defaultPlaceholder =
          variable.default === undefined ? "" : t("workspace.useDefaultValue", { value: variable.default });
        return (
          <div key={variable.name} className="text-xs text-ink-muted">
            <span>
              <code>{`{{${variable.name}}}`}</code>
              {variable.description ? ` · ${variable.description}` : ""}
            </span>
            {variable.type === "boolean" ? (
              <select
                aria-label={variable.name}
                value={value}
                onChange={(event) => onChange(variable, event.target.value)}
                className={variableInputClass}
              >
                <option value="">
                  {variable.default === undefined ? "" : t("workspace.useDefaultOption", { value: variable.default })}
                </option>
                <option value="true">true</option>
                <option value="false">false</option>
              </select>
            ) : variable.type === "secret" ? (
              <PasswordInput
                label={variable.name}
                value={value}
                placeholder={defaultPlaceholder}
                onChange={(next) => onChange(variable, next)}
                className={variableInputClass}
              />
            ) : (
              <input
                aria-label={variable.name}
                type={variable.type === "integer" ? "number" : "text"}
                value={value}
                placeholder={defaultPlaceholder}
                onChange={(event) => onChange(variable, event.target.value)}
                className={variableInputClass}
              />
            )}
          </div>
        );
      })}
    </>
  );
}
