import { useTranslate } from "../i18n/context";

// この接続がVPNプロファイルの経路を通ることを、接続を見ている場所で示す小さな札。
// Terminal と SFTP のどちらからも同じ言い方で出す。札は利用者が付けた経路の名前を持ち長さが決まらないので、
// 置き場所の幅に合わせて縮み、入りきらない分を「…」にする（全文は title で読める）。縮まないと、狭い
// ペインの見出しで右のボタンに重なる。
export function VPNProfileChip({ name, className = "" }: { name: string; className?: string }) {
  const t = useTranslate();
  if (name === "") return null;
  return (
    <span
      title={t("vpn.profileChipHint", { name })}
      className={`min-w-0 truncate rounded-sm border border-line px-1.5 py-0.5 text-3xs font-medium text-ink-muted ${className}`}
    >
      {t("vpn.profileChip", { name })}
    </span>
  );
}
