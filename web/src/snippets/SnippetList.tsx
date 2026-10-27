import { useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { PanelState } from "../ui/PanelState";
import { Button } from "../ui/surface";
import type { Snippet } from "./api";

export type SnippetLibraryState = "loading" | "ready" | "failed";

function matchesFilter(snippet: Snippet, filter: string): boolean {
  const needle = filter.trim().toLocaleLowerCase();
  if (needle === "") return true;
  return [snippet.name, snippet.description ?? "", snippet.command].some((text) => text.toLocaleLowerCase().includes(needle));
}

// SnippetList は、作業場の左の列である。絞り込みの欄と、スニペットごとの名前とコマンドの1行目を並べる。
// 一覧を読めなかったあいだは、空と見せずに読み込み中か読み直しのボタンを出す。
export function SnippetList({
  libraryState,
  snippets,
  openedId,
  empty,
  onRetry,
  onOpen,
}: {
  libraryState: SnippetLibraryState;
  snippets: Snippet[];
  openedId: string | null;
  // empty は、保存済みのスニペットが無いときに一覧の代わりに出すもの（スマホの空の状態）である。
  empty: ReactNode;
  onRetry: () => void;
  onOpen: (snippet: Snippet) => void;
}) {
  const t = useTranslate();
  const [filter, setFilter] = useState("");
  if (libraryState === "failed") {
    return (
      <PanelState
        tone="failed"
        title={t("snippets.loadFailed")}
        action={<Button onClick={onRetry}>{t("shell.bootstrapRetry")}</Button>}
      />
    );
  }
  if (libraryState === "loading") return <PanelState tone="loading" title={t("snippets.loading")} />;
  if (snippets.length === 0) return <>{empty}</>;
  const shown = snippets.filter((snippet) => matchesFilter(snippet, filter));
  return (
    <>
      <label className="relative mx-3 mb-2 mt-3.5 block shrink-0" htmlFor="snippet-filter">
        <span className="sr-only">{t("snippets.filter")}</span>
        <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-ink-faint" />
        <input
          id="snippet-filter"
          type="search"
          value={filter}
          onChange={(event) => setFilter(event.currentTarget.value)}
          placeholder={t("snippets.filter")}
          className={`${control} min-h-10 pl-8 md:min-h-0`}
        />
      </label>
      {shown.length === 0 ? (
        <p role="status" className="px-5 py-3 text-sm text-ink-muted">{t("snippets.noMatch")}</p>
      ) : (
        <ul className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-3 pb-[calc(0.75rem+var(--mobile-navigation-inset))] scroll-pb-(--mobile-navigation-inset)">
          {shown.map((snippet) => {
            const opened = snippet.id === openedId;
            return (
              <li key={snippet.id}>
                <button
                  type="button"
                  aria-current={opened ? "true" : undefined}
                  onClick={() => onOpen(snippet)}
                  className={`flex w-full min-w-0 flex-col gap-0.5 rounded-lg px-2.5 py-2 text-left transition-colors duration-150 ${opened ? "bg-select-fill" : "hover:bg-hover"}`}
                >
                  <span className="truncate text-[13.5px] text-ink">{snippet.name}</span>
                  <code className="truncate font-mono text-[11.5px] text-ink-faint">{snippet.command.split("\n")[0]}</code>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </>
  );
}
