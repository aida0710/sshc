import { useId, useState } from "react";
import { ApiError, type RequestFailureDiagnostic } from "../api/client";
import { vaultApi, type PasswordVaultStatus, type VaultApi } from "../api/vault";
import { useLanguage } from "../i18n/context";
import { CheckboxField, control } from "../ui/form";
import { EntryPreferences } from "../ui/EntryPreferences";
import { EntryScreen } from "../ui/EntryScreen";
import { Icon } from "../ui/icons";
import { PasswordField } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import { ErrorDiagnosticNotice } from "../shell/ErrorDiagnosticNotice";
import { SignOutButton } from "../session/SignOutButton";
import { isAcceptedButShort, meetsMasterPasswordMinimum } from "./masterPasswordLength";

// The lock screen's fields and button are taller than a page's, as in the design.
const largeField = `${control} h-[38px] text-sm`;

type LockScreenProps = {
  exists: boolean;
  passwordless?: boolean;
  // PasswordVaultStatus.minPassphraseLength, needed only to create a vault.
  minPassphraseLength?: number | undefined;
  onOpen: (status?: PasswordVaultStatus) => void;
  onExists?: () => void;
  version?: string;
  // offerSignOut shows sign-out for the whole-app lock screen, so a browser can
  // leave without the master password. The Passwords panel has the menu instead.
  offerSignOut?: boolean;
  api?: Pick<VaultApi, "passwordVault" | "initialiseVault" | "unlockVault" | "recoverCompatibleVault" | "resetUnsupportedVault">;
};

