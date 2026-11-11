import { useTranslate } from "../i18n/context";
import { ActionMenu } from "../ui/ActionMenu";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { literalTextEntry } from "../ui/softKeyboard";
import { Button } from "../ui/surface";
import type { SnippetStep } from "./api";
import { newStep } from "./snippetKind";

// stepColumns lays a step out on one line once the steps have the room: what to wait for, what to send, then its
// switches and menu. Narrower, each step takes three lines, each field with its name above it, and a line between steps.
const stepColumns = "@xl/steps:grid-cols-[minmax(0,11rem)_minmax(0,1fr)_auto] @xl/steps:items-center";
// fieldCaption names a field above it while the steps take three lines; on one line the column heads name them.
const fieldCaption = "text-xs text-ink-faint @xl/steps:sr-only";

// SnippetStepsEditor は、手順のスニペットの手順を並べて編集する。1行が1つの手順で、待つ文字列・送る文字列・
// Enter・省略可と、並べ替えと削除の「…」を持つ。
export function SnippetStepsEditor({ steps, onChange }: { steps: SnippetStep[]; onChange: (next: SnippetStep[]) => void }) {
  const t = useTranslate();
  const change = (index: number, update: Partial<SnippetStep>) =>
    onChange(steps.map((step, at) => (at === index ? { ...step, ...update } : step)));
  const move = (index: number, to: number) => {
    const next = [...steps];
    const [moved] = next.splice(index, 1);
    if (moved !== undefined) next.splice(to, 0, moved);
    onChange(next);
  };
  return (
    <div className="@container/steps flex flex-col gap-2">
      <div aria-hidden="true" className={`hidden gap-2 text-xs text-ink-faint @xl/steps:grid ${stepColumns}`}>
        <span>{t("snippets.stepWait")}</span>
        <span>{t("snippets.stepSend")}</span>
        <span />
      </div>
      <ol className="flex flex-col gap-3 @xl/steps:gap-2">
        {steps.map((step, index) => {
          const number = index + 1;
          return (
            <li key={index} className={`grid gap-1.5 border-t border-line pt-3 first:border-t-0 first:pt-0 @xl/steps:border-t-0 @xl/steps:pt-0 ${stepColumns}`}>
              <label className="flex min-w-0 flex-col gap-1">
                <span aria-hidden="true" className={fieldCaption}>{t("snippets.stepWait")}</span>
                <input
                  {...literalTextEntry}
                  value={step.wait ?? ""}
                  placeholder={t("snippets.stepNoWait")}
                  aria-label={t("snippets.stepWaitOf", { number })}
                  onChange={(event) => change(index, { wait: event.target.value, ...(event.target.value === "" ? { optional: false } : {}) })}
                  className={control}
                />
              </label>
              <label className="flex min-w-0 flex-col gap-1">
                <span aria-hidden="true" className={fieldCaption}>{t("snippets.stepSend")}</span>
                <input
                  {...literalTextEntry}
                  value={step.send}
                  aria-label={t("snippets.stepSendOf", { number })}
                  onChange={(event) => change(index, { send: event.target.value })}
                  className={`${control} font-mono`}
                />
              </label>
              <div className="flex items-center gap-3 text-sm text-ink">
                <label className="inline-flex items-center gap-1.5">
                  <input type="checkbox" checked={step.enter} onChange={(event) => change(index, { enter: event.target.checked })} aria-label={t("snippets.stepEnterOf", { number })} />
                  <span aria-hidden="true">{t("snippets.stepEnter")}</span>
                </label>
                <label className="inline-flex items-center gap-1.5" title={t("snippets.stepOptionalHint")}>
                  <input
                    type="checkbox"
                    checked={step.optional === true}
                    disabled={(step.wait ?? "") === ""}
                    onChange={(event) => change(index, { optional: event.target.checked })}
                    aria-label={t("snippets.stepOptionalOf", { number })}
                  />
                  <span aria-hidden="true">{t("snippets.stepOptional")}</span>
                </label>
                <span className="flex-1" />
                <ActionMenu
                  label={t("snippets.stepActions", { number })}
                  triggerClassName="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink md:size-7"
                  items={[
                    { label: t("snippets.moveStepUp"), onSelect: () => move(index, index - 1), disabled: index === 0 },
                    { label: t("snippets.moveStepDown"), onSelect: () => move(index, index + 1), disabled: index === steps.length - 1 },
                    { label: t("snippets.removeStep"), tone: "danger", onSelect: () => onChange(steps.filter((_, at) => at !== index)) },
                  ]}
                />
              </div>
            </li>
          );
        })}
      </ol>
      <Button className="inline-flex w-fit items-center gap-1.5" onClick={() => onChange([...steps, newStep()])}>
        <Icon name="plus" className="size-4" />
        {t("snippets.addStep")}
      </Button>
    </div>
  );
}
