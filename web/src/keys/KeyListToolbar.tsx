import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { belowSmallWidthQuery, useMediaQuery } from "../ui/useMediaQuery";
import type { ListFilter } from "./organizer";

// KeyListToolbar は、鍵の一覧の節の見出しの右に置く、表示する種類の切り替えと検索欄である。
// 狭い画面では、フォルダの選択と合わせて3つの欄が並ぶので、検索は虫眼鏡のボタンに畳み、押したら欄を開く。
export function KeyListToolbar({
  listFilter,
  onListFilterChange,
  query,
  onQueryChange,
}: {
  listFilter: ListFilter;
  onListFilterChange: (filter: ListFilter) => void;
  query: string;
  onQueryChange: (query: string) => void;
}) {
  const t = useTranslate();
  const narrow = useMediaQuery(belowSmallWidthQuery);
  const [searchOpened, setSearchOpened] = useState(false);
  const searchShown = !narrow || searchOpened || query !== "";
  return (
    <div className="flex w-full gap-2 sm:w-auto">
        <label className="w-28 shrink-0 sm:w-auto">
          <span className="sr-only">{t("keys.listFilter")}</span>
          <select
            className={control}
            value={listFilter}
            onChange={(event) => onListFilterChange(event.target.value as ListFilter)}
          >
            <option value="keys">{t("keys.listFilterKeys")}</option>
            <option value="all">{t("keys.listFilterAll")}</option>
          </select>
        </label>
        {searchShown ? <label className="relative min-w-0 grow sm:w-64 sm:grow-0">
          <span className="sr-only">{t("keys.search")}</span>
          <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint" />
          <input
            type="search"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder={t("keys.searchPlaceholder")}
            autoFocus={narrow && searchOpened}
            className={`${control} pl-8`}
          />
        </label> : (
          <button
            type="button"
            aria-label={t("keys.search")}
            onClick={() => setSearchOpened(true)}
            className="ml-auto flex size-11 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink"
          >
            <Icon name="search" className="size-4" />
          </button>
        )}
    </div>
  );
}
