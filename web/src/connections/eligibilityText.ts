import type { MessageKey } from "../i18n/messages";

// サーバーが報告するコードを、このホストにとっての意味へ対応付ける。
//
// 未知のコードは飲み込まずそのまま表示する。サーバーには追加されたがここには
// ない規則は、**見えなくなるのではなく見える必要がある。**
const eligibilityKeys: Record<string, MessageKey> = {
  password_authentication_off: "password.blocker.authenticationOff",
  alias_not_simple: "password.blocker.aliasNotSimple",
  identity_file_configured: "password.blocker.identityFile",
  host_key_unknown: "password.warn.hostKeyUnknown",
  hostname_unresolved: "password.warn.hostNameUnresolved",
};

// eligibilityText は、保存済みパスワードが使えない理由を読める文にする。
//
// **かつては diagnostics/PasswordPanel.tsx に同居していた。** あのパネルは
// どこからもレンダリングされないまま、この 5 行を import させるためだけに
// 330 行を抱えて残っていたので、ここへ移してパネルごと消した。
export function eligibilityText(translate: (key: MessageKey) => string, code: string): string {
  return code in eligibilityKeys ? translate(eligibilityKeys[code]!) : code;
}
