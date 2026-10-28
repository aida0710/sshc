import { useCallback, useEffect, useId, useRef, useState } from "react";
import { failureCode } from "../api/client";
import type { CredentialKind, CredentialList, CredentialsApi } from "../api/credentials";
import { useTranslate } from "../i18n/context";
import { Field, control, hintText } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { useLoad } from "../ui/useLoad";

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
  const headingId = useId();
  const reveal = useCallback(() => api.revealCredential(kind, name), [api, kind, name]);
  const revealed = useLoad(reveal);

  const heading = kind === "password"
    ? t("secrets.editPassword")
    : kind === "totp"
      ? t("secrets.editTOTP")
      : t("secrets.editPassphrase");

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3 rounded-lg bg-well p-3">
      <div>
        <h4 id={headingId} className="text-sm font-semibold text-ink">{heading}</h4>
        <p className={`mt-1 ${hintText}`}>{t("secrets.editNote")}</p>
      </div>
      {revealed.state === "loading" ? (
        <>
          <p aria-live="polite" className={hintText}>{t("secrets.revealing")}</p>
          <CredentialEditActions canSave={false} saving={false} onCancel={onClose} onSave={() => undefined} />
        </>
      ) : (
        // A value that could not be read can still be replaced: the form starts empty, under the failure.
        <CredentialEditForm
          kind={kind}
          name={name}
          initialName={revealed.state === "loaded" ? revealed.value.name : name}
          initialSecret={revealed.state === "loaded" ? revealed.value.secret : ""}
          revealFailure={revealed.state === "failed" ? failureCode(revealed.error) || t("secrets.revealFailed") : ""}
          api={api}
          onSaved={onSaved}
          onClose={onClose}
        />
      )}
    </section>
  );
}

function CredentialEditForm({ kind, name, initialName, initialSecret, revealFailure, api, onSaved, onClose }: {
  kind: CredentialKind;
  name: string;
  initialName: string;
  initialSecret: string;
  revealFailure: string;
  api: Pick<CredentialsApi, "updateCredential">;
  onSaved: (list: CredentialList) => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  const [nextName, setNextName] = useState(initialName);
  const [secret, setSecret] = useState(initialSecret);
  const save = useAsyncOperation();
  const nameInput = useRef<HTMLInputElement>(null);

  // The name field appears once the saved value is read; the panel's first field then takes the focus.
  useEffect(() => {
    nameInput.current?.focus();
  }, []);

  const close = useCallback(() => {
    setSecret("");
    onClose();
  }, [onClose]);

  async function submit() {
    const saved = await save.run(() => api.updateCredential(kind, name, nextName, secret), {
      apply: (list) => {
        setSecret("");
        onSaved(list);
      },
      describe: (caught) => {
        const code = failureCode(caught);
        return code === "credential_already_exists" ? t("secrets.nameExists") : code || t("secrets.updateFailed");
      },
    });
    if (saved) onClose();
  }

  const valueLabel = kind === "password"
    ? t("secrets.passwordValue")
    : kind === "totp"
      ? t("secrets.totpValue")
      : t("secrets.passphraseValue");
  const error = save.error || revealFailure;

  return (
    <>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      <div className="grid gap-3 @2xl:grid-cols-2">
        <Field label={t("secrets.credentialName")}>
          <input ref={nameInput} value={nextName} onChange={(event) => setNextName(event.target.value)} className={control} />
        </Field>
        <PasswordField
          label={valueLabel}
          value={secret}
          onChange={setSecret}
          initialShown={kind === "key_passphrase"}
          disabled={save.busy}
        />
      </div>
      <CredentialEditActions
        canSave={nextName !== "" && secret !== ""}
        saving={save.busy}
        onCancel={close}
        onSave={() => void submit()}
      />
    </>
  );
}

function CredentialEditActions({ canSave, saving, onCancel, onSave }: {
  canSave: boolean;
  saving: boolean;
  onCancel: () => void;
  onSave: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="flex justify-end gap-2">
      <Button onClick={onCancel}>{t("secrets.cancel")}</Button>
      <Button kind="primary" disabled={!canSave || saving} onClick={onSave}>
        {saving ? t("secrets.saving") : t("secrets.saveChanges")}
      </Button>
    </div>
  );
}
