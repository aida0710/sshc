import { useEffect } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { sizedControl } from "../ui/form";
import { Button } from "../ui/surface";
import type { TerminalSearch } from "./useTerminalSearch";
import { isImeComposing } from "../ui/imeComposition";
import { literalTextEntry } from "../ui/softKeyboard";

const pressedOption = "aria-pressed:border-accent aria-pressed:bg-accent/10 aria-pressed:text-accent";
// An icon is a block (Tailwind's preflight), so it would sit at the left of a
// button that the phone's 44-pixel minimum or the grid makes wider than it.
const iconButton = "inline-flex items-center justify-center";

// A press on the bar's buttons leaves focus in the query, so the next step
// or another character can follow at once.
function keepQueryFocus(event: { preventDefault(): void }) {
  event.preventDefault();
}

// The find-in-scrollback bar floating over the terminal.
export function TerminalSearchBar({ search, mobile }: { search: TerminalSearch; mobile: boolean }) {
  const t = useTranslate();
  const { input } = search;
  // The query carried over from the last search is selected, so typing replaces it.
  useEffect(() => {
    input.current?.select();
  }, [input]);
  return (
    <div className={`absolute inset-x-2 top-2 z-20 items-center gap-1.5 rounded-lg border border-line bg-toolbar/95 p-1.5 shadow-float backdrop-blur ${mobile ? "grid grid-cols-6" : "left-auto flex w-[34rem] max-w-[calc(100%-1rem)] flex-wrap"}`}>
      <input {...literalTextEntry}
        ref={input}
        autoFocus
        aria-label={t("terminal.searchInput")}
        value={search.query}
        onChange={(event) => search.setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (isImeComposing(event)) return;
          if (event.key === "Escape") search.close();
          else if (event.key === "Enter") search.step(event.shiftKey ? -1 : 1);
        }}
        className={`${sizedControl("min-w-0 flex-1")} ${mobile ? "col-span-6" : ""}`}
        placeholder={t("terminal.searchPlaceholder")}
      />
      <Button
        small
        aria-label={t("terminal.searchCaseSensitive")}
        aria-pressed={search.caseSensitive}
        className={pressedOption}
        onMouseDown={keepQueryFocus}
        onClick={() => search.setCaseSensitive((current) => !current)}
      >
        Aa
      </Button>
      <Button
        small
        aria-label={t("terminal.searchRegex")}
        aria-pressed={search.regex}
        className={`font-mono ${pressedOption}`}
        onMouseDown={keepQueryFocus}
        onClick={() => search.setRegex((current) => !current)}
      >
        .*
      </Button>
      <span role="status" className={`${mobile ? "min-w-0" : "w-14"} text-center text-[11px] ${search.invalid ? "text-danger" : "text-ink-muted"}`}>
        {search.invalid
          ? t("terminal.searchInvalidRegex")
          : search.result.total === 0
            ? t("terminal.searchNoResults")
            : `${search.result.index + 1}/${search.result.total}`}
      </span>
      <Button small aria-label={t("terminal.searchPrevious")} className={iconButton} onMouseDown={keepQueryFocus} onClick={() => search.step(-1)}><Icon name="arrowUp" className="size-3.5" /></Button>
      <Button small aria-label={t("terminal.searchNext")} className={iconButton} onMouseDown={keepQueryFocus} onClick={() => search.step(1)}><Icon name="arrowDown" className="size-3.5" /></Button>
      <Button small aria-label={t("terminal.searchClose")} className={iconButton} onClick={search.close}><Icon name="close" className="size-3.5" /></Button>
    </div>
  );
}
