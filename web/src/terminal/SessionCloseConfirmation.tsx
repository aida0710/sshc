import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";

// SessionCloseConfirmation asks before a live session is closed, from its row's × or the close session shortcut.
export function SessionCloseConfirmation({
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
      id="close-session-heading"
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
