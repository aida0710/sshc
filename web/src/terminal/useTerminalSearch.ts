import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import type { SearchAddon } from "@xterm/addon-search";
import type { Terminal } from "@xterm/xterm";
import { matchesShortcut, shortcutsBlocked } from "../keyconfig/bindings";
import { validSearchPattern, type TerminalSearchSettings } from "./search";

export type TerminalSearchResult = { index: number; total: number };

const noResult: TerminalSearchResult = { index: -1, total: 0 };

// Search state for one terminal view. The xterm instance is recreated with
// the session, so the addon is bound from inside the terminal effect; the
// query and settings live in refs as well so the bound callbacks read the
// latest values without that effect re-running.
export function useTerminalSearch({ shortcutActive, region }: {
  // When set, the shortcut is owned regardless of focus; otherwise only
  // while focus is inside the region.
  shortcutActive: boolean | undefined;
  region: RefObject<HTMLElement | null>;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [regex, setRegex] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [result, setResult] = useState(noResult);
  const queryRef = useRef("");
  queryRef.current = query;
  const settingsRef = useRef<TerminalSearchSettings>({ caseSensitive: false, regex: false });
  settingsRef.current = { caseSensitive, regex };
  const stepRef = useRef<(direction: 1 | -1) => void>(() => {});
  const refreshRef = useRef<() => void>(() => {});
  const clearRef = useRef<() => void>(() => {});

  useEffect(() => {
    const openSearch = (event: KeyboardEvent) => {
      if (!(shortcutActive ?? region.current?.contains(document.activeElement))) return;
      if (!matchesShortcut(event, "terminalSearch") || shortcutsBlocked(event)) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      if (!event.repeat) {
        setOpen(true);
        input.current?.focus();
        input.current?.select();
      }
    };
    window.addEventListener("keydown", openSearch, true);
    return () => window.removeEventListener("keydown", openSearch, true);
  }, [shortcutActive, region]);

  useEffect(() => {
    if (open) refreshRef.current();
    else clearRef.current();
  }, [open, query, caseSensitive, regex]);

  // Wires the addon of a freshly opened terminal. The returned function
  // unwires it and must run before that terminal is disposed.
  const bind = useCallback((view: Terminal, search: SearchAddon, container: HTMLElement): () => void => {
    const subscription = search.onDidChangeResults((changed) => {
      setResult({ index: changed.resultIndex, total: changed.resultCount });
    });
    const options = (incremental: boolean) => {
      const style = getComputedStyle(container);
      const match = style.getPropertyValue("--ui-term-yellow").trim();
      const active = style.getPropertyValue("--ui-term-bright-yellow").trim();
      return {
        ...settingsRef.current,
        incremental,
        decorations: {
          matchBackground: match,
          matchBorder: active,
          matchOverviewRuler: active,
          activeMatchBackground: active,
          activeMatchBorder: match,
          activeMatchColorOverviewRuler: match,
        },
      };
    };
    const reset = () => {
      search.clearDecorations();
      view.clearSelection();
      setResult(noResult);
    };
    const run = (direction: 1 | -1, incremental: boolean) => {
      const query = queryRef.current;
      if (!validSearchPattern(query, settingsRef.current)) {
        reset();
        setInvalid(true);
        return;
      }
      setInvalid(false);
      if (query === "") {
        reset();
        return;
      }
      if (direction === 1) search.findNext(query, options(incremental));
      else search.findPrevious(query, options(false));
    };
    stepRef.current = (direction) => run(direction, false);
    refreshRef.current = () => run(1, true);
    clearRef.current = () => {
      reset();
      setInvalid(false);
    };
    return () => {
      subscription.dispose();
      stepRef.current = () => {};
      refreshRef.current = () => {};
      clearRef.current = () => {};
    };
  }, []);

  const step = useCallback((direction: 1 | -1) => stepRef.current(direction), []);
  const toggle = useCallback(() => setOpen((current) => !current), []);
  const close = useCallback(() => setOpen(false), []);
  // Whether the search bar, rather than the terminal, holds keyboard focus.
  const hasFocus = useCallback(
    () => input.current?.parentElement?.contains(document.activeElement) === true,
    [],
  );

  return {
    input,
    open,
    query,
    setQuery,
    caseSensitive,
    setCaseSensitive,
    regex,
    setRegex,
    invalid,
    result,
    step,
    toggle,
    close,
    bind,
    hasFocus,
  };
}

export type TerminalSearch = ReturnType<typeof useTerminalSearch>;
