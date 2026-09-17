import { useState } from "react";
import { useTranslate } from "../i18n/context";
import type { TerminalSessionsState } from "../terminal/sessions";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Button } from "../ui/surface";
import { ActionArea, SettingsSection } from "./SettingsSection";

export type ConsoleSettingsConsoles = Pick<TerminalSessionsState, "sessions" | "busy" | "closeAll">;

// Closing every console at once, with a confirmation when any is still live.
export function ConsoleSettingsSection({ consoles, showHeading }: { consoles: ConsoleSettingsConsoles; showHeading: boolean }) {
  const t = useTranslate();
  const [confirming, setConfirming] = useState(false);
  const live = consoles.sessions.filter((session) => session.exited === undefined).length;
  return (
    <SettingsSection id="settings-connections" label={t("desktop.closeAllHeading")} icon="connections" showHeading={showHeading}>
      <p className="max-w-2xl text-sm leading-6 text-ink-muted">{t("desktop.closeAllNote")}</p>
      <ActionArea status={(
        <p role="status" className="text-sm text-ink-muted">
          <span className="mr-2 inline-block h-2 w-2 rounded-full bg-live" />
          {t("desktop.openCount", { count: consoles.sessions.length })}
        </p>
      )}>
        <Button
          disabled={consoles.busy || consoles.sessions.length === 0}
          onClick={() => live === 0 ? void consoles.closeAll() : setConfirming(true)}
        >
          {t("desktop.closeAll")}
        </Button>
      </ActionArea>
      {confirming ? (
        <ConfirmDialog
          id="close-all-consoles-heading"
          heading={t("desktop.closeAllHeading2", { count: String(live) })}
          body={<p className="text-sm text-ink-muted">{t("desktop.closeAllBody")}</p>}
          confirmLabel={t("desktop.closeAllConfirm")}
          cancelLabel={t("desktop.closeAllCancel")}
          onCancel={() => setConfirming(false)}
          onConfirm={() => {
            setConfirming(false);
            void consoles.closeAll();
          }}
        />
      ) : null}
    </SettingsSection>
  );
}
