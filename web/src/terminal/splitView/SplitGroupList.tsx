import type { TerminalSession } from "../../api/terminalSessions";
import { useTranslate } from "../../i18n/context";
import { ActionMenu } from "../../ui/ActionMenu";
import { SessionDestinationLine } from "../SessionDestinationLine";
import { sessionMarkerClass } from "../sessionStatus";
import type { UnreadSessions } from "../terminalNotifications";
import { terminalDisplayTitle } from "../terminalPresentation";
import { paneSessionIds } from "./layout";
import type { SplitGroup } from "./splitGroups";
import { SplitLayoutMap } from "./SplitLayoutMap";

// The split groups in the sidebar. Each group shows how its panes are arranged and, under it,
// the sessions it holds, so it is clear what opening it will show.
export function SplitGroupList({ groups, sessions, selected, unreadBySession = new Set(), onSelect, onSelectSession, onDissolve }: {
  groups: SplitGroup[];
  sessions: TerminalSession[];
  selected: number | null;
  unreadBySession?: UnreadSessions;
  onSelect: (number: number) => void;
  // onSelectSession opens a session of a group, which shows the group with that pane focused.
  onSelectSession: (sessionId: string) => void;
  onDissolve: (number: number) => void;
}) {
  const t = useTranslate();
  const sessionById = new Map(sessions.map((session) => [session.id, session]));
  if (groups.length === 0) return null;
  return (
    <div>
      {/* Like the Workspaces heading below and the group headings of the connection list, this heading stays at
          the top of the sidebar's list while its rows scroll under it, until the Workspaces heading takes its place. */}
      <span aria-hidden="true" className="sticky top-0 z-[2] block bg-sidebar px-2 pb-1 pt-[18px] text-xs text-ink-faint">{t("splitView.heading")}</span>
      <ul aria-label={t("splitView.heading")} className="flex flex-col gap-0.5">
        {groups.map((group) => (
          <SplitGroupRow key={group.number} group={group} selected={selected === group.number}
            members={paneSessionIds(group.view.root).flatMap((id) => {
              const session = sessionById.get(id);
              return session === undefined ? [] : [session];
            })}
            unreadBySession={unreadBySession}
            onSelect={() => onSelect(group.number)} onSelectSession={onSelectSession} onDissolve={() => onDissolve(group.number)} />
        ))}
      </ul>
    </div>
  );
}

function SplitGroupRow({ group, members, selected, unreadBySession, onSelect, onSelectSession, onDissolve }: {
  group: SplitGroup;
  members: TerminalSession[];
  selected: boolean;
  unreadBySession: UnreadSessions;
  onSelect: () => void;
  onSelectSession: (sessionId: string) => void;
  onDissolve: () => void;
}) {
  const t = useTranslate();
  const name = t("splitView.groupName", { number: group.number });
  // The shown group's active pane is the one typing goes to. Its row is framed like that pane on the screen.
  const activeSessionId = selected ? group.view.activeSessionId : null;
  return (
    <li data-split-group={group.number}>
      <div className={`flex items-center rounded-[7px] pr-1 ${selected ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-hover"}`}>
        <button type="button" aria-label={name} aria-current={selected ? "true" : undefined} onClick={onSelect}
          className="flex min-h-10 min-w-0 grow items-center gap-2.5 pl-2 text-left md:min-h-[30px]">
          <SplitLayoutMap root={group.view.root} className="h-[18px] w-[26px]" />
          <span className="min-w-0 grow truncate text-[13px]">{name}</span>
          {/* Only the number: the rows below say what they are. */}
          <span aria-hidden="true" className="shrink-0 text-[11px] text-ink-faint">{members.length}</span>
        </button>
        <ActionMenu
          label={t("splitView.groupActions", { name })}
          items={[{ label: t("splitView.dissolve"), onSelect: onDissolve }]}
          triggerClassName="ml-1 flex size-9 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-ink md:size-6"
          iconClassName="size-3.5"
        />
      </div>
      <ul aria-label={t("splitView.groupSessions", { name })}>
        {members.map((session) => {
          const title = terminalDisplayTitle(session);
          const active = session.id === activeSessionId;
          return (
            <li key={session.id}>
              {/* The dot sits where the dots of the workspace rows below sit. */}
              <button type="button" aria-label={t("splitView.groupSession", { name, title })} aria-describedby={`split-member-${session.id}`} aria-current={active ? "true" : undefined} onClick={() => onSelectSession(session.id)}
                className={`flex min-h-10 w-full min-w-0 items-start gap-2.5 rounded-[7px] py-1 pl-4 pr-2 text-left hover:bg-hover md:min-h-0 md:py-[3px] ${active ? "ring-1 ring-inset ring-accent/45" : ""}`}>
                <span aria-hidden="true" className={`mt-[5px] size-2 shrink-0 rounded-full ${sessionMarkerClass(session, unreadBySession.has(session.id))}`} />
                {/* As in the workspace rows, the name gets the whole line and the destination is under it. */}
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[12.5px] leading-[18px] text-ink">{title}</span>
                  <SessionDestinationLine session={session} id={`split-member-${session.id}`} />
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </li>
  );
}
