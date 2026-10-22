import { useTranslate } from "../i18n/context";
import { ActionMenu, revealOnHover, type ActionMenuItem } from "../ui/ActionMenu";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { Icon } from "../ui/icons";

// One workspace in the sidebar tree: opens and closes its sessions, starts a
// session in it, and holds its own actions.
export function WorkspaceRow({
  name,
  sessionCount,
  hidden,
  expanded,
  unread,
  canOpenSession,
  actions,
  onToggle,
  onNewSession,
}: {
  name: string;
  sessionCount: number;
  hidden: boolean;
  expanded: boolean;
  unread: boolean;
  canOpenSession: boolean;
  actions: readonly ActionMenuItem[];
  onToggle: () => void;
  onNewSession: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="group/row flex items-center gap-1 rounded-md px-1 py-0.5 md:py-0">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={onToggle}
        className="flex min-h-10 min-w-0 grow items-center gap-1.5 rounded-md px-1 text-left text-sm hover:bg-hover md:min-h-6"
      >
        <DisclosureChevron expanded={expanded} className="size-3.5 shrink-0 text-ink-faint" />
        {/* The workspace name labels its sessions, so it stays quieter than they are. */}
        <span className="min-w-0 grow truncate text-xs font-normal text-ink-faint">{name}</span>
        {hidden ? <span className="sr-only">{t("workspace.hiddenMark")}</span> : null}
        {unread && !expanded ? (
          <span aria-label={t("terminal.unreadNotification")} className="size-2 shrink-0 rounded-full bg-accent" />
        ) : null}
        <span className="shrink-0 text-[11px] text-ink-faint">{sessionCount}</span>
      </button>
      <button
        type="button"
        aria-label={t("workspace.newSessionIn", { name })}
        title={t("workspace.newSessionIn", { name })}
        disabled={!canOpenSession}
        onClick={onNewSession}
        className={`flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover disabled:text-ink-faint md:size-5 ${revealOnHover}`}
      >
        <Icon name="plus" className="size-3.5" />
      </button>
      <ActionMenu
        label={t("workspace.actions", { name })}
        items={actions}
        triggerClassName={`flex size-9 items-center justify-center rounded-md text-ink-muted hover:bg-hover md:size-5 ${revealOnHover}`}
        iconClassName="size-3.5"
      />
    </div>
  );
}
