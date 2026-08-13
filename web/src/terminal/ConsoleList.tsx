import type { TerminalSession } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";

type ConsoleListProps = {
  sessions: TerminalSession[];
  selected: string | null;
  maxSessions: number;
  busy: boolean;
  problem: string;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onOpenShell: () => void;
};

// ConsoleList は、開いているセッションと終了して残っているセッションの一覧である。
//
// ローカルシェルの入口はここだけである。localhost はローカルシェルであって ssh
// 接続ではないので、Home の接続一覧には出さない。あの一覧は ~/.ssh/config の
// 投影であり、localhost はそこに存在しない。
export function ConsoleList({
  sessions,
  selected,
  maxSessions,
  busy,
  problem,
  onSelect,
  onClose,
  onOpenShell,
}: ConsoleListProps) {
  const t = useTranslate();
  const live = sessions.filter((session) => session.exited === undefined).length;
  const full = live >= maxSessions;

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
        <ul aria-label={t("terminal.consoleList")} className="flex flex-col gap-0.5">
          {sessions.map((session) => {
            const running = session.exited === undefined;
            return (
              <li key={session.id} className="flex items-center gap-1">
                <button
                  type="button"
                  aria-current={session.id === selected ? "true" : undefined}
                  onClick={() => onSelect(session.id)}
                  className={`flex min-w-0 grow items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm ${
                    session.id === selected ? "bg-select-fill text-ink" : "text-ink hover:bg-select-fill"
                  }`}
                >
                  {/*
                    色は状態にだけ使う、という既存の規則の範囲に収まっている。
                    緑は生きているセッション、灰は終了済みである。点だけでは
                    伝わらないので、右の語がそれを言葉でも言う。
                  */}
                  <span
                    aria-hidden="true"
                    className={`size-1.5 shrink-0 rounded-full ${running ? "bg-live" : "bg-ink-faint"}`}
                  />
                  <span className="min-w-0 grow truncate">{session.title}</span>
                  <span className="shrink-0 text-xs text-ink-muted">
                    {running
                      ? session.kind === "ssh"
                        ? t("terminal.kindSsh")
                        : t("terminal.kindShell")
                      : t("terminal.exited")}
                  </span>
                </button>
                <button
                  type="button"
                  aria-label={t("terminal.closeSession", { title: session.title })}
                  onClick={() => onClose(session.id)}
                  className="flex size-7 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill"
                >
                  <Icon name="close" className="size-3.5" />
                </button>
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
    </div>
  );
}
