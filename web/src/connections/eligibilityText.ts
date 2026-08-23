import type { MessageKey } from "../i18n/messages";

const eligibilityKeys: Record<string, MessageKey> = {
  password_authentication_off: "password.blocker.authenticationOff",
  alias_not_simple: "password.blocker.aliasNotSimple",
  identity_file_configured: "password.blocker.identityFile",
  host_key_unknown: "password.warn.hostKeyUnknown",
  hostname_unresolved: "password.warn.hostNameUnresolved",
};

export function eligibilityText(translate: (key: MessageKey) => string, code: string): string {
  return code in eligibilityKeys ? translate(eligibilityKeys[code]!) : code;
}
