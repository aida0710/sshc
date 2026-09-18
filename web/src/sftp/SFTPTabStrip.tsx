import { useEffect, useRef, type DragEvent, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { activateTabFromKeyboard } from "../ui/tabKeyboard";
import { localHostAlias } from "./localHost";
import { maxTabsPerPane, type PaneSide, type SFTPPane, type SFTPTab } from "./sftpPanes";
import { sftpTabMimeType } from "./SFTPTabDropTarget";

export function tabElementId(tabId: string): string {
  return `sftp-tab-${tabId}`;
}

export function tabPanelElementId(tabId: string): string {
  return `sftp-tabpanel-${tabId}`;
}

function tabLabel(tab: SFTPTab, unnamed: string, localName: string): string {
  if (tab.alias === "") return unnamed;
  const name = tab.alias === localHostAlias ? localName : tab.alias;
  if (tab.path === "" || tab.path === "/") return name;
  const directory = tab.path.split("/").filter(Boolean).pop() ?? tab.path;
  return `${name}:${directory}`;
}

// SFTPTabStrip is the row of tabs above one pane. Tabs share one width so the
// strip does not reflow as hosts and directories change; when the pane is
// narrower than its tabs they shrink together and then scroll. A tab can be
// dragged onto a pane, or moved with Shift+Arrow, to split or join panes.
export function SFTPTabStrip({
  pane,
  label,
  closable,
  movable,
  trailing = null,
  onSelect,
  onClose,
  onAdd,
  onDragStart,
  onDragEnd,
  onMove,
}: {
  pane: SFTPPane;
  label: string;
  // Whether the tabs may be closed: false only for the last tab of the last pane.
  closable: boolean;
  // Tabs with unsaved edits stay where they are; a moved tab loses its editor.
  movable: (tab: SFTPTab) => boolean;
  trailing?: ReactNode;
  onSelect: (tabId: string) => void;
  onClose: (tab: SFTPTab) => void;
  onAdd: () => void;
  onDragStart: (tabId: string) => void;
  onDragEnd: () => void;
  onMove: (tabId: string, side: PaneSide) => void;
}) {
  const t = useTranslate();
  const scroller = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const selected = scroller.current?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]');
    selected?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [pane.activeId, pane.tabs.length]);

  function beginDrag(event: DragEvent<HTMLElement>, tab: SFTPTab) {
    event.dataTransfer.setData(sftpTabMimeType, tab.id);
    event.dataTransfer.effectAllowed = "move";
    onDragStart(tab.id);
  }

  function keyDown(event: ReactKeyboardEvent<HTMLButtonElement>, tab: SFTPTab, index: number) {
    if (event.shiftKey && (event.key === "ArrowLeft" || event.key === "ArrowRight")) {
      event.preventDefault();
      if (movable(tab)) onMove(tab.id, event.key === "ArrowLeft" ? "left" : "right");
      return;
    }
    activateTabFromKeyboard(event, index, pane.tabs, (target) => onSelect(target.id));
  }

  return (
    <div data-sftp-pane-tabs={pane.id} className="flex h-11 min-w-0 shrink-0 items-stretch gap-1 rounded-md border border-line/60 bg-toolbar p-1 md:h-9">
      <div
        ref={scroller}
        role="tablist"
        aria-label={label}
        className="flex min-w-0 flex-1 items-stretch gap-1 overflow-x-auto overscroll-x-contain"
      >
        {pane.tabs.map((tab, index) => {
          const name = tabLabel(tab, t("sftp.newTab"), t("sftp.local.connection"));
          const selected = tab.id === pane.activeId;
          const draggable = movable(tab);
          return (
            <span
              key={tab.id}
              draggable={draggable}
              onDragStart={draggable ? (event) => beginDrag(event, tab) : undefined}
              onDragEnd={draggable ? onDragEnd : undefined}
              className={`group relative flex w-44 min-w-24 shrink items-stretch rounded ${selected ? "bg-card shadow-sm" : "hover:bg-card/50"} ${draggable ? "cursor-grab active:cursor-grabbing" : ""}`}
            >
              <button
                type="button"
                role="tab"
                id={tabElementId(tab.id)}
                aria-selected={selected}
                aria-controls={tabPanelElementId(tab.id)}
                aria-keyshortcuts="Shift+ArrowLeft Shift+ArrowRight"
                tabIndex={selected ? 0 : -1}
                title={name}
                onClick={() => onSelect(tab.id)}
                onKeyDown={(event) => keyDown(event, tab, index)}
                className={`min-w-0 flex-1 truncate rounded px-3 text-left text-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent ${selected ? "font-medium text-ink" : "text-ink-muted"}`}
              >
                {name}
              </button>
              {closable ? (
                <button
                  type="button"
                  aria-label={t("sftp.closeTab", { name })}
                  onClick={() => onClose(tab)}
                  className="flex w-9 shrink-0 items-center justify-center rounded text-ink-faint hover:text-danger md:w-7"
                >
                  <Icon name="close" className="size-3" />
                </button>
              ) : null}
            </span>
          );
        })}
      </div>
      <button
        type="button"
        aria-label={t("sftp.newTab")}
        disabled={pane.tabs.length >= maxTabsPerPane}
        onClick={onAdd}
        className="flex w-9 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-card/50 hover:text-ink disabled:text-ink-faint md:w-7"
      >
        <Icon name="plus" className="size-4" />
      </button>
      {trailing}
    </div>
  );
}
