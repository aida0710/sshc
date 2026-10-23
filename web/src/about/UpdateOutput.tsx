import { useTranslate } from "../i18n/context";
import { CommandOutput } from "../ui/CommandOutput";
import type { UpdateOutputState } from "./useUpdateOutput";

// UpdateOutput shows what the installer and the engine restart wrote, under
// the steps the page reports. Nothing is drawn until the first line.
export function UpdateOutput({ output, reachedCapacity }: UpdateOutputState) {
  const t = useTranslate();
  if (output === "") return null;
  return (
    <div className="grid gap-1">
      <p aria-hidden="true" className="text-xs font-medium text-ink-muted">{t("about.updateOutput")}</p>
      <CommandOutput output={output} label={t("about.updateOutput")} height="tall" />
      {reachedCapacity ? <p className="text-xs text-ink-muted">{t("about.updateOutputCapped")}</p> : null}
    </div>
  );
}
