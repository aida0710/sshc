import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { control } from "../ui/form";
import { PageSection } from "../ui/page";
import { Button } from "../ui/surface";
import type { SnippetDraft, SnippetVariable } from "./api";
import { snippetVariableTypeLabelKey } from "./snippetLabels";
import { SnippetVariableValue } from "./SnippetVariableValue";

const variableTypes = ["string", "integer", "boolean", "secret"] as const;

// SnippetEditor は、開いているスニペットの名前・説明・コマンドと、コマンドに書いた変数を編集する節である。
// 変数の行では型と、確認・実行・自動実行に渡す値を入れる。
export function SnippetEditor({
  heading,
  draft,
  variables,
  inputs,
  savedJustNow,
  deletable,
  saving,
  deleting,
  busy,
  onDraftChange,
  onVariableChange,
  onInputChange,
  onSave,
  onDelete,
}: {
  heading: string;
  draft: SnippetDraft;
  // variables は、コマンドに書いた変数の定義（variablesFor で今のコマンドに合わせたもの）である。
  variables: SnippetVariable[];
  inputs: Record<string, string>;
  savedJustNow: boolean;
  // deletable は、保存済みのスニペットを開いているかである。新しいスニペットには削除が無い。
  deletable: boolean;
  saving: boolean;
  deleting: boolean;
  // busy は、このページのどれかの操作を待っている間である。終わるまでボタンを止める。
  busy: boolean;
  onDraftChange: (next: SnippetDraft) => void;
  onVariableChange: (name: string, update: Partial<SnippetVariable>) => void;
  onInputChange: (name: string, value: string) => void;
  onSave: () => void;
  onDelete: () => void;
}) {
  const t = useTranslate();
  return (
    <PageSection heading={heading}>
      {/* The rows sit in their own block, so the first one has no line above it under the heading. */}
      <div>
        <FieldRow label={t("snippets.name")}>
          <input value={draft.name} onChange={(event) => onDraftChange({ ...draft, name: event.target.value })} className={control} />
        </FieldRow>
        <FieldRow label={t("snippets.description")}>
          <input value={draft.description} onChange={(event) => onDraftChange({ ...draft, description: event.target.value })} className={control} />
        </FieldRow>
        <FieldRow label={t("snippets.command")} hint={t("snippets.variableHint")}>
          <textarea
            rows={3}
            value={draft.command}
            onChange={(event) => onDraftChange({ ...draft, command: event.target.value })}
            className={`${control} resize-y font-mono text-[12.5px]`}
          />
        </FieldRow>
        {variables.map((variable) => (
          <FieldRow key={variable.name} label={<code className="font-mono text-[12.5px]">{`{{${variable.name}}}`}</code>} interactiveChildren>
            <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
              <select
                aria-label={`${variable.name} · ${t("snippets.variableType")}`}
                value={variable.type}
                onChange={(event) => onVariableChange(variable.name, { type: event.target.value as SnippetVariable["type"] })}
                className={`${control} sm:w-36 sm:shrink-0`}
              >
                {variableTypes.map((type) => (
                  <option key={type} value={type}>{t(snippetVariableTypeLabelKey(type))}</option>
                ))}
              </select>
              <SnippetVariableValue
                variable={variable}
                value={inputs[variable.name] ?? ""}
                label={`${variable.name} · ${t("snippets.value")}`}
                className={control}
                onChange={(value) => onInputChange(variable.name, value)}
              />
            </div>
          </FieldRow>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-2 pt-1">
        {deletable ? (
          <Button kind="dangerText" busy={deleting} disabled={busy} onClick={onDelete}>
            {t("snippets.delete")}
          </Button>
        ) : null}
        <span className="flex-1" />
        {savedJustNow ? <p role="status" className="text-xs text-live">{t("snippets.saved")}</p> : null}
        <Button
          kind="primary"
          busy={saving}
          disabled={busy || draft.name.trim() === "" || draft.command === ""}
          onClick={onSave}
        >
          {t("snippets.save")}
        </Button>
      </div>
    </PageSection>
  );
}
