import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import type { ListFilter } from "./organizer";

// KeyListToolbar は、鍵の一覧の上に置く、表示する種類の切り替えと検索欄である。
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
    <div className="flex flex-wrap items-end justify-between gap-3">
      <p className="font-mono text-xs font-medium text-ink-muted">~/.ssh</p>
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
        <label className="min-w-0 grow sm:w-72 sm:grow-0">
          <span className="sr-only">{t("keys.search")}</span>
          <input
            type="search"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder={t("keys.searchPlaceholder")}
            className={control}
          />
        </label>
      </div>
    </div>
  );
}
