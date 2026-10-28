import type { PasswordVaultStatus } from "../api/vault";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { Button } from "../ui/surface";

// ConnectionVaultUnlock asks for the master password before a connection that keeps a password in the vault
// is saved: it unlocks the vault, or creates it, asking for the password twice only then. Creating a
// connection and editing one show the same fields.
export function ConnectionVaultUnlock({
  vault,
  masterPassword,
  onMasterPasswordChange,
  masterConfirmation,
  onMasterConfirmationChange,
  canOpen,
  busy,
  onOpen,
}: {
  vault: PasswordVaultStatus;
  masterPassword: string;
  onMasterPasswordChange: (value: string) => void;
  masterConfirmation: string;
  onMasterConfirmationChange: (value: string) => void;
  canOpen: boolean;
  busy: boolean;
  onOpen: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="flex flex-col gap-2.5 rounded-[9px] bg-notice p-3">
      <p className="text-xs leading-[18px] text-notice-ink">
        {t(vault.exists ? "conn.vaultLockedBeforeSave" : "conn.vaultMissingBeforeSave")}
      </p>
      <PasswordField label={t("conn.createMasterPassword")} value={masterPassword} onChange={onMasterPasswordChange} />
      {vault.exists ? null : (
        <PasswordField label={t("conn.createConfirmMaster")} value={masterConfirmation} onChange={onMasterConfirmationChange} />
      )}
      <Button kind="primary" className="self-start" disabled={!canOpen} busy={busy} onClick={onOpen}>
        {t(vault.exists ? "conn.createUnlockVault" : "conn.createInitialiseVault")}
      </Button>
    </div>
  );
}
