import { useCallback, useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import type {
  CredentialKind,
  CredentialList,
  IntegrationsApi,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { Field, control, hintText } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import { ModalShell } from "../ui/ModalShell";

type CredentialEditDialogProps = {
  kind: CredentialKind;
  name: string;
  api: Pick<IntegrationsApi, "revealCredential" | "updateCredential">;
  onSaved: (list: CredentialList) => void;
  onClose: () => void;
};

export function CredentialEditDialog({ kind, name, api, onSaved, onClose }: CredentialEditDialogProps) {
  const t = useTranslate();
  const [nextName, setNextName] = useState(name);
  const [secret, setSecret] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const nameInput = useRef<HTMLInputElement>(null);

  const close = useCallback(() => {
    setSecret("");
    onClose();
  }, [onClose]);

  useEffect(() => {
    let active = true;
    void api.revealCredential(kind, name).then(
      (credential) => {
        if (!active) return;
        setNextName(credential.name);
        setSecret(credential.secret);
        setLoading(false);
      },
      (caught) => {
        if (!active) return;
        setError(failureCode(caught) || t("secrets.revealFailed"));
        setLoading(false);
      },
    );
    return () => {
      active = false;
    };
  }, [api, kind, name, t]);

  async function save() {
    setSaving(true);
    try {
      const list = await api.updateCredential(kind, name, nextName, secret);
      setSecret("");
      onSaved(list);
      onClose();
    } catch (caught) {
      const code = failureCode(caught);
      setError(code === "credential_already_exists" ? t("secrets.nameExists") : code || t("secrets.updateFailed"));
      setSaving(false);
    }
  }

  const heading = kind === "password" ? t("secrets.editPassword") : t("secrets.editPassphrase");
  const valueLabel = kind === "password" ? t("secrets.passwordValue") : t("secrets.passphraseValue");

  return (
    <ModalShell
      labelledBy="credential-edit-heading"
      onDismiss={close}
      initialFocusRef={nameInput}
      panelClassName="flex w-full max-w-lg flex-col gap-4 rounded-lg p-5"
    >
        <div>
          <h2 id="credential-edit-heading" className="text-base font-semibold text-ink">{heading}</h2>
          <p className={`mt-1 ${hintText}`}>{t("secrets.editNote")}</p>
        </div>
        {error === "" ? null : <Notice tone="danger">{error}</Notice>}
        {loading ? (
          <p aria-live="polite" className={hintText}>{t("secrets.revealing")}</p>
        ) : (
          <>
            <Field label={t("secrets.credentialName")}>
              <input ref={nameInput} value={nextName} onChange={(event) => setNextName(event.target.value)} className={control} />
            </Field>
            <PasswordField
              label={valueLabel}
              value={secret}
              onChange={setSecret}
              initialShown={kind !== "password"}
              disabled={saving}
            />
          </>
        )}
        <div className="flex justify-end gap-2">
          <Button onClick={close}>{t("secrets.cancel")}</Button>
          <Button
            kind="primary"
            disabled={loading || saving || nextName === "" || secret === ""}
            onClick={() => void save()}
          >
            {saving ? t("secrets.saving") : t("secrets.saveChanges")}
          </Button>
        </div>
    </ModalShell>
  );
}
