import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import type { ListFilter } from "./organizer";

// KeyListToolbar は、鍵の一覧の節の見出しの右に置く、表示する種類の切り替えと検索欄である。
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
  return (
    <div className="flex w-full flex-wrap gap-2 sm:w-auto">
        <label>
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
        <label className="relative min-w-0 grow sm:w-64 sm:grow-0">
          <span className="sr-only">{t("keys.search")}</span>
          <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint" />
          <input
            type="search"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder={t("keys.searchPlaceholder")}
            className={`${control} pl-8`}
          />
        </label>
    </div>
  );
}
