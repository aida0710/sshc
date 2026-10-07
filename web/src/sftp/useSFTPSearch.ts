import { useEffect, useRef, useState } from "react";
import { sftpApi, type RemoteEntry, type RemoteSearchResult } from "./api";
import type { SFTPBrowserModel } from "./useSFTPBrowser";
import type { SearchMode } from "./contentToolTypes";

type SearchResult = RemoteSearchResult & { root: string; mode: SearchMode };

// The filter box and the recursive search behind it. While a search is
// showing, the rows are its results rather than one directory; everything
// that reads rows must read `listedEntries`, not the browser's entries.
export function useSFTPSearch({
  browser,
  onResults,
}: {
  browser: SFTPBrowserModel;
  // Selection and focus belong to the rows that were just replaced.
  onResults?: () => void;
}) {
  const { alias, path, entries } = browser;
  const [search, setSearch] = useState<SearchResult | null>(null);
  const [filter, setFilter] = useState("");
  const [remoteMode, setMode] = useState<SearchMode>("name");
  // Sources without recursive search retain their existing name filter.
  const mode: SearchMode = browser.source?.can.search === true ? remoteMode : "name";
  const [searching, setSearching] = useState(false);
  const request = useRef<AbortController | null>(null);
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);
  const searchInput = useRef<HTMLInputElement>(null);

  useEffect(() => () => { request.current?.abort(); }, [alias, path]);

  useEffect(() => {
    if (mobileSearchOpen) searchInput.current?.focus();
  }, [mobileSearchOpen]);

  const listedEntries = search === null ? entries : search.entries;
  const normalizedFilter = filter.trim().toLocaleLowerCase();
  // The filter box is the query in search mode; matching again locally would
  // hide results whose match is in a parent directory's name.
  const matches = (rows: RemoteEntry[]) => normalizedFilter === "" || search !== null || mode === "content"
    ? rows
    : rows.filter((entry) => entry.name.toLocaleLowerCase().includes(normalizedFilter));

  async function runSearch(query = filter, root = search?.root ?? path) {
    const needle = mode === "name" ? query.trim() : query;
    if (!browser.source?.can.search || alias === "" || needle.trim() === "" || root === "") return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setSearching(true);
    const found = await browser.track(root, async () => {
      try {
        return await sftpApi.search({ alias, path: root, query: needle, mode, signal: controller.signal });
      } catch (error) {
        if (controller.signal.aborted) return null;
        throw error;
      }
    });
    if (request.current !== controller) return;
    setSearching(false);
    request.current = null;
    if (found === null || controller.signal.aborted) return;
    setSearch({ ...found, root: found.path, mode });
    onResults?.();
  }

  function endSearch() {
    request.current?.abort();
    if (search === null) return;
    const root = search.root;
    setSearch(null);
    setFilter("");
    void browser.load(root);
  }

  // A rename or a delete made from the results has to be reflected there;
  // reloading the directory the user is not looking at would be no answer.
  function refreshAfterChange(directory: string, targetAlias: string): Promise<unknown> {
    if (search !== null) return runSearch(search.query, search.root);
    return browser.load(directory, { alias: targetAlias });
  }

  function refreshCurrent() {
    void (search !== null ? runSearch(search.query, search.root) : browser.refresh());
  }

  // Whether a completed deletion at `target` touches what is on screen.
  function covers(target: string, parentOf: (candidate: string) => string): boolean {
    if (search === null) return parentOf(target) === path;
    return search.root === "/" || target === search.root || target.startsWith(`${search.root}/`);
  }

  function clear() {
    request.current?.abort();
    setSearch(null);
    setFilter("");
    setMobileSearchOpen(false);
  }

  return {
    mode,
    setMode: (next: SearchMode) => { request.current?.abort(); setMode(next); setSearch(null); },
    searching,
    cancelSearch: () => request.current?.abort(),
    search,
    setSearch,
    filter,
    setFilter,
    normalizedFilter,
    listedEntries,
    matches,
    mobileSearchOpen,
    setMobileSearchOpen,
    searchInput,
    runSearch,
    endSearch,
    refreshAfterChange,
    refreshCurrent,
    covers,
    clear,
  };
}

export type SFTPSearchModel = ReturnType<typeof useSFTPSearch>;
