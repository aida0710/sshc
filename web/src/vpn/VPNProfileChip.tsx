import { useTranslate } from "../i18n/context";

// この接続がVPNプロファイルの経路を通ることを、接続を見ている場所で示す小さな札。
// Terminal と SFTP のどちらからも同じ言い方で出す。
export function VPNProfileChip({ name, className = "" }: { name: string; className?: string }) {
  const t = useTranslate();
  if (name === "") return null;
  return (
    <span
      title={t("vpn.profileChipHint", { name })}
      className={`shrink-0 rounded border border-line px-1.5 py-0.5 text-[10px] font-medium text-ink-muted ${className}`}
    >
      {t("vpn.profileChip", { name })}
    </span>
  );
}
