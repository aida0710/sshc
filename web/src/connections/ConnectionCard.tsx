import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { tagClass } from "../ui/Tag";

// 接続先のカードの形。Home の一覧（QuickConnectBrowser）と、SFTP の何も開いていないペイン（SFTPQuickHosts）が
// 同じ形で描く。押したときの動きは使う側が決める。

// connectionCardGrid は、カードを幅 330px までの列に、入るだけ並べる。
export const connectionCardGrid = "grid grid-cols-[repeat(auto-fill,minmax(min(100%,330px),1fr))] gap-2.5";

// connectionCardClass は、カードの枠。chosen は、選んでいるカードと、開いている途中のカード。
export function connectionCardClass(chosen: boolean): string {
  return `group/row relative min-w-0 rounded-lg border transition-colors ${
    chosen ? "border-accent bg-select-fill" : "border-transparent bg-well hover:border-accent/45"
  }`;
}

// connectionCardButton は、カードの中の押す所。印、名前と住所、右端の順に横へ並べる。
export const connectionCardButton = "flex w-full min-w-0 items-center gap-3 rounded-lg p-3.5 text-left";

// connectionCardGroup は、右端に出すグループの名前。
export const connectionCardGroup = "max-w-[30%] shrink-0 truncate text-2xs text-ink-faint";

// LastConnected は、名前の横の札に出す最後に接続した日時（at）と、その読み上げ（label）。
export type LastConnected = { at: string; label: string };

// ConnectionCardBody は、カードの中身。印、名前の行（名前、最後に接続した日時の札、名前の重なりの印）、
// 住所の行、状態の行（status）、右端（end）。最後に接続した日時は名前の横の札にするので、札のあるカードも
// 無いカードも同じ高さになる。
export function ConnectionCardBody({ icon, name, lastConnected, duplicateAlias = false, address, status, end }: {
  icon: ReactNode;
  name: string;
  lastConnected: LastConnected | null;
  duplicateAlias?: boolean;
  address: string;
  status?: ReactNode;
  end?: ReactNode;
}) {
  const t = useTranslate();
  return (
    <>
      {icon}
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-1.5">
          <span className="truncate text-sm font-semibold text-ink" title={name}>{name}</span>
          {lastConnected === null ? null : (
            <span title={lastConnected.label} aria-label={lastConnected.label} className={tagClass("accent")}>{lastConnected.at}</span>
          )}
          {duplicateAlias ? <span role="img" aria-label={t("browser.duplicateAlias")} className="text-notice-ink"><Icon name="warning" className="size-3.5" /></span> : null}
        </span>
        <span className="mt-0.5 block truncate font-mono text-xs text-ink-faint" title={address}>{address}</span>
        {status}
      </span>
      {end}
    </>
  );
}
