import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { control } from "../ui/form";
import { PageSection } from "../ui/page";
import { Button, Segmented } from "../ui/surface";
import type { SnippetDraft, SnippetVariable } from "./api";
import { editsSteps, newStep, stepsCanBeSaved } from "./snippetKind";
import { snippetVariableTypeLabelKey } from "./snippetLabels";
import { SnippetStepsEditor } from "./SnippetStepsEditor";
import { SnippetVariableValue } from "./SnippetVariableValue";
import { CodeField } from "../editor/CodeField";

const variableTypes = ["string", "integer", "boolean", "secret"] as const;

// defaultWaitSeconds is how long a step waits when the snippet says nothing (internal/snippets DefaultWaitSeconds).
const defaultWaitSeconds = 10;
// maxWaitSeconds is the longest a step may wait (internal/snippets MaxWaitSeconds).
const maxWaitSeconds = 300;

type SnippetKind = "command" | "steps";

// asKind turns a draft into the other kind and carries over what it says: a command becomes the first step, and the
// steps become a command of their lines, which a shell types one by one.
function asKind(draft: SnippetDraft, kind: SnippetKind): SnippetDraft {
  if (kind === "steps") {
    const { waitSeconds: _waitSeconds, ...rest } = draft;
    return { ...rest, command: "", steps: [newStep(draft.command)] };
  }
  const { steps = [], waitSeconds: _waitSeconds, ...rest } = draft;
  return { ...rest, command: steps.map((step) => step.send).filter((send) => send !== "").join("\n") };
}

// validWaitSeconds accepts a whole number of seconds the engine takes; none is the default.
function validWaitSeconds(seconds: number | undefined): boolean {
  return seconds === undefined || (Number.isInteger(seconds) && seconds >= 0 && seconds <= maxWaitSeconds);
}

// canBeSaved reports whether the engine takes the draft: a name, and a command or steps that each type something.
function canBeSaved(draft: SnippetDraft): boolean {
  if (draft.name.trim() === "") return false;
  return editsSteps(draft) ? stepsCanBeSaved(draft.steps ?? []) && validWaitSeconds(draft.waitSeconds) : draft.command !== "";
}
// SnippetEditor は、開いているスニペットの名前・説明・コマンド（または手順）と、そこに書いた変数を編集する節である。
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
  const steps = editsSteps(draft);
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
        <FieldRow label={t("snippets.kind")} hint={steps ? t("snippets.kindStepsHint") : undefined} interactiveChildren>
          <Segmented
            label={t("snippets.kind")}
            value={steps ? "steps" : "command"}
            options={[
              { value: "command", label: t("snippets.kindCommand") },
              { value: "steps", label: t("snippets.kindSteps") },
            ]}
            onChange={(kind) => {
              if (kind !== (steps ? "steps" : "command")) onDraftChange(asKind(draft, kind));
            }}
          />
        </FieldRow>
        {steps ? (
          <>
            <FieldRow label={t("snippets.steps")} hint={t("snippets.stepsVariableHint")} interactiveChildren>
              <SnippetStepsEditor steps={draft.steps ?? []} onChange={(next) => onDraftChange({ ...draft, steps: next })} />
            </FieldRow>
            <FieldRow
              label={t("snippets.waitSeconds")}
              hint={t("snippets.waitSecondsHint")}
              warning={validWaitSeconds(draft.waitSeconds) ? undefined : t("snippets.waitSecondsRange", { max: maxWaitSeconds })}
            >
              <span className="flex items-center gap-2">
                {/* An empty field is 0, which the engine takes for the default, shown as the placeholder. */}
                <span className="w-24 shrink-0">
                  <input
                    type="number"
                    min={1}
                    max={maxWaitSeconds}
                    step={1}
                    aria-label={t("snippets.waitSeconds")}
                    placeholder={String(defaultWaitSeconds)}
                    value={draft.waitSeconds === 0 ? "" : (draft.waitSeconds ?? defaultWaitSeconds)}
                    onChange={(event) => onDraftChange({ ...draft, waitSeconds: event.target.value === "" ? 0 : Number(event.target.value) })}
                    className={control}
                  />
                </span>
                <span className="text-sm text-ink-muted">{t("snippets.waitSecondsUnit")}</span>
              </span>
            </FieldRow>
          </>
        ) : (
          <FieldRow label={t("snippets.command")} hint={t("snippets.variableHint")} interactiveChildren>
            <CodeField
              label={t("snippets.command")}
              language="shell"
              value={draft.command}
              minLines={3}
              maxLines={12}
              onChange={(command) => onDraftChange({ ...draft, command })}
            />
          </FieldRow>
        )}
        {variables.map((variable) => (
          <FieldRow key={variable.name} label={<code className="font-mono text-xs">{`{{${variable.name}}}`}</code>} interactiveChildren>
            {/* The type and the value share a line while the value keeps 13rem; past that each takes a line of
                its own and the whole width. The value grows far faster, so on a shared line the type stays 9rem. */}
            <div className="flex min-w-0 flex-wrap gap-2">
              <select
                aria-label={`${variable.name} · ${t("snippets.variableType")}`}
                value={variable.type}
                onChange={(event) => onVariableChange(variable.name, { type: event.target.value as SnippetVariable["type"] })}
                className={`${control} min-w-0 flex-[1_1_9rem]`}
              >
                {variableTypes.map((type) => (
                  <option key={type} value={type}>{t(snippetVariableTypeLabelKey(type))}</option>
                ))}
              </select>
              <div className="flex min-w-0 flex-[999_1_13rem] flex-col">
                <SnippetVariableValue
                  variable={variable}
                  value={inputs[variable.name] ?? ""}
                  label={`${variable.name} · ${t("snippets.value")}`}
                  className={control}
                  onChange={(value) => onInputChange(variable.name, value)}
                />
              </div>
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
          disabled={busy || !canBeSaved(draft)}
          onClick={onSave}
        >
          {t("snippets.save")}
        </Button>
      </div>
    </PageSection>
  );
}
