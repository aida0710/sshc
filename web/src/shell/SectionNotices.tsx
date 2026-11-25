import type { ReactNode } from "react";
import type { CreateConnectionDraft } from "../connections/createConnectionDraft";
import { useTranslate } from "../i18n/context";
import type { Section } from "../routing/sectionRoute";
import { Banner, bannerButton } from "../ui/Banner";
import { Button } from "../ui/surface";

// The sections a connection being created sends the person to for what it
// still needs (a key); there the draft waits with a way back.
const connectionDraftPrerequisiteSections: readonly Section[] = ["Keys"];

// The notices across the top of every section: the Vault file was moved to
// a newer version, and a connection being created waits for the person to
// come back to it.
export function SectionNotices({
  section,
  vaultMigration,
  onDismissVaultMigration,
  connectionDraft,
  onReturnToConnectionDraft,
}: {
  section: Section | null;
  vaultMigration: { from: number; to: number } | null;
  onDismissVaultMigration: () => void;
  connectionDraft: CreateConnectionDraft | null;
  onReturnToConnectionDraft: () => void;
}) {
  const t = useTranslate();
  const draftWaiting = connectionDraft !== null && section !== null && connectionDraftPrerequisiteSections.includes(section);
  return (
    <>
      {vaultMigration === null ? null : (
        <SectionNotice
          status
          actionLabel={t("lock.migrationDismiss")}
          onAction={onDismissVaultMigration}
        >
          {t("lock.migrationCompleted", { current: vaultMigration.from, required: vaultMigration.to })}
        </SectionNotice>
      )}
      {draftWaiting ? (
        <SectionNotice actionLabel={t("conn.createReturnToDraft")} onAction={onReturnToConnectionDraft}>
          <p className="truncate">
            {t("conn.createDraftWaiting", { alias: connectionDraft.alias || t("conn.createUntitledDraft") })}
          </p>
        </SectionNotice>
      ) : null}
    </>
  );
}

function SectionNotice({
  status = false,
  actionLabel,
  onAction,
  children,
}: {
  status?: boolean;
  actionLabel: string;
  onAction: () => void;
  children: ReactNode;
}) {
  return (
    <Banner
      tone="notice"
      role={status ? "status" : undefined}
      actions={<Button small className={bannerButton} onClick={onAction}>{actionLabel}</Button>}
    >
      {children}
    </Banner>
  );
}
