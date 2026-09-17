import { useTranslate } from "../i18n/context";
import type { TerminalSearch } from "./useTerminalSearch";

// The find-in-scrollback bar floating over the terminal.
export function TerminalSearchBar({ search, mobile }: { search: TerminalSearch; mobile: boolean }) {
  const t = useTranslate();
  return (
    <div className={`absolute inset-x-2 top-2 z-20 items-center gap-1.5 rounded-lg border border-line bg-toolbar/95 p-1.5 shadow-lg backdrop-blur ${mobile ? "grid grid-cols-6" : "left-auto flex w-[34rem]"}`}>
      <input
        ref={search.input}
        autoFocus
        aria-label={t("terminal.searchInput")}
        value={search.query}
        onChange={(event) => search.setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape") search.close();
          else if (event.key === "Enter") search.step(event.shiftKey ? -1 : 1);
        }}
        className={`min-w-0 flex-1 rounded border border-control-line bg-control px-2 py-1 ${mobile ? "col-span-6 min-h-11 text-base" : "text-xs"}`}
        placeholder={t("terminal.searchPlaceholder")}
      />
      <button
        type="button"
        aria-label={t("terminal.searchCaseSensitive")}
        aria-pressed={search.caseSensitive}
        className={`rounded border px-1.5 py-1 text-xs ${search.caseSensitive ? "border-accent bg-accent/10 text-accent" : "border-control-line text-ink-muted"}`}
        onClick={() => search.setCaseSensitive((current) => !current)}
      >
        Aa
      </button>
      <button
        type="button"
        aria-label={t("terminal.searchRegex")}
        aria-pressed={search.regex}
        className={`rounded border px-1.5 py-1 font-mono text-xs ${search.regex ? "border-accent bg-accent/10 text-accent" : "border-control-line text-ink-muted"}`}
        onClick={() => search.setRegex((current) => !current)}
      >
        .*
      </button>
      <span role="status" className={`${mobile ? "min-w-0" : "w-14"} text-center text-[11px] ${search.invalid ? "text-danger" : "text-ink-muted"}`}>
        {search.invalid
          ? t("terminal.searchInvalidRegex")
          : search.result.total === 0
            ? t("terminal.searchNoResults")
            : `${search.result.index + 1}/${search.result.total}`}
      </span>
      <button type="button" aria-label={t("terminal.searchPrevious")} className="rounded border border-control-line px-2 py-0.5 text-sm" onClick={() => search.step(-1)}>↑</button>
      <button type="button" aria-label={t("terminal.searchNext")} className="rounded border border-control-line px-2 py-0.5 text-sm" onClick={() => search.step(1)}>↓</button>
      <button type="button" aria-label={t("terminal.searchClose")} className="rounded px-2 py-0.5 text-sm" onClick={search.close}>×</button>
    </div>
  );
}
