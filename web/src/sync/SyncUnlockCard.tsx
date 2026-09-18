import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { PasswordField } from "../ui/PasswordField";
import { Button, Card } from "../ui/surface";

// The bucket credentials sit in the vault, so a locked vault means the
// screen can only ask for the master password.
export function SyncUnlockCard({ master, busy, onMasterChange, onUnlock }: {
  master: string;
  busy: boolean;
  onMasterChange: (value: string) => void;
  onUnlock: () => void;
}) {
  const t = useTranslate();
  return (
    <Card as="section" radius="md" className="grid md:grid-cols-[minmax(0,0.9fr)_minmax(18rem,1.1fr)]">
      <div className="flex flex-col justify-between gap-8 bg-toolbar p-6 md:p-8">
        <span className="flex h-12 w-12 items-center justify-center rounded-md bg-select-fill text-accent">
          <Icon name="sync" className="h-6 w-6" />
        </span>
        <div>
          <h3 className="text-lg font-semibold text-ink">
            {t("sync.bucketHeading")}
          </h3>
          <p className="mt-2 text-sm leading-6 text-ink-muted">
            {t("sync.sealed")}
          </p>
        </div>
      </div>
      <div className="flex flex-col justify-center gap-4 p-6 md:p-8">
        <PasswordField
          label={t("secrets.master")}
          value={master}
          onChange={onMasterChange}
        />
        <Button
          kind="primary"
          disabled={busy || master === ""}
          onClick={onUnlock}
          className="self-start"
        >
          {t("secrets.unlock")}
        </Button>
      </div>
    </Card>
  );
}
