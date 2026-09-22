import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { useClipboardCopy } from "./useClipboardCopy";

type CopyButtonProps = {
  value: string;
  label: MessageKey;
  className?: string;
};

export function CopyButton({ value, label, className }: CopyButtonProps) {
  const t = useTranslate();
  const { state, copy } = useClipboardCopy(value);

  return (
    <span className="inline-flex items-center gap-2">
      <button
        type="button"
        onClick={copy}
        className={className ?? "rounded border border-control-line px-2 py-1 text-xs"}
      >
        {t("copy.button", { label: t(label) })}
      </button>

      <span aria-live="polite" className={state === "failed" ? "text-xs text-danger" : "text-xs text-ink-muted"}>
        {state === "copied" ? t("copy.done") : state === "failed" ? t("copy.refused") : ""}
      </span>
    </span>
  );
}
