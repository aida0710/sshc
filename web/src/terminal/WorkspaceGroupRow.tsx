import { useState, type MouseEvent, type RefObject } from "react";
import type { LiveWorkspaceSummary } from "../features/workspaces/live";
import { useTranslate } from "../i18n/context";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { Icon } from "../ui/icons";

// The row that stands for a split in the terminal list: it opens the split,
// folds its terminals in or out, and renames it. Its menu is opened and
// closed by the list, which keeps one row menu open at a time.
export function WorkspaceGroupRow({
  workspace,
  memberCount,
  unread,
  selected,
  expanded,
  onToggleExpanded,
  onOpen,
  onRename,
  menuOpen,
  menuRef,
  onToggleMenu,
  onCloseMenu,
}: {
  workspace: LiveWorkspaceSummary;
  memberCount: number;
  unread: boolean;
  selected: boolean;
  expanded: boolean;
  onToggleExpanded: () => void;
  onOpen: () => void;
  onRename: (name: string) => void;
  menuOpen: boolean;
  menuRef: RefObject<HTMLDivElement | null>;
  onToggleMenu: (event: MouseEvent<HTMLButtonElement>) => void;
  onCloseMenu: () => void;
}) {
  const t = useTranslate();
  const [renaming, setRenaming] = useState(false);
  const [draft, setDraft] = useState("");
  const count = <span className="block text-xs text-ink-faint">{t("workspace.groupCount", { count: String(memberCount) })}</span>;

  function commitRename() {
    const wanted = draft.trim();
    setRenaming(false);
    if (wanted === "" || wanted === workspace.name) return;
    onRename(wanted);
  }

  return (
    <li className="relative rounded-lg border border-control-line bg-control/70 p-1">
      <div className={`flex items-center gap-1 rounded-md ${selected ? "bg-select-fill" : ""}`}>
        <button
          type="button"
          aria-label={t(expanded ? "workspace.collapseGroup" : "workspace.expandGroup", { name: workspace.name })}
          aria-expanded={expanded}
          onClick={onToggleExpanded}
          className="flex size-7 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover"
        >
          <DisclosureChevron expanded={expanded} className="size-3.5" />
        </button>
        {renaming ? (
          <div className="min-w-0 grow px-1 py-1">
            <input
              autoFocus
              aria-label={t("workspace.renameLabel", { name: workspace.name })}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onBlur={commitRename}
              onKeyDown={(event) => {
                if (event.key === "Enter") commitRename();
                if (event.key === "Escape") setRenaming(false);
              }}
              className="w-full rounded border border-accent bg-card px-1 py-0.5 text-sm text-ink"
            />
            {count}
          </div>
        ) : (
          <button type="button" aria-label={workspace.name} onClick={onOpen} className="min-w-0 grow px-1 py-1 text-left">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="block min-w-0 truncate text-sm font-medium text-ink">{workspace.name}</span>
              {unread ? (
                <span aria-label={t("terminal.unreadWorkspace")} className="size-2 shrink-0 rounded-full bg-accent" />
              ) : null}
            </span>
            {count}
          </button>
        )}
        <button
          type="button"
          aria-label={t("workspace.rowMenu", { name: workspace.name })}
          aria-expanded={menuOpen}
          onClick={onToggleMenu}
          className="hidden size-7 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover md:flex"
        >
          <Icon name="moreHorizontal" className="size-3.5" />
        </button>
      </div>
      {menuOpen ? (
        <div
          ref={menuRef}
          role="menu"
          aria-label={t("workspace.rowMenu", { name: workspace.name })}
          className="absolute right-1 top-full z-10 mt-0.5 hidden w-48 rounded-lg border border-control-line bg-card p-1 shadow-lg md:block"
        >
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setDraft(workspace.name);
              setRenaming(true);
              onCloseMenu();
            }}
            className="block w-full rounded px-2 py-1.5 text-left text-xs text-ink hover:bg-hover"
          >
            {t("workspace.rename")}
          </button>
        </div>
      ) : null}
    </li>
  );
}
