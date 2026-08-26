import { useState } from "react";
import { ApiError, type RequestFailureDiagnostic } from "../api/client";
import { integrationsApi, type IntegrationsApi, type PasswordVaultStatus } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import { ErrorDiagnosticNotice } from "../shell/ErrorDiagnosticNotice";

type LockScreenProps = {
  exists: boolean;
  onOpen: (status?: PasswordVaultStatus) => void;
  onExists?: () => void;
  version?: string;
  api?: IntegrationsApi;
};

export function LockScreen({
  exists,
  onOpen,
  onExists = () => undefined,
  version = "",
  api = integrationsApi,
}: LockScreenProps) {
  const t = useTranslate();
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
  const minimum = 12;
  const tooShort = password.length < minimum;
  const mismatched = !exists && confirmation !== password;

  async function submit() {
    setBusy(true);
    setError("");
    setDiagnostic(null);
    setVersionMismatch(null);
    setResetAcknowledged(false);
    try {
      const status = exists
        ? await api.unlockVault(password)
        : await api.initialiseVault(password);
      setPassword("");
      setConfirmation("");
      onOpen(status);
    } catch (caught) {
      const code = caught instanceof ApiError ? caught.code : "network_request_failed";
      const method = "POST";
      const path = exists ? "/api/v1/passwords/unlock" : "/api/v1/passwords/initialise";
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
          setError(t("lock.tooShort", { count: minimum }));
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
        case "vault_storage_busy":
          setError(t("lock.storageBusy"));
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
          setError(t("lock.failed"));
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
      setError(t(code === "vault_compatible_backup_missing" ? "lock.noCompatibleBackup" : "lock.recoveryFailed"));
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
    } catch {
      setError(t("lock.resetFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-canvas p-6">
      <section className="sshc-card w-full max-w-3xl overflow-hidden rounded-2xl bg-card">
        {diagnostic === null ? null : (
          <ErrorDiagnosticNotice diagnostic={diagnostic} version={version} onClose={() => setDiagnostic(null)} />
        )}
        <div className="grid md:grid-cols-[minmax(0,0.9fr)_minmax(20rem,1.1fr)]">
          <div className="flex min-h-60 flex-col justify-center gap-8 bg-toolbar p-7 md:p-10">
            <div>
              <h1 className="text-2xl font-semibold tracking-tight text-ink">{t("shell.title")}</h1>
              <p className="mt-2 text-sm leading-6 text-ink-muted">
                {exists ? t("lock.explainOpen") : t("lock.explainNew")}
              </p>
            </div>
          </div>

          <form
            className="flex flex-col justify-center gap-4 p-7 md:p-10"
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            {exists ? null : (
              <p className="rounded-lg bg-notice px-3 py-2 text-sm text-notice-ink">{t("lock.noRecovery")}</p>
            )}
            {error === "" ? null : <Notice tone="danger">{error}</Notice>}
            {versionMismatch === null ? null : (
              <div className="flex flex-col gap-3 rounded-xl border border-control-line bg-toolbar p-4">
                <p className="text-sm leading-6 text-ink-muted">{t("lock.schemaRecoveryHint")}</p>
                <Button type="button" disabled={busy || tooShort} onClick={() => void recoverCompatibleBackup()}>
                  {t("lock.restoreCompatibleBackup")}
                </Button>
                <label className="flex items-start gap-2 text-sm leading-5 text-ink-muted">
                  <input
                    className="mt-1"
                    type="checkbox"
                    checked={resetAcknowledged}
                    onChange={(event) => setResetAcknowledged(event.target.checked)}
                  />
                  <span>{t("lock.resetUnsupportedAcknowledge")}</span>
                </label>
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
            <PasswordField label={t("lock.password")} value={password} onChange={setPassword} autoFocus />
            {exists ? null : (
              <PasswordField label={t("lock.confirm")} value={confirmation} onChange={setConfirmation} />
            )}
            <Button kind="primary" className="self-start" type="submit" disabled={busy || tooShort || mismatched}>
              {exists ? t("lock.open") : t("lock.create")}
            </Button>
          </form>
        </div>
      </section>
    </main>
  );
}
