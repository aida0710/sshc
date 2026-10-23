import { useEffect, useId, useState, type FormEvent, type RefObject } from "react";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { useTranslate } from "../i18n/context";
import { runTargetProblemText } from "../terminal/runTargetProblem";
import { ModalShell } from "../ui/ModalShell";
import { PasswordField } from "../ui/PasswordField";
import { Button } from "../ui/surface";

// RemoteVaultUnlockDialog asks for the Vault password of a run-target engine
// and unlocks it. The password goes to that engine through this machine's
// engine; neither keeps it here. An engine with no Vault yet is refused the
// same way as a locked one; that Vault is created in the engine's own screen.
export function RemoteVaultUnlockDialog({
  alias,
  api,
  onUnlocked,
  onOpenInNewTab,
  onClose,
  returnFocusRef,
}: {
  alias: string;
  api: RemoteEnginesApi;
  onUnlocked: () => void;
  onOpenInNewTab: () => void;
  onClose: () => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const headingId = useId();
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  // vaultMissing is null until the engine says whether it has a Vault.
  const [vaultMissing, setVaultMissing] = useState<boolean | null>(null);

  useEffect(() => {
    let active = true;
    void api.status(alias)
      .then((status) => {
        if (!active) return;
        // Unlocked elsewhere in the meantime: there is nothing to ask.
        if (status.vault && status.unlocked) onUnlocked();
        else setVaultMissing(!status.vault);
      })
      // The password is still asked for; unlocking says what is wrong.
      .catch(() => { if (active) setVaultMissing(false); });
    return () => { active = false; };
  }, [alias, api, onUnlocked]);

  async function unlock(event: FormEvent) {
    event.preventDefault();
    if (busy || password === "") return;
    setBusy(true);
    setError("");
    try {
      await api.unlockVault(alias, password);
      setPassword("");
      onUnlocked();
    } catch (caught) {
      setError(runTargetProblemText(t, caught, alias));
    } finally {
      setBusy(false);
    }
  }

  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onClose}
      dismissible={!busy}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      panelClassName="w-full max-w-sm rounded-lg p-4"
    >
      {vaultMissing === true ? (
        <div className="flex flex-col gap-3">
          <h2 id={headingId} className="text-sm font-medium text-ink">{t("runTarget.vaultMissingHeading", { alias })}</h2>
          <p className="text-sm text-ink-muted">{t("runTarget.vaultMissingBody", { alias })}</p>
          <div className="flex justify-end gap-2">
            <Button onClick={onClose}>{t("runTarget.unlockCancel")}</Button>
            <Button kind="primary" onClick={onOpenInNewTab}>{t("runTarget.openInNewTab")}</Button>
          </div>
        </div>
      ) : (
        <form onSubmit={(event) => void unlock(event)} className="flex flex-col gap-3">
          <h2 id={headingId} className="text-sm font-medium text-ink">{t("runTarget.unlockHeading", { alias })}</h2>
          <p className="text-sm text-ink-muted">{t("runTarget.unlockBody", { alias })}</p>
          <PasswordField label={t("runTarget.unlockPassword", { alias })} value={password} onChange={setPassword} error={error} disabled={busy} autoFocus />
          <div className="flex justify-end gap-2">
            <Button disabled={busy} onClick={onClose}>{t("runTarget.unlockCancel")}</Button>
            <Button type="submit" kind="primary" disabled={busy || password === ""}>
              {busy ? t("runTarget.unlocking") : t("runTarget.unlock")}
            </Button>
          </div>
        </form>
      )}
    </ModalShell>
  );
}
