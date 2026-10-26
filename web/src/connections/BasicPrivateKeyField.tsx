import type { KeyItem } from "../keys/api";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import { useTranslate } from "../i18n/context";
import { control, hintText } from "../ui/form";
import { FieldRow, FieldRowNote } from "../ui/FieldRow";
import { PasswordInput } from "../ui/PasswordField";
import { Notice } from "../ui/surface";
import { customKeyId, type BasicDraft, type BasicFormDerived, type KeySelection } from "./basicFormDraft";
import type { ConnectionSecretsModel } from "./useConnectionSecrets";

// Which private key the connection uses, and, for an encrypted key the vault
// can hold a passphrase for, the passphrase itself.
export function BasicPrivateKeyField({
  draft, derived, keySelection, secrets, preferredKey, serverKeyError, serverKeyPassphraseError,
  passphraseOpen, onPassphraseOpenChange, onSelectKey, onEdit,
}: {
  draft: BasicDraft;
  derived: BasicFormDerived;
  keySelection: KeySelection;
  secrets: Pick<ConnectionSecretsModel, "privateKeys" | "keyOptionsStatus" | "loading" | "unlocked">;
  preferredKey: GeneratedPrivateKeyHandoff | null;
  serverKeyError: string;
  serverKeyPassphraseError: string;
  passphraseOpen: boolean;
  onPassphraseOpenChange: (open: boolean) => void;
  onSelectKey: (keyId: string) => void;
  onEdit: (patch: Partial<Pick<BasicDraft, "keyPassphrase" | "keyPassphraseConfirmation">>) => void;
}) {
  const t = useTranslate();
  const { keyState, customKey } = keySelection;
  const { selectedPrivateKey, identityFileChange } = derived;
  return (
    <>
      <FieldRow
        label={t("conn.basicPrivateKey")}
        warning={serverKeyError || undefined}
        hint={keyState === "custom"
          ? t("conn.basicCustomKey", { path: customKey })
          : keyState === "complex"
            ? t("conn.basicComplexKey")
            : selectedPrivateKey !== undefined && !selectedPrivateKey.encrypted
              ? t("conn.basicKeyPassphraseUnencrypted")
              : undefined}
      >
        <select
          aria-label={t("conn.basicPrivateKey")}
          value={draft.selectedKey}
          disabled={secrets.loading || secrets.keyOptionsStatus !== "ready" || keyState === "custom" || keyState === "complex"}
          onChange={(event) => onSelectKey(event.target.value)}
          className={control}
        >
          <option value="">{t("conn.basicAgentOrInherited")}</option>
          {keyState === "custom" ? <option value={customKeyId}>{customKey}</option> : null}
          {secrets.privateKeys.map((key: KeyItem) => (
            <option key={key.id} value={key.id}>
              {key.relativePath}{key.fingerprint === "" ? "" : ` · ${key.fingerprint}`}
            </option>
          ))}
        </select>
      </FieldRow>
      {preferredKey !== null && identityFileChange?.action === "set" &&
      identityFileChange.keyId === preferredKey.privateKeyId ? (
        <FieldRowNote>
          <p className="text-xs text-notice-ink">
            {t("conn.basicGeneratedKeyStaged", { path: preferredKey.privateRelativePath })}
          </p>
        </FieldRowNote>
      ) : null}

      {secrets.unlocked && keyState === "editable" && selectedPrivateKey !== undefined && selectedPrivateKey.encrypted ? (
        <details
          open={passphraseOpen}
          onToggle={(event) => onPassphraseOpenChange(event.currentTarget.open)}
          className="border-t border-line"
        >
          <DisclosureSummary className="py-3.5 text-[13.5px] font-medium text-ink">
            {t("conn.basicManageKeyPassphrase")}
          </DisclosureSummary>
          <FieldRowNote>
            <p className="text-[13px] text-ink-muted">{t("conn.basicKeyPassphraseHeading")}</p>
            <p className={hintText}>
              {derived.dedicatedKeyPassphrase
                  ? t("conn.basicKeyPassphraseDedicated")
                  : derived.namedKeyPassphrase !== undefined
                    ? t("conn.basicKeyPassphraseShared", { name: derived.namedKeyPassphrase.name })
                    : t("conn.basicKeyPassphraseNone")}
            </p>
            {derived.namedKeyPassphrase !== undefined && derived.otherNamedKeyUses.length > 0 ? (
              <p className={hintText}>
                {t("conn.basicKeyPassphraseSharedOthers", { count: derived.otherNamedKeyUses.length })}
              </p>
            ) : null}
            {derived.namedKeyPassphrase !== undefined ? (
              <p className={hintText}>{t("conn.basicKeyPassphraseDetach")}</p>
            ) : null}
          </FieldRowNote>
          <FieldRow label={t("conn.basicNewKeyPassphrase")} interactiveChildren>
            <PasswordInput
              label={t("conn.basicNewKeyPassphrase")}
              value={draft.keyPassphrase}
              onChange={(value) => onEdit({ keyPassphrase: value })}
            />
          </FieldRow>
          <FieldRow label={t("conn.basicConfirmKeyPassphrase")} hint={t("conn.basicKeyPassphraseStoredNote")} interactiveChildren>
            <PasswordInput
              label={t("conn.basicConfirmKeyPassphrase")}
              value={draft.keyPassphraseConfirmation}
              onChange={(value) => onEdit({ keyPassphraseConfirmation: value })}
            />
          </FieldRow>
          {(derived.hasKeyPassphraseDraft && !derived.keyPassphraseValid) || serverKeyPassphraseError !== "" ? (
            <div className="flex flex-col gap-2 pb-3.5">
              {derived.hasKeyPassphraseDraft && !derived.keyPassphraseValid ? (
                <Notice tone="danger">{t("conn.basicKeyPassphraseMismatch")}</Notice>
              ) : null}
              {serverKeyPassphraseError === "" ? null : (
                <Notice tone="danger">{serverKeyPassphraseError}</Notice>
              )}
            </div>
          ) : null}
        </details>
      ) : null}
    </>
  );
}
