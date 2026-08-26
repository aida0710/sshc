import { useState } from "react";
import { ApiError, type RequestFailureDiagnostic } from "../api/client";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import { ErrorDiagnosticNotice } from "../shell/ErrorDiagnosticNotice";

type LockScreenProps = {
  exists: boolean;
  onOpen: () => void;
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
  const [busy, setBusy] = useState(false);
  const minimum = 12;
  const tooShort = password.length < minimum;
  const mismatched = !exists && confirmation !== password;

  async function submit() {
    setBusy(true);
    setError("");
    setDiagnostic(null);
    try {
      if (exists) {
        await api.unlockVault(password);
      } else {
        await api.initialiseVault(password);
      }
      setPassword("");
      setConfirmation("");
      onOpen();
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
        default:
          setError(t("lock.failed"));
      }
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