export function LockScreen({
  exists,
  passwordless = false,
  minPassphraseLength,
  onOpen,
  onExists = () => undefined,
  version = "",
  offerSignOut = false,
  api = vaultApi,
}: LockScreenProps) {
  const { t } = useLanguage();
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [diagnostic, setDiagnostic] = useState<RequestFailureDiagnostic | null>(null);
  const [versionMismatch, setVersionMismatch] = useState<{
    kind: "older" | "newer";
    current: number;
    required: number;
  } | null>(null);
  const [resetAcknowledged, setResetAcknowledged] = useState(false);
  const [busy, setBusy] = useState(false);
  const [withoutPassword, setWithoutPassword] = useState(false);
  const noPassword = exists ? passwordless : withoutPassword;
  const tooShort = !noPassword && (exists ? password === "" : !meetsMasterPasswordMinimum(password, minPassphraseLength));
  const mismatched = !exists && !noPassword && confirmation !== password;

  async function submit() {
    if (busy || tooShort || mismatched) return;
    setBusy(true);
    setError("");
    setDiagnostic(null);
    setVersionMismatch(null);
    setResetAcknowledged(false);
    try {
      const status = exists
        ? await api.unlockVault(noPassword ? "" : password)
        : await api.initialiseVault(noPassword ? "" : password);
      setPassword("");
      setConfirmation("");
      onOpen(status);
    } catch (caught) {
      const code = caught instanceof ApiError ? caught.code : "network_request_failed";
      const method = "POST";
      const path = exists ? "/api/v1/passwords/unlock" : "/api/v1/passwords/initialise";
      const failureMessage = t(exists ? "lock.unlockFailed" : "lock.createFailed");
      setDiagnostic({
        code,
        status: caught instanceof ApiError ? caught.status : 0,
        method,
        path,
        ...(caught instanceof ApiError && typeof caught.problem?.detail === "string"
          ? { detail: caught.problem.detail }
          : {}),
      });
      if (code === "vault_already_exists") {
        try {
          const status = await api.passwordVault();
          if (status.exists) {
            setPassword("");
            setConfirmation("");
            onExists();
            setError(t("lock.alreadyExists"));
            return;
          }
        } catch {
          // The original failure remains the most useful safe diagnostic.
        }
      }
      switch (code) {
        case "wrong_passphrase":
          setError(t("lock.wrong"));
          break;
        case "passphrase_too_short":
          setError(minPassphraseLength === undefined ? failureMessage : t("lock.tooShort", { count: minPassphraseLength }));
          break;
        case "vault_storage_permission_denied":
          setError(t("lock.storagePermission"));
          break;
        case "vault_storage_full":
          setError(t("lock.storageFull"));
          break;
        case "vault_storage_read_only":
          setError(t("lock.storageReadOnly"));
          break;
        case "workspace_busy":
          setError(t("lock.storageBusy"));
          break;
        case "workspace_pending_transaction":
          setError(t("lock.pendingTransaction"));
          break;
        case "vault_storage_io_failed":
          setError(t("lock.storageIO"));
          break;
        case "vault_schema_older":
        case "vault_schema_newer": {
          const current = caught instanceof ApiError && typeof caught.problem?.currentVersion === "number"
            ? caught.problem.currentVersion
            : 0;
          const required = caught instanceof ApiError && typeof caught.problem?.requiredVersion === "number"
            ? caught.problem.requiredVersion
            : 0;
          const kind = code === "vault_schema_older" ? "older" : "newer";
          setVersionMismatch({ kind, current, required });
          setError(t(kind === "older" ? "lock.schemaOlder" : "lock.schemaNewer", { current, required }));
          break;
        }
        case "vault_migration_failed": {
          const current = caught instanceof ApiError && typeof caught.problem?.currentVersion === "number"
            ? caught.problem.currentVersion
            : 0;
          const required = caught instanceof ApiError && typeof caught.problem?.requiredVersion === "number"
            ? caught.problem.requiredVersion
            : 0;
          setError(t("lock.migrationFailed", { current, required }));
          break;
        }
        case "vault_envelope_unsupported":
          setError(t("lock.envelopeUnsupported"));
          break;
        default:
          setError(failureMessage);
      }
    } finally {
      setBusy(false);
    }
  }

  async function recoverCompatibleBackup() {
    setBusy(true);
    setError("");
    try {
      const status = await api.recoverCompatibleVault(password);
      setPassword("");
      setVersionMismatch(null);
      onOpen(status);
    } catch (caught) {
      const code = caught instanceof ApiError ? caught.code : "network_request_failed";
      switch (code) {
        case "vault_compatible_backup_missing":
          setError(t("lock.noCompatibleBackup"));
          break;
        case "vault_backups_too_many":
          setError(t("lock.tooManyBackups"));
          break;
        default:
          setError(t("lock.recoveryFailed"));
      }
    } finally {
      setBusy(false);
    }
  }

  async function resetUnsupportedVault() {
    if (!resetAcknowledged) return;
    setBusy(true);
    setError("");
    try {
      const status = await api.resetUnsupportedVault(password);
      setPassword("");
      setVersionMismatch(null);
      setResetAcknowledged(false);
      onOpen(status);
    } catch (caught) {
      const code = caught instanceof ApiError ? caught.code : "network_request_failed";
      setError(t(code === "vault_backups_too_many" ? "lock.tooManyBackups" : "lock.resetFailed"));
    } finally {
      setBusy(false);
    }
  }

  const noRecovery = exists || noPassword ? undefined : (
    <p className="flex gap-2.5 rounded-[9px] bg-notice px-3 py-2.5 text-[12.5px] leading-[19px] text-notice-ink">
      <Icon name="warning" className="mt-0.5 size-[15px]" />
      {t("lock.noRecovery")}
    </p>
  );
  return (
    <EntryScreen
      heading={t("shell.title")}
      description={exists ? t(noPassword ? "lock.explainPasswordless" : "lock.explainOpen") : t("lock.explainNew")}
      asideNote={noRecovery}
      corner={<EntryPreferences />}
      footer={offerSignOut ? <SignOutButton /> : undefined}
    >
      <form
        className="flex flex-col gap-3.5"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        {diagnostic === null ? null : (
          <ErrorDiagnosticNotice diagnostic={diagnostic} version={version} onClose={() => setDiagnostic(null)} />
        )}
        {error === "" ? null : <Notice tone="danger">{error}</Notice>}
        {versionMismatch === null ? null : (
          <div className="flex flex-col gap-3 rounded-[10px] bg-well p-4">
            <p className="text-sm leading-6 text-ink-muted">{t("lock.schemaRecoveryHint")}</p>
            <Button type="button" disabled={busy || tooShort} onClick={() => void recoverCompatibleBackup()}>
              {t("lock.restoreCompatibleBackup")}
            </Button>
            <CheckboxField label={t("lock.resetUnsupportedAcknowledge")} checked={resetAcknowledged} onChange={setResetAcknowledged} tone="danger" />
            <Button
              kind="danger"
              type="button"
              disabled={busy || tooShort || !resetAcknowledged}
              onClick={() => void resetUnsupportedVault()}
            >
              {t("lock.resetUnsupported")}
            </Button>
          </div>
        )}
        {exists ? null : <PasswordlessSwitch checked={withoutPassword} onChange={setWithoutPassword} />}
        {noPassword ? (exists ? <p className="text-sm text-ink-muted">{t("lock.withoutPasswordHint")}</p> : null) : <>
          <PasswordField label={t("lock.password")} value={password} onChange={setPassword} autoFocus inputClassName={largeField} />
          {exists ? null : (
            <PasswordField label={t("lock.confirm")} value={confirmation} onChange={setConfirmation} inputClassName={largeField} />
          )}
        </>}
        {!exists && !noPassword && isAcceptedButShort(password, minPassphraseLength) ? <p className="text-sm text-ink-muted">{t("lock.shortPasswordHint")}</p> : null}
        <Button kind="primary" className="h-[38px] w-full text-sm" type="submit" disabled={busy || tooShort || mismatched}>
          {exists ? t("lock.open") : t("lock.create")}
        </Button>
      </form>
    </EntryScreen>
  );
}

// PasswordlessSwitch is the choice to use the vault without a master password, drawn as a switch in a
// tinted row as in the design. The checkbox stays a checkbox for assistive technology. Its explanation
// shows once it is on, where the password fields were.
function PasswordlessSwitch({ checked, onChange }: { checked: boolean; onChange: (checked: boolean) => void }) {
  const { t } = useLanguage();
  const hintId = useId();
  return (
    <div className="rounded-[10px] bg-well px-3 py-2.5">
      <label className="flex cursor-pointer items-center gap-2.5 text-[13px] text-ink">
        <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)}
          aria-describedby={checked ? hintId : undefined} className="peer sr-only" />
        <span aria-hidden="true" className="relative h-[18px] w-[30px] shrink-0 rounded-full bg-ink/20 transition-colors after:absolute after:left-0.5 after:top-0.5 after:size-3.5 after:rounded-full after:bg-sidebar after:transition-transform peer-checked:bg-accent peer-checked:after:translate-x-3 peer-focus-visible:ring-[3px] peer-focus-visible:ring-accent/30" />
        {t("lock.withoutPassword")}
      </label>
      {checked ? <p id={hintId} className="mt-1 pl-10 text-[11.5px] leading-[17px] text-ink-faint">{t("lock.withoutPasswordHint")}</p> : null}
    </div>
  );
}
