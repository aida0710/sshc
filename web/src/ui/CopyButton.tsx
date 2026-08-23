import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { clipboard } from "./clipboard";

type CopyButtonProps = {
  value: string;
  label: MessageKey;
  className?: string;
};

type CopyState = "idle" | "copied" | "failed";

export function CopyButton({ value, label, className }: CopyButtonProps) {
  const t = useTranslate();
  const [state, setState] = useState<CopyState>("idle");

  useEffect(() => {
    setState("idle");
  }, [value]);

  async function copy() {
    try {
      await clipboard.writeText(value);
      setState("copied");
    } catch {
      setState("failed");
    }
  }

  return (
    <span className="inline-flex items-center gap-2">
      <button
        type="button"
        onClick={() => void copy()}
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
