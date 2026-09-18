import type { SyncPushDraft, SyncStatus } from "../api/sync";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { control, hintText, sectionHeading } from "../ui/form";
import { Button, Card } from "../ui/surface";

// The push side: a commit message for the local changes the engine counted,
// and the buttons that send or preview them.
export function SyncTransferCard({ status, busy, pushDraft, pushMessage, onMessageChange, onPush, onPreview }: {
  status: SyncStatus;
  busy: boolean;
  pushDraft: SyncPushDraft | null;
  pushMessage: string;
  onMessageChange: (value: string) => void;
  onPush: () => void;
  onPreview: () => void;
}) {
  const t = useTranslate();
  const pushChangeCount =
    pushDraft === null
      ? null
      : pushDraft.added + pushDraft.modified + pushDraft.removed;
  return (
    <Card
      as="section"
      aria-labelledby="sync-transfer-heading"
      radius="md"
      className="flex flex-col gap-3 p-4"
    >
      <h3 id="sync-transfer-heading" className={sectionHeading}>
        {t("sync.transferHeading")}
      </h3>
      <p className="text-sm leading-6 text-ink-muted">
        {t(`sync.transferHint.${status.direction}` as MessageKey)}
      </p>
      <div className="flex flex-col gap-1 border-t border-line pt-3 text-sm text-ink">
        <label htmlFor="sync-commit-message" className="font-medium">
          {t("sync.commitMessage")}
        </label>
        <input
          id="sync-commit-message"
          aria-describedby="sync-commit-message-hint"
          value={pushMessage}
          maxLength={240}
          onChange={(event) => onMessageChange(event.target.value)}
          className={control}
          placeholder={t("sync.commitMessagePlaceholder")}
        />
        <span id="sync-commit-message-hint" className={hintText}>
          {pushDraft === null
            ? t("sync.commitMessageHint")
            : pushChangeCount === 0
              ? t("sync.noLocalChanges")
              : t("sync.commitMessageChanges", {
                  added: pushDraft.added,
                  modified: pushDraft.modified,
                  removed: pushDraft.removed,
                })}
        </span>
      </div>
      <div className="flex flex-wrap gap-2 border-t border-line pt-3">
        <Button
          kind="primary"
          disabled={
            busy ||
            !status.keyConfigured ||
            pushMessage.trim() === "" ||
            pushChangeCount === null ||
            pushChangeCount === 0
          }
          onClick={onPush}
        >
          {t("sync.push")}
        </Button>
        <Button
          disabled={busy || !status.keyConfigured}
          onClick={onPreview}
        >
          {t("sync.preview")}
        </Button>
      </div>
    </Card>
  );
}
