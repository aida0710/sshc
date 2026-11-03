import type { ComponentPropsWithoutRef, ComponentPropsWithRef } from "react";
import { useTranslate } from "../i18n/context";
import type { RemoteEntry } from "./api";
import type { SFTPEntryListModel } from "./useSFTPEntryList";

// Both lists wire their rows through these, so that focus, selection and the long press work the same whichever
// list is on screen. Only the markup around them differs.
export function useEntryRowProps({ model, busy }: { model: SFTPEntryListModel; busy: boolean }) {
  const t = useTranslate();
  const {
    selectedPaths, activeRowKey, registerRow, setFocusedKey, clickEntry, toggleSelection,
    beginLongPress, trackLongPress, cancelLongPress,
  } = model;
  const entryCheckboxProps = (entry: RemoteEntry): ComponentPropsWithoutRef<"input"> => ({
    "aria-label": t("sftp.selectEntry", { name: entry.name }),
    checked: selectedPaths.has(entry.path),
    tabIndex: activeRowKey === entry.path ? 0 : -1,
    disabled: busy,
    onChange: () => toggleSelection(entry),
    className: "size-4 accent-accent",
  });
  // A locked list still lets a row be selected; the model refuses to open it.
  const entryButtonProps = (entry: RemoteEntry): ComponentPropsWithRef<"button"> => ({
    ref: (node) => { registerRow(entry.path, node); },
    "aria-label": entry.name,
    "aria-pressed": selectedPaths.has(entry.path),
    tabIndex: activeRowKey === entry.path ? 0 : -1,
    disabled: busy,
    onFocus: () => setFocusedKey(entry.path),
    onClick: (event) => clickEntry(entry, event),
    onPointerDown: (event) => beginLongPress(event, entry),
    onPointerMove: trackLongPress,
    onPointerUp: cancelLongPress,
    onPointerCancel: cancelLongPress,
  });
  return { entryCheckboxProps, entryButtonProps };
}
