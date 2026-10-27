import { useCallback, useEffect, useId, useRef, useState } from "react";
import { failureCode } from "../api/client";
import type { CredentialKind, CredentialList, CredentialsApi } from "../api/credentials";
import { useTranslate } from "../i18n/context";
import { Field, control, hintText } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";

type CredentialEditPanelProps = {
  kind: CredentialKind;
  name: string;
  api: Pick<CredentialsApi, "revealCredential" | "updateCredential">;
  onSaved: (list: CredentialList) => void;
  onClose: () => void;
};

// CredentialEditPanel opens under the credential's row: the saved value is read from the engine, shown, and
// written back with the name, so a rename keeps the assignments. It is not a dialog, so the page stays in
// reach while it is open.
export function CredentialEditPanel({ kind, name, api, onSaved, onClose }: CredentialEditPanelProps) {
  const t = useTranslate();
  const [nextName, setNextName] = useState(name);
  const [secret, setSecret] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const nameInput = useRef<HTMLInputElement>(null);
  const headingId = useId();

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

  // The name field appears once the saved value is read; the panel's first field then takes the focus.
  useEffect(() => {
    if (!loading) nameInput.current?.focus();
  }, [loading]);

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

  const heading = kind === "password"
    ? t("secrets.editPassword")
    : kind === "totp"
      ? t("secrets.editTOTP")
      : t("secrets.editPassphrase");
  const valueLabel = kind === "password"
    ? t("secrets.passwordValue")
    : kind === "totp"
      ? t("secrets.totpValue")
      : t("secrets.passphraseValue");

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3 rounded-lg bg-well p-3">
      <div>
        <h4 id={headingId} className="text-sm font-semibold text-ink">{heading}</h4>
        <p className={`mt-1 ${hintText}`}>{t("secrets.editNote")}</p>
      </div>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {loading ? (
        <p aria-live="polite" className={hintText}>{t("secrets.revealing")}</p>
      ) : (
        <div className="grid gap-3 @2xl:grid-cols-2">
          <Field label={t("secrets.credentialName")}>
            <input ref={nameInput} value={nextName} onChange={(event) => setNextName(event.target.value)} className={control} />
          </Field>
          <PasswordField
            label={valueLabel}
            value={secret}
            onChange={setSecret}
            initialShown={kind === "key_passphrase"}
            disabled={saving}
          />
        </div>
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
    </section>
  );
}
