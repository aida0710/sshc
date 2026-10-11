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
  regular: "min-h-7 items-center gap-1.5 [@media(pointer:coarse)]:min-h-10",
};
const controlSize = {
  compact: "min-h-11 w-11",
  regular: "min-h-7 w-7 [@media(pointer:coarse)]:min-h-10 [@media(pointer:coarse)]:w-8",
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
}) {
  const t = useTranslate();
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
    if (event.shiftKey && (event.key === "ArrowLeft" || event.key === "ArrowRight")) {
      event.preventDefault();
      if (movable(tab)) onMove(tab.id, event.key === "ArrowLeft" ? "left" : "right");
      return;
    }
    activateTabFromKeyboard(event, index, pane.tabs, (target) => onSelect(target.id));
  }

  return (
    <div data-sftp-pane-tabs={pane.id} className="flex min-w-0 shrink-0 items-stretch gap-1 border-b border-line/70 bg-toolbar px-1 py-0.5">
      <div
        ref={scroller}
        role="tablist"
        aria-label={label}
        className="flex min-w-0 flex-1 items-stretch gap-1 overflow-x-auto overscroll-x-contain [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {pane.tabs.map((tab, index) => {
          const labels = tabLabels(tab, t("sftp.newTab"), t("sftp.local.connection"));
          const selected = tab.id === pane.activeId;
          const draggable = movable(tab);
          return (
            <span
              key={tab.id}
              draggable={draggable}
              onDragStart={draggable ? (event) => beginDrag(event, tab) : undefined}
              onDragEnd={draggable ? onDragEnd : undefined}
              className={`group relative flex w-52 shrink-0 items-stretch rounded ${selected ? "bg-card shadow-sm" : "hover:bg-card/50"} ${draggable ? "cursor-grab active:cursor-grabbing" : ""}`}
            >
              <button
                type="button"
                role="tab"
                id={tabElementId(tab.id)}
                aria-selected={selected}
                aria-controls={tabPanelElementId(tab.id)}
                aria-keyshortcuts={draggable ? "Shift+ArrowLeft Shift+ArrowRight" : undefined}
                tabIndex={selected ? 0 : -1}
                aria-label={labels.name}
                title={labels.title || labels.name}
                onClick={() => onSelect(tab.id)}
                onKeyDown={(event) => keyDown(event, tab, index)}
                className={`flex min-w-0 flex-1 rounded px-2 py-0.5 text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent ${tabLayout[size]} ${selected ? "text-ink" : "text-ink-muted"}`}
              >
                <span className={`block min-w-0 truncate text-sm font-medium leading-4 ${compact ? "w-full" : ""}`}>{labels.folder}</span>
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
                  className={`flex shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink disabled:text-ink-faint ${controlSize[size]}`}
                >
                  <Icon name="chevronRight" className="size-3.5 rotate-90" />
                </button>
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
        className={`flex shrink-0 items-center justify-center rounded text-ink-muted hover:bg-card/50 hover:text-ink disabled:text-ink-faint ${controlSize[size]}`}
      >
        <Icon name="plus" className={compact ? "size-5" : "size-4"} />
      </button>
      {trailing}
    </div>
  );
}
