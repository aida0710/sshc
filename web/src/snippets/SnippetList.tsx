import { useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { phoneCardFrame } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { Button } from "../ui/surface";
import type { Snippet } from "./api";
import { snippetSummary } from "./snippetKind";

export type SnippetLibraryState = "loading" | "ready" | "failed";

function matchesFilter(snippet: Snippet, filter: string): boolean {
  const needle = filter.trim().toLocaleLowerCase();
  if (needle === "") return true;
  const stepTexts = (snippet.steps ?? []).flatMap((step) => [step.wait ?? "", step.send]);
  return [snippet.name, snippet.description ?? "", snippet.command, ...stepTexts].some((text) => text.toLocaleLowerCase().includes(needle));
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
  columns = false,
  phoneCard = false,
}: {
  libraryState: SnippetLibraryState;
  snippets: Snippet[];
  openedId: string | null;
  // empty は、保存済みのスニペットが無いときに一覧の代わりに出すもの（スマホの空の状態）である。
  empty: ReactNode;
  onRetry: () => void;
  onOpen: (snippet: Snippet) => void;
  // columns は、一覧がページの幅を全部使うとき（何も開いていないとき）に、1行を名前・コマンド・説明の列にする。
  columns?: boolean;
  // phoneCard は、スマホの組み方のときに、一覧を Account passwords と同じ板（phoneCardFrame）に入れ、行を線で区切る。
  phoneCard?: boolean;
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
  // 一覧はこの入れ物の中でスクロールする。スマホでは下のバーが一覧の端に重なるので、端にその分の余白を空ける。
  const scrollArea = "min-h-0 flex-1 overflow-y-auto px-3 pb-[calc(0.75rem+var(--mobile-navigation-inset))] scroll-pb-(--mobile-navigation-inset)";
  const rows = shown.map((snippet) => {
    const opened = snippet.id === openedId;
    return (
      <li key={snippet.id} className={phoneCard ? "border-t border-line first:border-t-0" : undefined}>
        <button
          type="button"
          aria-current={opened ? "true" : undefined}
          onClick={() => onOpen(snippet)}
          className={`sshc-focus-inset w-full min-w-0 rounded-lg px-2.5 py-2 text-left transition-colors duration-150 ${columns ? "grid grid-cols-[minmax(0,16rem)_minmax(0,28rem)_minmax(0,1fr)] items-center gap-4" : "flex flex-col gap-0.5"} ${opened ? "bg-select-fill" : "hover:bg-hover"}`}
        >
          <span title={snippet.name} className="truncate text-sm text-ink">{snippet.name}</span>
          <code title={snippet.command} className="truncate font-mono text-xs text-ink-faint">{snippetSummary(t, snippet)}</code>
          {columns ? <span title={snippet.description} className="truncate text-xs text-ink-muted">{snippet.description}</span> : null}
        </button>
      </li>
    );
  });
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
      ) : phoneCard ? (
        <div className={scrollArea}>
          <ul className={`${phoneCardFrame} pl-3.5 pr-1.5`}>{rows}</ul>
        </div>
      ) : (
        <ul className={`flex flex-col gap-0.5 ${scrollArea}`}>{rows}</ul>
      )}
    </>
  );
}
