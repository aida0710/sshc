import type { PasswordVaultStatus } from "../api/vault";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { Button } from "../ui/surface";

// CreateConnectionVaultPrompt は、パスワードで認証する接続を作る前に、Vault の
// ロックを解除する（無ければ作る）欄である。作るときだけ確認の入力を求める。
export function CreateConnectionVaultPrompt({
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
    <div className="flex flex-col gap-3 rounded-lg border border-notice-line bg-notice p-3">
      <p className="text-sm text-notice-ink">{t(vault.exists ? "conn.createVaultLocked" : "conn.createVaultMissing")}</p>
      <PasswordField label={t("conn.createMasterPassword")} value={masterPassword} onChange={onMasterPasswordChange} />
      {vault.exists ? null : (
        <PasswordField
          label={t("conn.createConfirmMaster")}
          value={masterConfirmation}
          onChange={onMasterConfirmationChange}
        />
      )}
      <Button kind="primary" disabled={busy || !canOpen} onClick={onOpen}>
        {t(vault.exists ? "conn.createUnlockVault" : "conn.createInitialiseVault")}
      </Button>
    </div>
  );
}
