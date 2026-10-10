import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import type { TransferSettings } from "./api";
import { maximumExclusionPatterns, maximumExclusionPatternBytes, validTransferExclusionPatterns } from "./transferExclusions";

export function TransferExclusionSettings({ patterns, onCommit, defaultOpen = false }: {
  patterns: readonly string[];
  onCommit: (settings: Partial<TransferSettings>) => void;
  defaultOpen?: boolean;
}) {
  const t = useTranslate();
  const stored = patterns.join("\n");
  const [draft, setDraft] = useState<string | null>(null);
  useEffect(() => setDraft(null), [stored]);
  const text = draft ?? stored;
  const parsed = text.split(/\r?\n/).map((line) => line.trim()).filter((line) => line !== "");
  const valid = validTransferExclusionPatterns(parsed);
  const changed = parsed.join("\n") !== stored;
  return <details open={defaultOpen || undefined} className="min-w-0 max-w-full text-ink-muted">
    <DisclosureSummary className="min-h-11 text-sm font-medium">{patterns.length === 0 ? t("sftp.manager.exclusions") : t("sftp.manager.exclusionsCount", { count: patterns.length })}</DisclosureSummary>
    <div className="mt-2 flex flex-col gap-3 text-sm">
      <p>{t("sftp.manager.exclusionsHint")}</p>
      <label className="flex flex-col gap-1">
        {t("sftp.manager.exclusionsPatterns")}
        <textarea value={text} rows={3} maxLength={maximumExclusionPatterns * (maximumExclusionPatternBytes + 1)}
          onChange={(event) => setDraft(event.target.value)}
          className="w-full resize-y rounded-md border border-control-line bg-control p-3 font-mono text-ink" />
      </label>
      <p>{t("sftp.manager.exclusionsSyntax")}</p>
      {!valid ? <p role="alert" className="text-danger">{t("sftp.manager.exclusionsInvalid")}</p> : null}
      <button type="button" className="min-h-11 self-start rounded-md border border-control-line px-3 py-2 disabled:opacity-50"
        disabled={!valid || !changed} onClick={() => onCommit({ excludePatterns: parsed })}>
        {t("sftp.manager.exclusionsApply")}
      </button>
    </div>
  </details>;
}
