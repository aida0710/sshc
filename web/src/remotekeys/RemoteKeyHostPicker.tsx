import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { asciiTextEntry } from "../ui/softKeyboard";

// 鍵を登録する接続先を選ぶ欄。検索の欄・選択中の件数・［一致する項目を選択］［解除］を1行に置き、
// その下に保存済みの接続先を3列のチェックで全部並べる。検索の欄には、保存していない alias を
// そのまま入力してもよい（何も選んでいなければ、その alias を接続先にする）。

// quietAction は、枠も地も無い文字だけのボタンである。行の端の補助の操作に使う。
const quietAction =
  "h-8 whitespace-nowrap rounded-md px-2 text-[13px] text-ink-muted transition-colors duration-150 hover:bg-hover hover:text-ink";

export function RemoteKeyHostPicker({
  query,
  hostOptions,
  matchingHosts,
  selectedAliases,
  busy,
  onQueryChange,
  onToggle,
  onSelectMatches,
  onClear,
}: {
  query: string;
  // hostOptions は保存済みの接続先の全部、matchingHosts はそのうち検索に一致するものである。
  hostOptions: readonly string[];
  matchingHosts: readonly string[];
  selectedAliases: readonly string[];
  busy: boolean;
  onQueryChange: (query: string) => void;
  onToggle: (alias: string) => void;
  onSelectMatches: () => void;
  onClear: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="relative block w-full sm:w-80">
          <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint" />
          <input {...asciiTextEntry}
            type="search"
            aria-label={t("rk.hostAlias")}
            value={query}
            disabled={busy}
            placeholder={t("rk.hostSearchPlaceholder")}
            onChange={(event) => onQueryChange(event.target.value)}
            className={`${control} pl-8`}
          />
        </span>
        {hostOptions.length === 0 ? null : (
          <div className="ml-auto flex flex-wrap items-center gap-1">
            <span className="mr-1 text-[12.5px] text-ink-muted">
              {t("rk.hostsSelected", { count: selectedAliases.length })}
            </span>
            <button type="button" disabled={busy || matchingHosts.length === 0} onClick={onSelectMatches} className={quietAction}>
              {t("rk.selectMatches")}
            </button>
            <button type="button" disabled={busy || selectedAliases.length === 0} onClick={onClear} className={quietAction}>
              {t("rk.clearSelection")}
            </button>
          </div>
        )}
      </div>
      {hostOptions.length === 0 ? (
        <p className="text-xs text-ink-faint">{t("rk.hostTypeHint")}</p>
      ) : matchingHosts.length === 0 ? (
        <p className="text-xs text-ink-faint">{t("rk.noHostMatches")}</p>
      ) : (
        <div role="group" aria-label={t("rk.hostChoices")} className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3 sm:gap-y-2">
          {matchingHosts.map((host) => (
            <label key={host} className="flex min-w-0 cursor-pointer items-center gap-2">
              <input
                type="checkbox"
                className="size-[15px] shrink-0 accent-accent"
                checked={selectedAliases.includes(host)}
                disabled={busy}
                onChange={() => onToggle(host)}
              />
              <span className="min-w-0 truncate font-mono text-[12.5px] text-ink">{host}</span>
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
