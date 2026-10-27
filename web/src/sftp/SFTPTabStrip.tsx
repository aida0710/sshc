import { useEffect, useRef, useState, type DragEvent, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { Icon } from "../ui/icons";
import { activateTabFromKeyboard } from "../ui/tabKeyboard";
import { draggableCursor } from "../ui/dragAndDropLook";
import { localHostAlias } from "./localHost";
import { maxTabsPerPane, type PaneSide, type SFTPPane, type SFTPTab } from "./sftpPanes";
import { carriesTab, sftpTabMimeType } from "./SFTPTabDropTarget";

export function tabElementId(tabId: string): string {
  return `sftp-tab-${tabId}`;
}

export function tabPanelElementId(tabId: string): string {
  return `sftp-tabpanel-${tabId}`;
}

function tabLabels(tab: SFTPTab, unnamed: string, localName: string) {
  if (tab.alias === "") return { name: unnamed, folder: unnamed, host: "", title: "" };
  const host = tab.alias === localHostAlias ? localName : tab.alias;
  const folder = tab.path === "" ? host : tab.path.split("/").filter(Boolean).pop() ?? "/";
  return {
    name: tab.path === "" || tab.path === "/" ? host : `${host}:${folder}`,
    folder,
    host: tab.path === "" ? "" : host,
    title: tab.path === "" ? host : `${host}:${tab.path}`,
  };
}

// On a mouse screen a tab is one line, the folder and its host side by side.
// Touch keeps the heights a finger needs, and a compact screen stacks the
// host under the folder inside its 44px tab.
const tabLayout = {
  compact: "min-h-11 flex-col justify-center",
  regular: "min-h-[30px] items-center gap-1.5 mobile-layout:min-h-10",
};
const controlSize = {
  compact: "min-h-11 w-11",
  regular: "min-h-[30px] w-6 mobile-layout:min-h-10 mobile-layout:w-8",
};
// On a wide screen the selected tab takes the colour of the path bar under it, so the two read as one piece.
const stripLook = {
  compact: "gap-1 border-b border-line/70 bg-toolbar px-1 py-0.5",
  regular: "gap-1.5 px-3 pt-2.5",
};
const tabLook = {
  compact: { selected: "rounded bg-card shadow-sm", other: "rounded hover:bg-card/50" },
  regular: { selected: "rounded-t-md bg-well", other: "rounded-md hover:bg-hover" },
};

// Tabs share one width so changing hosts and directories does not reflow the
// strip. Compact screens combine all file tabs here and keep touch controls
// large; other screens allow movable tabs to split or join panes.
export function SFTPTabStrip({
  pane,
  label,
  closable,
  movable,
  compact = false,
  addDisabled = pane.tabs.length >= maxTabsPerPane,
  trailing = null,
  onSelect,
  onClose,
  onAdd,
  onChangeHost,
  canChangeHost = () => true,
  onDragStart,
  onDragEnd,
  onMove,
  onDropAt,
  onReorder,
  tabActions,
}: {
  pane: SFTPPane;
  label: string;
  // Whether the tabs may be closed: false only for the last tab of the last pane.
  closable: boolean;
  // Tabs with unsaved edits stay where they are; a moved tab loses its editor.
  movable: (tab: SFTPTab) => boolean;
  compact?: boolean;
  addDisabled?: boolean;
  trailing?: ReactNode;
  onSelect: (tabId: string) => void;
  // Shift on the close button asks to skip a confirmation.
  onClose: (tab: SFTPTab, options: { skipConfirmation: boolean }) => void;
  onAdd: () => void;
  onChangeHost?: (tab: SFTPTab) => void;
  canChangeHost?: (tab: SFTPTab) => boolean;
  onDragStart: (tabId: string) => void;
  onDragEnd: () => void;
  onMove: (tabId: string, side: PaneSide) => void;
  // A dragged tab let go over this strip goes into this pane before the tab now at index.
  onDropAt?: (index: number) => void;
  // Ctrl+Shift+Arrow moves the tab one place along this strip.
  onReorder?: (tabId: string, offset: -1 | 1) => void;
  // What the selected tab's "…" offers; a right-click on any tab selects it and opens the same menu.
  tabActions?: (tab: SFTPTab, index: number) => ActionMenuItem[];
}) {
  const t = useTranslate();
  // Where a dragged tab would land: before the tab at this index, or after the last one at tabs.length.
  const [dropIndex, setDropIndex] = useState<number | null>(null);
  const [menuOpenFor, setMenuOpenFor] = useState<string | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const size = compact ? "compact" : "regular";

  useEffect(() => {
    const selected = scroller.current?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]');
    // The destination and close controls belong to the selected tab too.
    selected?.parentElement?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [compact, pane.activeId, pane.tabs.length]);

  function beginDrag(event: DragEvent<HTMLElement>, tab: SFTPTab) {
    event.dataTransfer.setData(sftpTabMimeType, tab.id);
    event.dataTransfer.effectAllowed = "move";
    onDragStart(tab.id);
  }

  function keyDown(event: ReactKeyboardEvent<HTMLButtonElement>, tab: SFTPTab, index: number) {
    const sideways = event.key === "ArrowLeft" || event.key === "ArrowRight";
    if (sideways && event.shiftKey && (event.ctrlKey || event.metaKey) && onReorder !== undefined) {
      event.preventDefault();
      onReorder(tab.id, event.key === "ArrowLeft" ? -1 : 1);
      return;
    }
    if (sideways && event.shiftKey) {
      event.preventDefault();
      if (movable(tab)) onMove(tab.id, event.key === "ArrowLeft" ? "left" : "right");
      return;
    }
    activateTabFromKeyboard(event, index, pane.tabs, (target) => onSelect(target.id));
  }

  // The place a dragged tab would take: before the first tab whose middle is right of the pointer.
  function dropIndexAt(clientX: number): number {
    const tabs = Array.from(scroller.current?.querySelectorAll<HTMLElement>("[data-sftp-tab]") ?? []);
    const after = tabs.findIndex((element) => {
      const bounds = element.getBoundingClientRect();
      return clientX < bounds.left + bounds.width / 2;
    });
    return after < 0 ? tabs.length : after;
  }

  function dragOver(event: DragEvent<HTMLDivElement>) {
    if (onDropAt === undefined || !carriesTab(event)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setDropIndex(dropIndexAt(event.clientX));
  }

  function dragLeave(event: DragEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDropIndex(null);
  }

  function drop(event: DragEvent<HTMLDivElement>) {
    if (onDropAt === undefined || !carriesTab(event)) return;
    event.preventDefault();
    setDropIndex(null);
    onDropAt(dropIndexAt(event.clientX));
  }

  return (
    <div data-sftp-pane-tabs={pane.id} className={`flex min-w-0 shrink-0 items-stretch ${stripLook[size]}`}>
      <div
        ref={scroller}
        role="tablist"
        aria-label={label}
        onDragOver={dragOver}
        onDragLeave={dragLeave}
        onDrop={drop}
        className={`flex min-w-0 items-stretch gap-1 overflow-x-auto overscroll-x-contain [scrollbar-width:none] [&::-webkit-scrollbar]:hidden ${compact ? "flex-1" : "shrink"}`}
      >
        {pane.tabs.map((tab, index) => {
          const labels = tabLabels(tab, t("sftp.newTab"), t("sftp.local.connection"));
          const selected = tab.id === pane.activeId;
          const draggable = movable(tab);
          const actions = tabActions?.(tab, index);
          return (
            <span
              key={tab.id}
              data-sftp-tab={tab.id}
              draggable={draggable}
              onDragStart={draggable ? (event) => beginDrag(event, tab) : undefined}
              onDragEnd={draggable ? () => { setDropIndex(null); onDragEnd(); } : undefined}
              onContextMenu={actions === undefined ? undefined : (event) => {
                event.preventDefault();
                onSelect(tab.id);
                setMenuOpenFor(tab.id);
              }}
              className={`group relative flex w-52 shrink-0 items-stretch ${selected ? tabLook[size].selected : tabLook[size].other} ${draggable ? draggableCursor : ""}`}
            >
              {/* The marker is drawn inside the tab: the strip scrolls sideways and would clip it at its edges. */}
              {dropIndex === index ? <span aria-hidden="true" data-sftp-tab-drop-marker className="pointer-events-none absolute inset-y-1 left-0 w-0.5 rounded bg-accent" /> : null}
              {dropIndex === pane.tabs.length && index === pane.tabs.length - 1 ? <span aria-hidden="true" data-sftp-tab-drop-marker className="pointer-events-none absolute inset-y-1 right-0 w-0.5 rounded bg-accent" /> : null}
              <button
                type="button"
                role="tab"
                id={tabElementId(tab.id)}
                aria-selected={selected}
                aria-controls={tabPanelElementId(tab.id)}
                aria-keyshortcuts={[draggable ? "Shift+ArrowLeft Shift+ArrowRight" : "", onReorder === undefined ? "" : "Control+Shift+ArrowLeft Control+Shift+ArrowRight"].filter(Boolean).join(" ") || undefined}
                tabIndex={selected ? 0 : -1}
                aria-label={labels.name}
                title={labels.title || labels.name}
                onClick={() => onSelect(tab.id)}
                onKeyDown={(event) => keyDown(event, tab, index)}
                className={`flex min-w-0 flex-1 rounded py-0.5 text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent ${compact ? "px-2" : "pl-3 pr-1"} ${tabLayout[size]} ${selected ? "text-ink" : "text-ink-muted"}`}
              >
                {compact || tab.alias === "" ? null : <Icon name={tab.alias === localHostAlias ? "files" : "connections"} className="size-[13px] shrink-0" />}
                <span className={`block min-w-0 truncate leading-4 ${compact ? "w-full text-sm font-medium" : "text-[12.5px]"}`}>{labels.folder}</span>
                {labels.host === "" ? null : <span className={`block min-w-0 truncate text-[11px] leading-3.5 text-ink-muted ${compact ? "mt-0.5 w-full" : "max-w-[50%] shrink-0"}`}>{labels.host}</span>}
              </button>
              {selected && onChangeHost !== undefined ? (
                <button
                  type="button"
                  aria-label={t("sftp.host")}
                  aria-haspopup="dialog"
                  data-value={tab.alias}
                  title={t("sftp.chooseHostHeading")}
                  disabled={!canChangeHost(tab)}
                  onClick={() => onChangeHost(tab)}
                  className={`flex shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink ${controlSize[size]}`}
                >
                  <Icon name="chevronRight" className="size-3.5 rotate-90" />
                </button>
              ) : null}
              {selected && actions !== undefined ? (
                <ActionMenu
                  label={t("sftp.tabActions", { name: labels.name })}
                  items={actions}
                  open={menuOpenFor === tab.id}
                  onOpenChange={(open) => setMenuOpenFor(open ? tab.id : null)}
                  triggerClassName={`flex shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink ${controlSize[size]}`}
                  iconClassName="size-3.5"
                />
              ) : null}
              {closable ? (
                <button
                  type="button"
                  aria-label={t("sftp.closeTab", { name: labels.name })}
                  onClick={(event) => onClose(tab, { skipConfirmation: event.shiftKey })}
                  className={`flex shrink-0 items-center justify-center rounded text-ink-faint hover:bg-hover hover:text-danger ${controlSize[size]}`}
                >
                  <Icon name="close" className={compact ? "size-4" : "size-3"} />
                </button>
              ) : null}
            </span>
          );
        })}
      </div>
      <button
        type="button"
        aria-label={t("sftp.newTab")}
        disabled={addDisabled}
        onClick={onAdd}
        className={`flex shrink-0 items-center justify-center text-ink-muted hover:text-ink ${compact ? "rounded hover:bg-card/50 min-h-11 w-11" : "min-h-[30px] w-7 rounded-[7px] hover:bg-hover mobile-layout:min-h-10 mobile-layout:w-10"}`}
      >
        <Icon name="plus" className={compact ? "size-5" : "size-4"} />
      </button>
      {trailing}
    </div>
  );
}
