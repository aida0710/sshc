import { useState } from "react";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";

type LockScreenProps = {
  exists: boolean;
  onOpen: () => void;
  api?: IntegrationsApi;
};

export function LockScreen({ exists, onOpen, api = integrationsApi }: LockScreenProps) {
  const t = useTranslate();
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const minimum = 12;
  const tooShort = password.length < minimum;
  const mismatched = !exists && confirmation !== password;

  async function submit() {
    setBusy(true);
    setError("");
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
      const code = failureCode(caught);
      setError(
        code === "wrong_passphrase"
          ? t("lock.wrong")
          : code === "passphrase_too_short"
            ? t("lock.tooShort", { count: minimum })
            : t("lock.failed"),
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-canvas p-6">
      <section className="sshc-card grid w-full max-w-3xl overflow-hidden rounded-2xl bg-card md:grid-cols-[minmax(0,0.9fr)_minmax(20rem,1.1fr)]">
        <div className="flex min-h-60 flex-col justify-center gap-8 bg-toolbar p-7 md:p-10">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-ink">{t("shell.title")}</h1>
            <p className="mt-2 text-sm leading-6 text-ink-muted">{exists ? t("lock.explainOpen") : t("lock.explainNew")}</p>
          </div>
        </div>

        <form
          className="flex flex-col justify-center gap-4 p-7 md:p-10"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          {exists ? null : <p className="rounded-lg bg-notice px-3 py-2 text-sm text-notice-ink">{t("lock.noRecovery")}</p>}
          {error === "" ? null : <Notice tone="danger">{error}</Notice>}
          <PasswordField label={t("lock.password")} value={password} onChange={setPassword} autoFocus />
          {exists ? null : (
            <PasswordField label={t("lock.confirm")} value={confirmation} onChange={setConfirmation} />
          )}
          <Button kind="primary" className="self-start" type="submit" disabled={busy || tooShort || mismatched}>
            {exists ? t("lock.open") : t("lock.create")}
          </Button>
        </form>
      </section>
    </main>
  );
}
