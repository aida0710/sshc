import { useState } from "react";
import type { Translate } from "../i18n/context";
import { Field, control, sectionHeading } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { ModalShell } from "../ui/ModalShell";

type SyncForcePushDialogProps = {
  busy: boolean;
  keyConfigured: boolean;
  message: string;
  t: Translate;
  onMessageChange: (message: string) => void;
  onClose: () => void;
  onSubmit: () => void;
};

export function SyncForcePushDialog({
  busy,
  keyConfigured,
  message,
  t,
  onMessageChange,
  onClose,
  onSubmit,
}: SyncForcePushDialogProps) {
  const [confirmed, setConfirmed] = useState(false);
  return (
    <ModalShell
      labelledBy="sync-force-push-heading"
      onDismiss={() => {
        if (!busy) onClose();
      }}
      placement="sheet"
      panelClassName="flex max-h-[90vh] w-full max-w-lg flex-col overflow-auto rounded-lg"
    >
        <header className="flex items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
          <h3 id="sync-force-push-heading" className={sectionHeading}>
            {t("sync.forceHeading")}
          </h3>
          <Button onClick={onClose} disabled={busy}>
            {t("sync.dialogClose")}
          </Button>
        </header>
        <div className="flex flex-col gap-4 p-4">
          <Notice tone="danger">{t("sync.forceHint")}</Notice>
          <Field label={t("sync.commitMessage")}>
            <input
              value={message}
              maxLength={240}
              onChange={(event) => onMessageChange(event.target.value)}
              className={control}
            />
          </Field>
          <label className="flex items-start gap-2 text-sm text-ink">
            <input
              type="checkbox"
              checked={confirmed}
              onChange={(event) => setConfirmed(event.target.checked)}
              className="mt-0.5"
            />
            <span>{t("sync.forceConfirm")}</span>
          </label>
          <Button
            kind="danger"
            disabled={
              busy || !confirmed || !keyConfigured || message.trim() === ""
            }
            onClick={onSubmit}
            className="self-start"
          >
            {t("sync.forcePush")}
          </Button>
        </div>
    </ModalShell>
  );
}
