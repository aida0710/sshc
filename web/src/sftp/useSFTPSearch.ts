import { useEffect, useRef, useState } from "react";
import { sftpApi, type RemoteEntry } from "./api";
import type { SFTPBrowserModel } from "./useSFTPBrowser";

type SearchResult = { root: string; query: string; entries: RemoteEntry[]; truncated: boolean };

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
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);
  const searchInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (mobileSearchOpen) searchInput.current?.focus();
  }, [mobileSearchOpen]);

  const listedEntries = search === null ? entries : search.entries;
  const normalizedFilter = filter.trim().toLocaleLowerCase();
  // The filter box is the query in search mode; matching again locally would
  // hide results whose match is in a parent directory's name.
  const matches = (rows: RemoteEntry[]) => normalizedFilter === "" || search !== null
    ? rows
    : rows.filter((entry) => entry.name.toLocaleLowerCase().includes(normalizedFilter));

  async function runSearch(query = filter, root = search?.root ?? path) {
    const needle = query.trim();
    if (alias === "" || needle === "" || root === "") return;
    const found = await browser.track(root, () => sftpApi.search(alias, root, needle));
    if (found === null) return;
    setSearch({ root: found.path, query: found.query, entries: found.entries, truncated: found.truncated });
    onResults?.();
  }

  function endSearch() {
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
    setSearch(null);
    setFilter("");
    setMobileSearchOpen(false);
  }

  return {
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

