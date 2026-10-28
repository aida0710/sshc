import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { PageSection } from "../ui/page";
import { Button } from "../ui/surface";

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
    <PageSection heading={t("sync.bucketHeading")} hint={t("sync.sealed")}>
      <div className="flex max-w-sm flex-col gap-4">
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
    </PageSection>
  );
}
