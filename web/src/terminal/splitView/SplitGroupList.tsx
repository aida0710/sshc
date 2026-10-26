import { useState } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";
import { terminalDisplayTitle } from "../terminalPresentation";
import { paneSessionIds } from "./layout";
import type { SplitGroup } from "./splitGroups";

export function SplitGroupList({ groups, sessions, selected, onSelect, onDissolve }: {
  groups: SplitGroup[];
  sessions: TerminalSession[];
  selected: number | null;
  onSelect: (number: number) => void;
  onDissolve: (number: number) => void;
}) {
  const t = useTranslate();
  const sessionById = new Map(sessions.map((session) => [session.id, session]));
  if (groups.length === 0) return null;
  return (
    <div>
      <span aria-hidden="true" className="block px-2 pb-1 pt-[18px] text-xs text-ink-faint">{t("splitView.heading")}</span>
      <ul aria-label={t("splitView.heading")} className="flex flex-col gap-0.5">
        {groups.map((group) => (
          <SplitGroupRow key={group.number} group={group} selected={selected === group.number}
            titles={paneSessionIds(group.view.root).map((id) => {
              const session = sessionById.get(id);
              return session === undefined ? "" : terminalDisplayTitle(session);
            })}
            onSelect={() => onSelect(group.number)} onDissolve={() => onDissolve(group.number)} />
        ))}
      </ul>
    </div>
  );
}

function SplitGroupRow({ group, titles, selected, onSelect, onDissolve }: {
  group: SplitGroup;
  titles: string[];
  selected: boolean;
  onSelect: () => void;
  onDissolve: () => void;
}) {
  const t = useTranslate();
  const [expanded, setExpanded] = useState(true);
  const name = t("splitView.groupName", { number: group.number });
  const summary = titles.join(t("splitView.sessionSeparator"));
  return (
    <li data-split-group={group.number}>
      <div className={`flex items-center rounded-[7px] pr-1 ${selected ? "bg-select-fill text-ink" : "text-ink-muted hover:bg-hover"}`}>
        <button type="button" aria-label={t("splitView.toggleGroup", { name })} aria-expanded={expanded}
          onClick={() => setExpanded(!expanded)} className="flex size-9 shrink-0 items-center justify-center rounded hover:bg-hover md:h-6 md:w-4">
          <Icon name="chevronRight" className={`size-3 transition-transform ${expanded ? "rotate-90" : ""}`} />
        </button>
        <button type="button" aria-label={name} aria-current={selected ? "true" : undefined} onClick={onSelect}
          className="flex min-h-9 min-w-0 grow items-center gap-2 text-left md:min-h-[30px]">
          <Icon name="splitView" className={`size-4 ${selected ? "text-accent" : ""}`} />
          <span className="min-w-0 grow truncate text-[13px]">{name}</span>
          <span className="shrink-0 text-[11px] text-ink-faint">{t("splitView.paneCount", { count: titles.length })}</span>
        </button>
        <button type="button" aria-label={t("splitView.dissolveGroup", { name })} onClick={onDissolve}
          className="ml-1 min-h-9 shrink-0 rounded-md px-1.5 text-[11px] text-ink-faint hover:bg-hover hover:text-ink md:min-h-6">
          {t("splitView.dissolve")}
        </button>
      </div>
      {expanded ? <p title={summary} className="truncate pb-1 pl-10 pr-1 text-[11px] text-ink-faint">{summary}</p> : null}
    </li>
  );
}
