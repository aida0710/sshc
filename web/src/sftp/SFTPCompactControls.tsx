import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import type { SFTPSearchModel } from "./useSFTPSearch";

export function SFTPCompactSelection({ label, menuLabel, expanded, onClear, onMenu, onSearch }: {
  label: string;
  menuLabel: string;
  expanded: boolean;
  onClear: () => void;
  onSearch: () => void;
  onMenu: (button: HTMLButtonElement) => void;
}) {
  const t = useTranslate();
  return <>
    <button type="button" aria-label={t("sftp.clearSelection")} onClick={onClear} className="flex size-12 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill"><Icon name="close" className="size-5" /></button>
    <span className="min-w-0 flex-1 truncate text-sm font-medium">{label}</span>
    <button type="button" aria-label={t("sftp.mobile.search")} aria-expanded="false" onClick={onSearch} className="flex size-12 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill"><Icon name="search" className="size-5" /></button>
    <button type="button" aria-label={menuLabel} aria-haspopup="dialog" aria-expanded={expanded} onClick={(event) => onMenu(event.currentTarget)} className="flex size-12 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill"><Icon name="moreHorizontal" className="size-5" /></button>
  </>;
}

export function SFTPCompactSearch({ search, recursive, disabled, onOptions }: {
  search: SFTPSearchModel;
  recursive: boolean;
  disabled: boolean;
  onOptions: () => void;
}) {
  const t = useTranslate();
  return <>
    <input ref={search.searchInput} type="search"
      aria-label={t(search.mode === "content" ? "sftp.search.contentPlaceholder" : "sftp.filter")}
      placeholder={t(search.mode === "content" ? "sftp.search.contentPlaceholder" : "sftp.filterPlaceholder")}
      value={search.filter} onChange={(event) => search.setFilter(event.target.value)}
      onKeyDown={(event) => { if (event.key !== "Enter") return; event.preventDefault(); if (recursive) void search.runSearch(); event.currentTarget.blur(); }}
      className="h-12 min-w-0 flex-1 rounded border border-control-line bg-control px-2 text-base outline-none focus:border-accent" />
    {recursive ? <>
      <button type="button" aria-label={t("sftp.search.mode")} onClick={onOptions} className="flex size-12 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill"><Icon name="settings" className="size-5" /></button>
      <button type="button" aria-label={search.searching ? t("sftp.search.stop") : t("sftp.searchBelow")} disabled={!search.searching && (disabled || search.filter.trim() === "")}
        onClick={() => { search.searchInput.current?.blur(); if (search.searching) search.cancelSearch(); else void search.runSearch(); }}
        className="flex size-12 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill disabled:text-ink-faint"><Icon name={search.searching ? "close" : "search"} className="size-5" /></button>
    </> : null}
    <button type="button" aria-label={t("sftp.close")} onClick={() => { search.setMobileSearchOpen(false); search.setFilter(""); }} className="flex size-12 shrink-0 items-center justify-center rounded text-ink-muted active:bg-select-fill"><Icon name="close" className="size-5" /></button>
  </>;
}
