import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import type { PasswordVaultStatus, VaultApi } from "../api/vault";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { CheckboxField } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { ActionArea, SettingsSection } from "./SettingsSection";
import { useAsyncOperation } from "../ui/useAsyncOperation";

export type MasterPasswordApi = Pick<VaultApi, "passwordVault" | "changeMasterPassword">;

type MasterDraft = { current: string; next: string; confirm: string; withoutPassword: boolean };
const emptyDraft: MasterDraft = { current: "", next: "", confirm: "", withoutPassword: false };

// Changing (or removing) the master password that seals the vault.
export function MasterPasswordSection({ api, showHeading, onVaultChanged }: {
  api: MasterPasswordApi;
  showHeading: boolean;
  onVaultChanged?: ((status: PasswordVaultStatus) => void) | undefined;
}) {
  const t = useTranslate();
  // null until the vault has said whether it has a password at all.
  const [passwordless, setPasswordless] = useState<boolean | null>(null);
  const [draft, setDraft] = useState<MasterDraft>(emptyDraft);
  const save = useAsyncOperation();

  // Only the stable reporter is a dependency: the operation's facts change on
  // every save and must not re-read the vault state mid-flow.
  const reportFailure = save.fail;
  useEffect(() => {
    let active = true;
    setPasswordless(null);
    void api.passwordVault().then((status) => {
      if (active) setPasswordless(status.passwordless ?? false);
    }).catch(() => { if (active) reportFailure(t("secrets.failed")); });
    return () => { active = false; };
  }, [api, t, reportFailure]);

  function edit(patch: Partial<MasterDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
  }

  async function submit() {
    const { current, next, withoutPassword } = draft;
    await save.run(async () => {
      const result = await api.changeMasterPassword(passwordless ? "" : current, withoutPassword ? "" : next);
      setPasswordless(result.vault.passwordless ?? false);
      onVaultChanged?.(result.vault);
    }, { describe: (error) => failureCode(error) === "wrong_passphrase" ? t("secrets.wrongCurrent") : t("secrets.changeFailed") });
    setDraft((state) => ({ ...emptyDraft, withoutPassword: state.withoutPassword }));
  }

  const strong = [...draft.next].length >= 4 && draft.next === draft.confirm;
  const canChange = passwordless !== null && !save.busy && (draft.withoutPassword || strong);
  return (
    <SettingsSection id="settings-password" label={t("secrets.changeHeading")} icon="secrets" showHeading={showHeading}>
      <div className="max-w-2xl">
        <p className="mb-5 text-sm leading-6 text-ink-muted">{t("secrets.changeNote")}</p>
        <CheckboxField label={t("lock.withoutPassword")} checked={draft.withoutPassword} onChange={(checked) => edit({ withoutPassword: checked })} />
        {draft.withoutPassword ? <p className="mb-3 text-sm text-ink-muted">{t("lock.withoutPasswordHint")}</p> : null}
        <div className="grid gap-4 sm:grid-cols-2">
          {passwordless === false ? <div className="sm:col-span-2">
            <PasswordField
              label={t("secrets.currentMaster")}
              value={draft.current}
              onChange={(value) => edit({ current: value })}
              disabled={save.busy}
            />
          </div> : null}
          {draft.withoutPassword ? null : <>
          <PasswordField
            label={t("secrets.newMaster")}
            value={draft.next}
            onChange={(value) => edit({ next: value })}
            disabled={save.busy}
          />
          <PasswordField
            label={t("secrets.confirmMaster")}
            value={draft.confirm}
            onChange={(value) => edit({ confirm: value })}
            disabled={save.busy}
          />
          </>}
        </div>
        {!draft.withoutPassword && [...draft.next].length >= 4 && [...draft.next].length < 12 ? <p className="mt-3 text-sm text-ink-muted">{t("lock.shortPasswordHint")}</p> : null}
        <ActionArea status={save.error === ""
          ? (save.saved ? <p role="status" className="text-sm text-live">{t("secrets.changedMasterLocally")}</p> : undefined)
          : <Notice tone="danger">{save.error}</Notice>}
        >
          <Button kind="primary" disabled={!canChange} onClick={() => void submit()}>
            {t("secrets.change")}
          </Button>
        </ActionArea>
      </div>
    </SettingsSection>
  );
}
