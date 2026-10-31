import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import type { DescribableControlProps } from "../ui/describeControl";
import { FieldRow } from "../ui/FieldRow";
import { control } from "../ui/form";
import type { TransferSettings } from "./api";
import { maximumExclusionPatterns, maximumExclusionPatternBytes, validTransferExclusionPatterns } from "./transferExclusions";

// The folder exclusion rules: one row with the rules and how they are written. They are saved when the box is
// left, as the other typed settings are, and only when every rule is valid: a half-typed rule must not apply to
// a transfer.
export function TransferExclusionSettings({ patterns, onCommit }: {
  patterns: readonly string[];
  onCommit: (settings: Partial<TransferSettings>) => Promise<unknown>;
}) {
  const t = useTranslate();
  const stored = patterns.join("\n");
  const [draft, setDraft] = useState<string | null>(null);
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
        onLeave={() => {
          if (valid && changed) void onCommit({ excludePatterns: parsed });
        }}
      />
    </FieldRow>
  );
}

// PatternsEditor is the box of rules. The row's description (how rules are written, what is wrong with them)
// belongs to the box, so it is passed on to the box.
function PatternsEditor({ text, onTextChange, onLeave, ...description }: {
  text: string;
  onTextChange: (text: string) => void;
  onLeave: () => void;
} & DescribableControlProps) {
  const t = useTranslate();
  return (
    <textarea
      aria-label={t("sftp.manager.exclusionsPatterns")}
      {...description}
      value={text}
      rows={3}
      maxLength={maximumExclusionPatterns * (maximumExclusionPatternBytes + 1)}
      onChange={(event) => onTextChange(event.target.value)}
      onBlur={onLeave}
      className={`${control} min-h-[92px] resize-y font-mono`}
    />
  );
}
