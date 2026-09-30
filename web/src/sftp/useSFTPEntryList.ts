import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import type { RemoteEntry } from "./api";

// The parent row keeps its own key so that arrow navigation can land on it
// without pretending that ".." is a listed entry.
export const parentRowKey = "..";

const longPressDelay = 500;
const longPressSlack = 12;

// One list, whether the rows come from an SSH host or from the engine's own
// filesystem. Selection, focus and the keyboard model live here so that the
// two sides cannot drift apart; what an activation or a context menu does is
// the caller's business.
export function useSFTPEntryList({
  entries,
  loadedEntries,
  parentRowVisible,
  busy,
  locked = false,
  mobileInteraction,
  onActivate,
  onOpenParent,
  onInteract,
  onContextMenu,
  onRenameKey,
  onDeleteKey,
  onEscape,
}: {
  // The rows in display order, after sorting and filtering.
  entries: RemoteEntry[];
  // The listing as it was loaded. A focus requested before a reload is
  // applied when this changes, not when a filter narrows the rows.
  loadedEntries: RemoteEntry[];
  parentRowVisible: boolean;
  busy: boolean;
  // True while something (an unsaved editor) must keep the user on this
  // directory: rows can still be selected but not opened.
  locked?: boolean;
  mobileInteraction: boolean;
  onActivate: (entry: RemoteEntry) => void;
  onOpenParent: () => void;
  // Called whenever the selection changes by hand, so that an open menu can
  // close.
  onInteract?: () => void;
  onContextMenu?: (entry: RemoteEntry, x: number, y: number) => void;
  onRenameKey?: () => void;
  onDeleteKey?: () => void;
  onEscape?: (() => void) | undefined;
}) {
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(() => new Set());
  const [focusedKey, setFocusedKey] = useState<string | null>(null);
  const selectAll = useRef<HTMLInputElement>(null);
  const selectionAnchor = useRef<string | null>(null);
  const rowNodes = useRef(new Map<string, HTMLElement>());
  // Dialogs opened from a menu outlive their trigger, so they are handed the
  // row itself as the element that takes focus back.
  const activeRow = useRef<HTMLElement | null>(null);
  // Written only through focusAfterReload and cancelPendingFocus, so that the
  // request's lifetime stays with this hook.
  const pendingFocus = useRef<string | null>(null);
  const longPress = useRef<{ timer: ReturnType<typeof globalThis.setTimeout>; x: number; y: number } | null>(null);
  const suppressNextClick = useRef(false);

  const selectedEntries = loadedEntries.filter((entry) => selectedPaths.has(entry.path));
  const selectedEntry = selectedEntries.length === 1 ? selectedEntries[0] ?? null : null;
  const allDisplayedSelected = entries.length > 0 && entries.every((entry) => selectedPaths.has(entry.path));
  const rowKeys = [...(parentRowVisible ? [parentRowKey] : []), ...entries.map((entry) => entry.path)];
  // Exactly one row owns the tab stop. A filter or a reload can drop the
  // remembered row, so fall back to the first one instead of stranding the
  // keyboard outside the list.
  const activeRowKey = focusedKey !== null && rowKeys.includes(focusedKey) ? focusedKey : rowKeys[0] ?? null;

  useEffect(() => () => {
    if (longPress.current !== null) globalThis.clearTimeout(longPress.current.timer);
  }, []);

  useEffect(() => {
    if (selectAll.current !== null) {
      selectAll.current.indeterminate = selectedEntries.length > 0 && !allDisplayedSelected;
    }
  }, [allDisplayedSelected, selectedEntries.length]);

  useEffect(() => {
    activeRow.current = activeRowKey === null ? null : rowNodes.current.get(activeRowKey) ?? null;
  });

  useEffect(() => {
    const key = pendingFocus.current;
    if (key === null) return;
    pendingFocus.current = null;
    const node = rowNodes.current.get(key);
    if (node === undefined) return;
    setFocusedKey(key);
    node.focus();
  }, [loadedEntries]);

  function activate(entry: RemoteEntry) {
    if (busy || locked) return;
    onInteract?.();
    onActivate(entry);
  }

  // Asks for `key` to take focus when the next listing arrives: a created or
  // renamed entry, the row that takes a deleted one's place, or the parent row
  // after going up. A later request replaces an earlier one.
  function focusAfterReload(key: string) {
    pendingFocus.current = key;
  }

  function cancelPendingFocus() {
    pendingFocus.current = null;
  }

  function openParent() {
    focusAfterReload(parentRowKey);
    onOpenParent();
  }

  function selectEntry(entry: RemoteEntry, modifiers: { shift?: boolean; additive?: boolean } = {}) {
    const anchorIndex = selectionAnchor.current === null
      ? -1
      : entries.findIndex((candidate) => candidate.path === selectionAnchor.current);
    const entryIndex = entries.findIndex((candidate) => candidate.path === entry.path);
    if (modifiers.shift === true && anchorIndex >= 0 && entryIndex >= 0) {
      const start = Math.min(anchorIndex, entryIndex);
      const end = Math.max(anchorIndex, entryIndex);
      setSelectedPaths((current) => {
        const next = modifiers.additive === true ? new Set(current) : new Set<string>();
        for (const candidate of entries.slice(start, end + 1)) next.add(candidate.path);
        return next;
      });
    } else if (modifiers.additive === true) {
      toggleSelection(entry);
      selectionAnchor.current = entry.path;
      return;
    } else {
      setSelectedPaths(new Set([entry.path]));
      selectionAnchor.current = entry.path;
    }
    onInteract?.();
  }

  function clickEntry(entry: RemoteEntry, event: ReactMouseEvent<HTMLButtonElement>) {
    // A long press already opened the context menu for this row. The synthetic
    // click that follows the touch must not close it again.
    if (suppressNextClick.current) {
      suppressNextClick.current = false;
      return;
    }
    if (busy) return;
    setFocusedKey(entry.path);
    if (mobileInteraction && !event.shiftKey && !event.metaKey && !event.ctrlKey) {
      if (selectedPaths.size > 0) toggleSelection(entry);
      else activate(entry);
      return;
    }
    selectEntry(entry, { shift: event.shiftKey, additive: event.metaKey || event.ctrlKey });
  }

  function focusRow(key: string) {
    setFocusedKey(key);
    const node = rowNodes.current.get(key);
    node?.focus();
    node?.scrollIntoView?.({ block: "nearest" });
  }

  function registerRow(key: string, node: HTMLElement | null): void {
    if (node === null) rowNodes.current.delete(key);
    else rowNodes.current.set(key, node);
  }

  // The row that owns the keystroke is read from the DOM rather than from
  // state, so that tabbing or clicking into a row is honoured even before the
  // focus event has been reduced into React state.
  function currentRowKey(target: EventTarget | null): string | null {
    const element = target instanceof Element ? target.closest("[data-row-key]") : null;
    return element?.getAttribute("data-row-key") ?? activeRowKey;
  }

  function moveRowFocus(event: ReactKeyboardEvent<HTMLDivElement>, from: string | null) {
    if (rowKeys.length === 0) return;
    const current = from === null ? -1 : rowKeys.indexOf(from);
    const last = rowKeys.length - 1;
    const next = event.key === "Home"
      ? 0
      : event.key === "End"
        ? last
        : event.key === "ArrowDown"
          ? current < 0 ? 0 : Math.min(last, current + 1)
          : current < 0 ? last : Math.max(0, current - 1);
    const destination = rowKeys[next];
    if (destination === undefined) return;
    focusRow(destination);
    const entry = entries.find((candidate) => candidate.path === destination);
    // Ctrl moves the cursor without disturbing a multi-row selection, and the
    // parent row is a destination rather than something selectable.
    if (entry === undefined || event.ctrlKey || event.metaKey) return;
    selectEntry(entry, { shift: event.shiftKey, additive: false });
  }

  function entryForKey(key: string | null): RemoteEntry | null {
    return entries.find((candidate) => candidate.path === key) ?? null;
  }

  function handleListKeys(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (busy) return;
    if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === "a") {
      event.preventDefault();
      selectAllDisplayed();
      return;
    }
    // A checkbox owns Space, and the browser owns typing inside inputs.
    const withinInput = event.target instanceof HTMLInputElement;
    const rowKey = currentRowKey(event.target);
    switch (event.key) {
      case "ArrowDown":
      case "ArrowUp":
      case "Home":
      case "End":
        event.preventDefault();
        moveRowFocus(event, rowKey);
        return;
      case " ": {
        if (withinInput) return;
        const entry = entryForKey(rowKey);
        if (entry === null) return;
        event.preventDefault();
        toggleSelection(entry);
        return;
      }
      case "Enter": {
        if (withinInput || busy || locked) return;
        if (rowKey === parentRowKey) {
          event.preventDefault();
          openParent();
          return;
        }
        const entry = entryForKey(rowKey);
        if (entry === null) return;
        event.preventDefault();
        activate(entry);
        return;
      }
      case "F2":
        if (onRenameKey === undefined) return;
        event.preventDefault();
        onRenameKey();
        return;
      case "Delete":
        if (onDeleteKey === undefined) return;
        event.preventDefault();
        onDeleteKey();
        return;
      case "Escape":
        if (selectedPaths.size > 0) {
          event.preventDefault();
          setSelectedPaths(new Set());
          selectionAnchor.current = null;
          return;
        }
        if (onEscape === undefined) return;
        event.preventDefault();
        onEscape();
        return;
      default:
    }
  }

  // A right click on a row outside the selection acts on that row alone, the
  // way every file manager does; inside it, the whole selection is kept.
  function openContextMenu(entry: RemoteEntry, x: number, y: number) {
    if (onContextMenu === undefined || busy || locked) return;
    if (!selectedPaths.has(entry.path)) {
      setSelectedPaths(new Set([entry.path]));
      selectionAnchor.current = entry.path;
    }
    focusRow(entry.path);
    onContextMenu(entry, x, y);
  }

  function rowContextMenu(event: ReactMouseEvent<HTMLElement>, entry: RemoteEntry) {
    if (onContextMenu === undefined) return;
    event.preventDefault();
    openContextMenu(entry, event.clientX, event.clientY);
  }

  function cancelLongPress() {
    if (longPress.current === null) return;
    globalThis.clearTimeout(longPress.current.timer);
    longPress.current = null;
  }

  function beginLongPress(event: ReactPointerEvent<HTMLElement>, entry: RemoteEntry) {
    if (onContextMenu === undefined || event.pointerType === "mouse" || busy || locked) return;
    cancelLongPress();
    // A long press that opened a menu but was never followed by a click must
    // not swallow the first tap on some other row.
    suppressNextClick.current = false;
    const { clientX, clientY } = event;
    const timer = globalThis.setTimeout(() => {
      longPress.current = null;
      suppressNextClick.current = true;
      openContextMenu(entry, clientX, clientY);
    }, longPressDelay);
    longPress.current = { timer, x: clientX, y: clientY };
  }

  function trackLongPress(event: ReactPointerEvent<HTMLElement>) {
    const pending = longPress.current;
    if (pending === null) return;
    if (Math.abs(event.clientX - pending.x) > longPressSlack || Math.abs(event.clientY - pending.y) > longPressSlack) {
      cancelLongPress();
    }
  }

  function toggleSelection(entry: RemoteEntry) {
    if (busy) return;
    setSelectedPaths((current) => {
      const next = new Set(current);
      if (next.has(entry.path)) next.delete(entry.path);
      else next.add(entry.path);
      return next;
    });
    selectionAnchor.current = entry.path;
    onInteract?.();
  }

  function toggleAllDisplayed() {
    setSelectedPaths((current) => {
      const next = new Set(current);
      for (const entry of entries) {
        if (allDisplayedSelected) next.delete(entry.path);
        else next.add(entry.path);
      }
      return next;
    });
    onInteract?.();
  }

  function selectAllDisplayed() {
    setSelectedPaths((current) => {
      const next = new Set(current);
      for (const entry of entries) next.add(entry.path);
      return next;
    });
    selectionAnchor.current = entries[0]?.path ?? null;
    onInteract?.();
  }

  function invertDisplayedSelection() {
    setSelectedPaths((current) => {
      const next = new Set(current);
      for (const entry of entries) {
        if (next.has(entry.path)) next.delete(entry.path);
        else next.add(entry.path);
      }
      return next;
    });
    onInteract?.();
  }

  function clearSelection() {
    setSelectedPaths(new Set());
    selectionAnchor.current = null;
  }

  return {
    selectedPaths,
    setSelectedPaths,
    selectedEntries,
    selectedEntry,
    allDisplayedSelected,
    rowKeys,
    activeRowKey,
    setFocusedKey,
    focusRow,
    registerRow,
    focusAfterReload,
    cancelPendingFocus,
    activeRow,
    selectAll,
    activate,
    openParent,
    selectEntry,
    clickEntry,
    toggleSelection,
    toggleAllDisplayed,
    selectAllDisplayed,
    invertDisplayedSelection,
    clearSelection,
    handleListKeys,
    rowContextMenu,
    beginLongPress,
    trackLongPress,
    cancelLongPress,
  };
}

export type SFTPEntryListModel = ReturnType<typeof useSFTPEntryList>;
