import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import type { SFTPSearchModel } from "./useSFTPSearch";
import { isImeComposing } from "../ui/imeComposition";
import { literalTextEntry } from "../ui/softKeyboard";
import { IconButton } from "../ui/IconButton";

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
    <IconButton label={t("sftp.clearSelection")} size="barLarge" ground="none" className="active:bg-select-fill" onClick={onClear}><Icon name="close" className="size-5" /></IconButton>
    <span title={label} className="min-w-0 flex-1 truncate text-sm font-medium">{label}</span>
    <IconButton label={t("sftp.mobile.search")} size="barLarge" ground="none" className="active:bg-select-fill" aria-expanded="false" onClick={onSearch}><Icon name="search" className="size-5" /></IconButton>
    <IconButton label={menuLabel} size="barLarge" ground="none" className="active:bg-select-fill" aria-haspopup="dialog" aria-expanded={expanded} onClick={(event) => onMenu(event.currentTarget)}><Icon name="moreHorizontal" className="size-5" /></IconButton>
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
    <input {...literalTextEntry} ref={search.searchInput} type="search"
      aria-label={t(search.mode === "content" ? "sftp.search.contentPlaceholder" : "sftp.filter")}
      placeholder={t(search.mode === "content" ? "sftp.search.contentPlaceholder" : "sftp.filterPlaceholder")}
      value={search.filter} onChange={(event) => search.setFilter(event.target.value)}
      onKeyDown={(event) => { if (isImeComposing(event) || event.key !== "Enter") return; event.preventDefault(); if (recursive) void search.runSearch(); event.currentTarget.blur(); }}
      className="ml-2 h-12 min-w-0 flex-1 rounded-md border border-control-line bg-control px-2 text-base" />
    {recursive ? <>
      <IconButton label={t("sftp.search.mode")} size="barLarge" ground="none" className="active:bg-select-fill" onClick={onOptions}><Icon name="settings" className="size-5" /></IconButton>
      <IconButton label={search.searching ? t("sftp.search.stop") : t("sftp.searchBelow")} size="barLarge" ground="none" className="active:bg-select-fill" disabled={!search.searching && (disabled || search.filter.trim() === "")}
        onClick={() => { search.searchInput.current?.blur(); if (search.searching) search.cancelSearch(); else void search.runSearch(); }}><Icon name={search.searching ? "close" : "search"} className="size-5" /></IconButton>
    </> : null}
    <IconButton label={t("sftp.close")} size="barLarge" ground="none" className="active:bg-select-fill" onClick={() => { search.setMobileSearchOpen(false); search.setFilter(""); }}><Icon name="close" className="size-5" /></IconButton>
  </>;
}
