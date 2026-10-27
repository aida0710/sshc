import type { SnippetVariable } from "./api";
import { SnippetVariableValue } from "./SnippetVariableValue";

const variableInputClass =
  "mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm text-ink";

// SnippetVariableInputs は、スニペットの変数ごとに、名前と説明の下へ値の欄を並べる。
export function SnippetVariableInputs({
  variables,
  inputs,
  onChange,
}: {
  variables: SnippetVariable[];
  inputs: Record<string, string>;
  onChange: (variable: SnippetVariable, value: string) => void;
}) {
  return (
    <>
      {variables.map((variable) => (
        <div key={variable.name} className="text-xs text-ink-muted">
          <span>
            <code>{`{{${variable.name}}}`}</code>
            {variable.description ? ` · ${variable.description}` : ""}
          </span>
          <SnippetVariableValue
            variable={variable}
            value={inputs[variable.name] ?? ""}
            label={variable.name}
            className={variableInputClass}
            onChange={(value) => onChange(variable, value)}
          />
        </div>
      ))}
    </>
  );
}
