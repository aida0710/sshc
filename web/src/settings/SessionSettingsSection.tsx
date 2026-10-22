import { useState } from "react";
import { useTranslate } from "../i18n/context";
import type { TerminalSessionsState } from "../terminal/sessions";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Button } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { ActionArea, SettingsSection } from "./SettingsSection";

export type SessionSettingsState = Pick<TerminalSessionsState, "sessions" | "busy" | "closeAll">;

// Closing every terminal session at once, with a confirmation when any is still live.
export function SessionSettingsSection({ terminalSessions, showHeading }: { terminalSessions: SessionSettingsState; showHeading: boolean }) {
  const t = useTranslate();
  const [confirming, setConfirming] = useState(false);
  // terminalSessions.busy は、どこで始めたセッションの操作でも立つ。回転の印は、ここで
  // 始めた「すべて閉じる」を待つ間だけ出す。
  const closing = useAsyncOperation();
  const live = terminalSessions.sessions.filter((session) => session.exited === undefined).length;
  const closeEverySession = () => void closing.run(() => terminalSessions.closeAll());
  return (
    <SettingsSection id="settings-connections" label={t("desktop.closeAllHeading")} icon="connections" showHeading={showHeading}>
      <p className="max-w-2xl text-sm leading-6 text-ink-muted">{t("desktop.closeAllNote")}</p>
      <ActionArea status={(
        <p role="status" className="text-sm text-ink-muted">
          <span className="mr-2 inline-block h-2 w-2 rounded-full bg-live" />
          {t("desktop.openCount", { count: terminalSessions.sessions.length })}
        </p>
      )}>
        <Button
          busy={closing.busy}
          disabled={terminalSessions.busy || terminalSessions.sessions.length === 0}
          onClick={() => live === 0 ? closeEverySession() : setConfirming(true)}
        >
          {t("desktop.closeAll")}
        </Button>
      </ActionArea>
      {confirming ? (
        <ConfirmDialog
          id="close-all-sessions-heading"
          heading={t("desktop.closeAllHeading2", { count: String(live) })}
          body={<p className="text-sm text-ink-muted">{t("desktop.closeAllBody")}</p>}
          confirmLabel={t("desktop.closeAllConfirm")}
          cancelLabel={t("desktop.closeAllCancel")}
          onCancel={() => setConfirming(false)}
          onConfirm={() => {
            setConfirming(false);
            closeEverySession();
          }}
        />
      ) : null}
    </SettingsSection>
  );
}
