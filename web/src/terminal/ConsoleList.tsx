import { useEffect, useRef, useState, type DragEvent } from "react";
import type { TerminalForward, TerminalSession } from "../api/integrations";
import { useTranslate, type Translate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Icon } from "../ui/icons";

type ConsoleListProps = {
  sessions: TerminalSession[];
  selected: string | null;
  maxSessions: number;
  busy: boolean;
  problem: string;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onRename: (id: string, title: string) => Promise<boolean>;
  onDuplicate: (id: string) => void;
  onReorder: (order: string[]) => void;
  onOpenShell: () => void;
};

const dragMimeType = "application/x-sshc-console";

function describeForward(t: Translate, forward: TerminalForward): string {
  switch (forward.kind) {
    case "agent":
      return t("terminal.forwardAgent");
    case "dynamic":
      return t("terminal.forwardDynamic", { listen: forward.listen });
    default:
      return t("terminal.forwardLocal", { listen: forward.listen, to: forward.to });
  }
}

export function ConsoleList({
  sessions,
  selected,
  maxSessions,
  busy,
  problem,
  onSelect,
  onClose,
  onRename,
  onDuplicate,
  onReorder,
  onOpenShell,
}: ConsoleListProps) {
  const t = useTranslate();
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [closing, setClosing] = useState<TerminalSession | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [dragging, setDragging] = useState<string | null>(null);
  const [dropBefore, setDropBefore] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);

  const live = sessions.filter((session) => session.exited === undefined).length;
  const full = maxSessions > 0 && live >= maxSessions;

  useEffect(() => {
    if (menuFor !== null && !sessions.some((session) => session.id === menuFor)) setMenuFor(null);
    if (renaming !== null && !sessions.some((session) => session.id === renaming)) setRenaming(null);
  }, [sessions, menuFor, renaming]);

  useEffect(() => {
    if (menuFor === null) return;
    function dismiss(event: PointerEvent) {
      if (!menuRef.current?.contains(event.target as Node)) setMenuFor(null);
    }
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [menuFor]);

  function move(id: string, delta: number) {
    const order = sessions.map((session) => session.id);
    const from = order.indexOf(id);
    const to = from + delta;
    if (from < 0 || to < 0 || to >= order.length) return;
    order.splice(to, 0, ...order.splice(from, 1));
    onReorder(order);
    setMenuFor(null);
  }

  function drop(targetId: string | null) {
    const held = dragging;
    setDragging(null);
    setDropBefore(null);
    if (held === null || held === targetId) return;
    const order = sessions.map((session) => session.id).filter((id) => id !== held);
    const at = targetId === null ? order.length : order.indexOf(targetId);
    order.splice(at < 0 ? order.length : at, 0, held);
    onReorder(order);
  }

  async function commitRename(id: string) {
    const wanted = draft.trim();
    setRenaming(null);
    const current = sessions.find((session) => session.id === id)?.title ?? "";
    if (wanted === "" || wanted === current) return;
    await onRename(id, wanted);
  }

  return (
    <div className="flex flex-col gap-2">
      {problem === "" ? null : (
        <p role="alert" className="rounded-md border border-notice-line bg-notice px-2 py-1.5 text-xs text-notice-ink">
          {problem}
        </p>
      )}
      {sessions.length === 0 ? (
        <p className="px-1 text-xs text-ink-muted">{t("terminal.noSessions")}</p>
      ) : (
        <ul
          aria-label={t("terminal.consoleList")}
          className="flex flex-col gap-0.5"
          onDragOver={(event: DragEvent) => {
            if (event.dataTransfer.types.includes(dragMimeType)) event.preventDefault();
          }}
          onDrop={() => drop(null)}
        >
          {sessions.map((session, index) => {
            const running = session.exited === undefined;
            const destination = session.kind === "ssh" ? session.alias ?? "" : t("terminal.localhost");
            const status = running
              ? t("terminal.running")
              : t("terminal.exitedWith", { code: String(session.exited?.code ?? 0) });
            return (
              <li
                key={session.id}
                draggable={renaming === null}
                onDragStart={(event: DragEvent) => {
                  event.dataTransfer.setData(dragMimeType, session.id);
                  event.dataTransfer.effectAllowed = "move";
                  setDragging(session.id);
                }}
                onDragEnd={() => {
                  setDragging(null);
                  setDropBefore(null);
                }}
                onDragOver={(event: DragEvent) => {
                  if (!event.dataTransfer.types.includes(dragMimeType)) return;
                  event.preventDefault();
                  setDropBefore(session.id);
                }}
                onDrop={(event: DragEvent) => {
                  event.stopPropagation();
                  drop(session.id);
                }}
                className={`relative ${dragging === session.id ? "opacity-40" : ""}`}
              >
                {dropBefore === session.id && dragging !== session.id ? (
                  <span aria-hidden="true" className="absolute inset-x-2 -top-px block h-0.5 rounded bg-accent" />
                ) : null}
                <div
                  className={`flex items-start gap-2 rounded-md px-2 py-1.5 ${
                    session.id === selected ? "bg-select-fill" : "hover:bg-select-fill"
                  }`}
                >

                  <span
                    aria-hidden="true"
                    className={`mt-1.5 size-1.5 shrink-0 rounded-full ${running ? "bg-live" : "bg-ink-faint"}`}
                  />
                  <div className="min-w-0 grow">
                    {renaming === session.id ? (
                      <input
                        autoFocus
                        aria-label={t("terminal.renameLabel", { title: session.title })}
                        value={draft}
                        onChange={(event) => setDraft(event.target.value)}
                        onBlur={() => void commitRename(session.id)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter") void commitRename(session.id);
                          if (event.key === "Escape") setRenaming(null);
                        }}
                        className="w-full rounded border border-accent bg-card px-1 py-0.5 text-sm text-ink"
                      />
                    ) : (
                      <button
                        type="button"
                        aria-current={session.id === selected ? "true" : undefined}
                        onClick={() => onSelect(session.id)}
                        className="block w-full truncate text-left text-sm text-ink"
                      >
                        {session.title}
                      </button>
                    )}
                    <p className="truncate text-xs text-ink-faint">
                      {t("terminal.rowDetail", { status, destination })}
                    </p>

                    {(session.forwards ?? []).map((forward) => (
                      <p
                        key={`${forward.kind}:${forward.listen}:${forward.to}`}
                        className={`truncate text-xs ${forward.problem === "" ? "text-ink-faint" : "text-notice-ink"}`}
                      >
                        {forward.problem === "" ? describeForward(t, forward) : forward.problem}
                      </p>
                    ))}
                  </div>
                  <button
                    type="button"
                    aria-label={t("terminal.rowMenu", { title: session.title })}
                    aria-expanded={menuFor === session.id}
                    onClick={() => setMenuFor(menuFor === session.id ? null : session.id)}
                    className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill"
                  >
                    <Icon name="moreHorizontal" className="size-3.5" />
                  </button>
                  <button
                    type="button"
                    aria-label={t("terminal.closeSession", { title: session.title })}
                    onClick={() =>
                      session.exited === undefined ? setClosing(session) : onClose(session.id)
                    }
                    className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill"
                  >
                    <Icon name="close" className="size-3.5" />
                  </button>
                </div>
                {menuFor === session.id ? (
                  <div
                    ref={menuRef}
                    role="menu"
                    aria-label={t("terminal.rowMenu", { title: session.title })}
                    className="absolute right-1 z-10 mt-0.5 w-48 rounded-lg border border-control-line bg-card p-1 shadow-lg"
                  >
                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        setDraft(session.title);
                        setRenaming(session.id);
                        setMenuFor(null);
                      }}
                      className="block w-full rounded px-2 py-1.5 text-left text-xs text-ink hover:bg-select-fill"
                    >
                      {t("terminal.rename")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      disabled={busy || full}
                      onClick={() => {
                        onDuplicate(session.id);
                        setMenuFor(null);
                      }}
                      className="block w-full rounded px-2 py-1.5 text-left text-xs text-ink hover:bg-select-fill disabled:text-ink-faint"
                    >
                      {t("terminal.duplicate")}
                    </button>

                    <button
                      type="button"
                      role="menuitem"
                      disabled={index === 0}
                      onClick={() => move(session.id, -1)}
                      className="block w-full rounded px-2 py-1.5 text-left text-xs text-ink hover:bg-select-fill disabled:text-ink-faint"
                    >
                      {t("terminal.moveUp")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      disabled={index === sessions.length - 1}
                      onClick={() => move(session.id, 1)}
                      className="block w-full rounded px-2 py-1.5 text-left text-xs text-ink hover:bg-select-fill disabled:text-ink-faint"
                    >
                      {t("terminal.moveDown")}
                    </button>
                  </div>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
      <button
        type="button"
        disabled={busy || full}
        onClick={onOpenShell}
        className="flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-ink hover:bg-select-fill disabled:text-ink-faint"
      >
        <Icon name="plus" className="size-3.5" aria-hidden="true" />
        {t("terminal.openShell")}
      </button>
      {full ? <p className="px-2 text-xs text-ink-muted">{t("terminal.limitReached", { max: maxSessions })}</p> : null}
      {closing === null ? null : (
        <CloseConfirmation
          session={closing}
          onCancel={() => setClosing(null)}
          onConfirm={() => {
            onClose(closing.id);
            setClosing(null);
          }}
        />
      )}
    </div>
  );
}
function CloseConfirmation({
  session,
  onCancel,
  onConfirm,
}: {
  session: TerminalSession;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const t = useTranslate();
  const forwards = (session.forwards ?? []).length;
  return (
    <ConfirmDialog
      id="close-console-heading"
      heading={t("terminal.closeHeading", { title: session.title })}
      body={
        <>
          <p className="text-sm text-ink-muted">{t("terminal.closeBody")}</p>
          {forwards === 0 ? null : (
            <p className="text-sm text-ink-muted">
              {t("terminal.closeForwards", { count: String(forwards) })}
            </p>
          )}
        </>
      }
      confirmLabel={t("terminal.closeConfirm")}
      cancelLabel={t("terminal.closeCancel")}
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
}
