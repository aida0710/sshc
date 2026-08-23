import { useState } from "react";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
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
    <main className="mx-auto flex min-h-screen max-w-md flex-col justify-center gap-4 p-6">
      <h1 className="text-lg font-medium">{t("shell.title")}</h1>
      <p className={hintText}>{exists ? t("lock.explainOpen") : t("lock.explainNew")}</p>

      {exists ? null : <p className="text-sm text-notice-ink">{t("lock.noRecovery")}</p>}
      {error === "" ? null : (
        <Notice tone="danger">{error}</Notice>
      )}

      <form
        className="flex flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <PasswordField label={t("lock.password")} value={password} onChange={setPassword} autoFocus />
        {exists ? null : (
          <PasswordField label={t("lock.confirm")} value={confirmation} onChange={setConfirmation} />
        )}
        <div className="flex flex-wrap items-center gap-2">
          <Button
            kind="primary"
            type="submit"
            disabled={busy || tooShort || mismatched}
          >
            {exists ? t("lock.open") : t("lock.create")}
          </Button>
        </div>
      </form>
    </main>
  );
}
