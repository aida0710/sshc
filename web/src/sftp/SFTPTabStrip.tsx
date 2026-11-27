import { useEffect, useRef, useState, type DragEvent, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { Icon } from "../ui/icons";
import { activateTabFromKeyboard, tabsScopeProps } from "../ui/tabKeyboard";
import { draggableCursor } from "../ui/dragAndDropLook";
import { localHostAlias } from "./localHost";
import { maxTabsPerPane, type PaneSide, type SFTPPane, type SFTPTab } from "./sftpPanes";
import { carriesTab, sftpTabMimeType } from "./SFTPTabDropTarget";
import { IconButton } from "../ui/IconButton";

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
// 広い画面では、タブの並びの下に線を引き、選んだタブの下だけ線を途切れさせる。選んだタブは線で囲み、下のパスの行と
// 同じ色にして、2つをひと続きに見せる（フォルダのタブの形）。
// 線は並びの一番下の1pxに描く（before:）。タブは、横にスクロールする箱の下の余白（pb-px）でその1px上に止まり、
// 選んだタブだけ -mb-px でその1pxまで下りる。スクロールする箱ははみ出しを切るので、下ろす先を箱の余白に置く。
// 選んだタブは線より後に描かれるので、線を隠す。選んでいないタブは、マウスを乗せても線を隠さない。
const stripLook = {
  compact: "gap-1 border-b border-line/70 bg-toolbar px-1 py-0.5",
  regular: "relative gap-1.5 px-3 pt-2.5 before:pointer-events-none before:absolute before:inset-x-0 before:bottom-0 before:h-px before:bg-line",
};
const scrollerLook = {
  compact: "flex-1",
  regular: "shrink pb-px",
};
const tabLook = {
  compact: { selected: "rounded-md bg-card shadow-card", other: "rounded-md hover:bg-card/50" },
  regular: { selected: "-mb-px rounded-t-md border border-b-0 border-line bg-well", other: "rounded-md hover:bg-hover" },
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
        {...tabsScopeProps}
        onDragEnter={dragOver}
        onDragOver={dragOver}
        onDragLeave={dragLeave}
        onDrop={drop}
        className={`flex min-w-0 items-stretch gap-1 overflow-x-auto overscroll-x-contain [scrollbar-width:none] [&::-webkit-scrollbar]:hidden ${scrollerLook[size]}`}
      >
        {/* A tablist may hold only tabs, and each tab here has its own buttons beside it, so the list owns the tabs by id. */}
        <div role="tablist" aria-label={label} aria-owns={pane.tabs.map((tab) => tabElementId(tab.id)).join(" ")} className="sr-only" />
        {pane.tabs.map((tab, index) => {
          const labels = tabLabels(tab, t("sftp.newTab"), t("sftp.local.connection"));
          const selected = tab.id === pane.activeId;
          // On a phone the path bar right under the selected tab says which folder it is.
          const hostOnly = compact && selected && labels.host !== "";
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
              {dropIndex === index ? <span aria-hidden="true" data-sftp-tab-drop-marker className="pointer-events-none absolute inset-y-1 left-0 w-0.5 rounded-full bg-accent" /> : null}
              {dropIndex === pane.tabs.length && index === pane.tabs.length - 1 ? <span aria-hidden="true" data-sftp-tab-drop-marker className="pointer-events-none absolute inset-y-1 right-0 w-0.5 rounded-full bg-accent" /> : null}
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
                className={`sshc-focus-inset flex min-w-0 flex-1 rounded-md py-0.5 text-left ${compact ? "px-2" : "pl-3 pr-1"} ${tabLayout[size]} ${selected ? "text-ink" : "text-ink-muted"}`}
              >
                {compact || tab.alias === "" ? null : <Icon name={tab.alias === localHostAlias ? "files" : "connections"} className="size-[13px] shrink-0" />}
                <span className={`block min-w-0 truncate leading-4 ${compact ? "w-full text-sm font-medium" : "text-xs"}`}>{hostOnly ? labels.host : labels.folder}</span>
                {labels.host === "" || hostOnly ? null : <span className={`block min-w-0 truncate text-2xs leading-3.5 text-ink-muted ${compact ? "mt-0.5 w-full" : "max-w-[50%] shrink-0"}`}>{labels.host}</span>}
              </button>
              {selected && onChangeHost !== undefined ? (
                <IconButton label={t("sftp.host")} size={compact ? "bar" : "tab"} className="sshc-focus-inset"
                  aria-haspopup="dialog"
                  data-value={tab.alias}
                  disabled={!canChangeHost(tab)}
                  onClick={() => onChangeHost(tab)}>
                  <Icon name="chevronRight" className="size-3.5 rotate-90" />
                </IconButton>
              ) : null}
              {selected && actions !== undefined ? (
                <ActionMenu
                  label={t("sftp.tabActions", { name: labels.name })}
                  items={actions}
                  open={menuOpenFor === tab.id}
                  onOpenChange={(open) => setMenuOpenFor(open ? tab.id : null)}
                  triggerSize={compact ? "bar" : "tab"}
                  iconClassName="size-3.5"
                />
              ) : null}
              {closable ? (
                <IconButton label={t("sftp.closeTab", { name: labels.name })} size={compact ? "bar" : "tab"} tone="danger" className="sshc-focus-inset"
                  onClick={(event) => onClose(tab, { skipConfirmation: event.shiftKey })}>
                  <Icon name="close" className={compact ? "size-4" : "size-3"} />
                </IconButton>
              ) : null}
            </span>
          );
        })}
      </div>
      <IconButton label={t("sftp.newTab")} size={compact ? "bar" : "tabWide"} ground={compact ? "card" : "hover"}
        disabled={addDisabled}
        onClick={onAdd}>
        <Icon name="plus" className={compact ? "size-5" : "size-4"} />
      </IconButton>
      {trailing}
    </div>
  );
}
