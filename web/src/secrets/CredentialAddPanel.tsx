import { useEffect, useId, useRef, useState } from "react";
import type { CredentialKind } from "../api/credentials";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Field, control } from "../ui/form";
import { InlinePanel } from "../ui/InlinePanel";
import { phoneCardFrame } from "../ui/page";
import { PasswordField } from "../ui/PasswordField";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { Button } from "../ui/surface";

const headingKeys: Record<CredentialKind, MessageKey> = {
  password: "secrets.addPassword",
  key_passphrase: "secrets.addPassphrase",
  totp: "secrets.addTOTP",
};

const valueLabelKeys: Record<CredentialKind, MessageKey> = {
  password: "secrets.passwordValue",
  key_passphrase: "secrets.passphraseValue",
  totp: "secrets.newTOTPValue",
};

// CredentialAddPanel is what the page's ［＋ 追加］ opens above the list: a name and the value. Saving
// resolves true once the credential is stored, and the panel is closed by the page then.
export function CredentialAddPanel({ kind, onStore, onCancel }: {
  kind: CredentialKind;
  onStore: (name: string, secret: string) => Promise<boolean>;
  onCancel: () => void;
}) {
  const t = useTranslate();
  const headingId = useId();
  const nameInput = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [secret, setSecret] = useState("");
  const [saving, setSaving] = useState(false);
  const compact = useMediaQuery(mobileViewportQuery);

  useEffect(() => {
    nameInput.current?.focus();
  }, []);

  async function store() {
    setSaving(true);
    const stored = await onStore(name, secret);
    if (!stored) setSaving(false);
  }

  return (
    <InlinePanel className="mb-3">
      <section aria-labelledby={headingId} className={`flex flex-col gap-3 ${compact ? `${phoneCardFrame} px-3.5 py-3` : "rounded-lg bg-well p-3"}`}>
        <h3 id={headingId} className="text-sm font-semibold text-ink">{t(headingKeys[kind])}</h3>
        <div className="grid gap-3 @2xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] @2xl:items-end">
          <Field label={t("secrets.credentialName")}>
            <input ref={nameInput} value={name} onChange={(event) => setName(event.target.value)} disabled={saving} className={control} />
          </Field>
          <PasswordField label={t(valueLabelKeys[kind])} value={secret} onChange={setSecret} disabled={saving} />
          <div className="flex justify-end gap-2">
            <Button onClick={onCancel}>{t("secrets.cancel")}</Button>
            <Button kind="primary" disabled={saving || name === "" || secret === ""} onClick={() => void store()}>
              {saving ? t("secrets.saving") : t("secrets.save")}
            </Button>
          </div>
        </div>
      </section>
    </InlinePanel>
  );
}
