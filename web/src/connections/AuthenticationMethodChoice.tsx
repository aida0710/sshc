import type { CreateConnectionAuthentication } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { fieldLabel } from "../ui/form";

type AuthenticationKind = CreateConnectionAuthentication["kind"];

const authenticationChoices: readonly [AuthenticationKind, MessageKey][] = [
  ["identity_file", "conn.createIdentityFile"],
  ["dedicated_password", "conn.createDedicatedPassword"],
  ["saved_password", "conn.createSavedPassword"],
  ["new_shared_password", "conn.createNewSharedPassword"],
];

// AuthenticationMethodChoice は、新しい接続の認証の方法を選ぶラジオボタンである。
export function AuthenticationMethodChoice({
  value,
  disabled,
  onChange,
}: {
  value: AuthenticationKind;
  disabled: boolean;
  onChange: (kind: AuthenticationKind) => void;
}) {
  const t = useTranslate();
  return (
    <fieldset className="grid gap-2 sm:grid-cols-2" disabled={disabled}>
      <legend className={fieldLabel}>{t("conn.createAuthenticationMethod")}</legend>
      {authenticationChoices.map(([kind, label]) => (
        <label
          key={kind}
          className="flex cursor-pointer items-center gap-2 rounded-lg border border-line bg-card p-3 text-sm text-ink transition-colors hover:bg-select-fill has-[:checked]:border-accent has-[:checked]:bg-select-fill"
        >
          <input
            type="radio"
            name="create-authentication"
            value={kind}
            checked={value === kind}
            onChange={() => onChange(kind)}
            className="accent-accent"
          />
          <span>{t(label)}</span>
        </label>
      ))}
    </fieldset>
  );
}
