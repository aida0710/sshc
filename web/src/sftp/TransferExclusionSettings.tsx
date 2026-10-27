import { useEffect, useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import type { DescribableControlProps } from "../ui/describeControl";
import { FieldRow } from "../ui/FieldRow";
import { control } from "../ui/form";
import { Button } from "../ui/surface";
import { usePendingResult } from "../ui/usePendingResult";
import type { TransferSettings } from "./api";
import { maximumExclusionPatterns, maximumExclusionPatternBytes, validTransferExclusionPatterns } from "./transferExclusions";

// The folder exclusion rules: one row with the rules, their own save button under them, and how they are
// written. Unlike the other transfer settings they are saved only with that button, since a half-typed rule
// must not apply to a transfer.
export function TransferExclusionSettings({ patterns, onCommit }: {
  patterns: readonly string[];
  onCommit: (settings: Partial<TransferSettings>) => Promise<unknown>;
}) {
  const t = useTranslate();
  const stored = patterns.join("\n");
  const [draft, setDraft] = useState<string | null>(null);
  const applying = usePendingResult();
  useEffect(() => setDraft(null), [stored]);
  const text = draft ?? stored;
  const parsed = text.split(/\r?\n/).map((line) => line.trim()).filter((line) => line !== "");
  const valid = validTransferExclusionPatterns(parsed);
  const changed = parsed.join("\n") !== stored;

  return (
    <FieldRow
      label={<>{t("sftp.manager.exclusionsLabel")}<small className="mt-[3px] block text-xs font-normal leading-[17px] text-ink-faint">{t("sftp.manager.exclusionsLabelNote")}</small></>}
      hint={t("sftp.manager.exclusionsSyntax")}
      warning={valid ? undefined : t("sftp.manager.exclusionsInvalid")}
      interactiveChildren
    >
      <PatternsEditor
        text={text}
        onTextChange={setDraft}
        saveButton={(
          <Button busy={applying.pending} disabled={!valid || !changed} onClick={() => applying.watch(onCommit({ excludePatterns: parsed }))}>
            {t("sftp.manager.exclusionsApply")}
          </Button>
        )}
      />
    </FieldRow>
  );
}

// PatternsEditor is the box of rules with the save button under it. The row's description (how rules are
// written, what is wrong with them) belongs to the box, so it is passed on to the box.
function PatternsEditor({ text, onTextChange, saveButton, ...description }: {
  text: string;
  onTextChange: (text: string) => void;
  saveButton: ReactNode;
} & DescribableControlProps) {
  const t = useTranslate();
  return (
    <span className="flex min-w-0 flex-col items-start gap-2">
      <textarea
        aria-label={t("sftp.manager.exclusionsPatterns")}
        {...description}
        value={text}
        rows={3}
        maxLength={maximumExclusionPatterns * (maximumExclusionPatternBytes + 1)}
        onChange={(event) => onTextChange(event.target.value)}
        className={`${control} min-h-[92px] resize-y font-mono`}
      />
      {saveButton}
    </span>
  );
}
