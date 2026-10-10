import { useTranslate } from "../i18n/context";

export function TransferExclusionSummary({ patterns = [] }: { patterns?: readonly string[] }) {
  const t = useTranslate();
  if (patterns.length === 0) return null;
  return <span className="text-xs text-notice-ink" title={`${t("sftp.manager.exclusionsApplied")}\n${patterns.join("\n")}`}>
    {t("sftp.manager.exclusionsCount", { count: patterns.length })}
  </span>;
}
