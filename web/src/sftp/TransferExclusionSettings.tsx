import { CodeField } from "../editor/CodeField";
import { useTranslate } from "../i18n/context";
import type { DescribableControlProps } from "../ui/describeControl";
import { FieldRow } from "../ui/FieldRow";
import { useTypedSetting, type ParsedSetting } from "../settings/useTypedSetting";
import { maximumExclusionPatterns, maximumExclusionPatternBytes, validTransferExclusionPatterns } from "./transferExclusions";

function rulesIn(text: string): string[] {
  return text.split(/\r?\n/).map((line) => line.trim()).filter((line) => line !== "");
}

// The folder exclusion rules: one row with the rules and how they are written. The draft takes them only while every
// rule is valid: a half-typed rule must not apply to a transfer.
export function TransferExclusionSettings({ patterns, onChange }: {
  patterns: readonly string[];
  onChange: (patterns: string[]) => void;
}) {
  const t = useTranslate();
  const typed = useTypedSetting<string[]>({
    text: patterns.join("\n"),
    parse: (text): ParsedSetting<string[]> => {
      const rules = rulesIn(text);
      return validTransferExclusionPatterns(rules) ? { value: rules } : { problem: t("sftp.manager.exclusionsInvalid") };
    },
    onChange,
  });

  return (
    <FieldRow
      label={<>{t("sftp.manager.exclusionsLabel")}<small className="mt-[3px] block text-xs font-normal leading-[17px] text-ink-faint">{t("sftp.manager.exclusionsLabelNote")}</small></>}
      hint={t("sftp.manager.exclusionsSyntax")}
      error={typed.problem === "" ? undefined : typed.problem}
      interactiveChildren
    >
      <PatternsEditor text={typed.shown} onTextChange={typed.type} onLeave={typed.leave} />
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
    <CodeField
      label={t("sftp.manager.exclusionsPatterns")}
      {...description}
      value={text}
      minLines={4}
      maxLines={12}
      onChange={(next) => onTextChange(next.slice(0, maximumExclusionPatterns * (maximumExclusionPatternBytes + 1)))}
      onBlur={onLeave}
    />
  );
}
