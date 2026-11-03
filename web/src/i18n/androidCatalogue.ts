import type { Locale } from "./locale";
import { messages, type MessageKey } from "./messages";

type Catalogue = Record<MessageKey, string>;

// The Android app draws the same pages as a browser, but a person holding it has no browser tab, no command line and
// no way to open sshc's files. What it says differently is kept here, so that a screen asks for one key whichever it
// runs in.

// rewordings hold across the catalogue. The person's phone is "this device" (docs/writing-style.md); "machine" stays
// where it means the machines that sync, which may be PCs.
const rewordings: Record<Locale, readonly (readonly [RegExp, string])[]> = {
  ja: [
    [/このマシン|このブラウザ/gu, "このデバイス"],
    [/ブラウザのメモリ/gu, "アプリのメモリ"],
  ],
  en: [
    [/\bthis (?:machine|browser)\b/gu, "this device"],
    [/\bThis (?:machine|browser)\b/gu, "This device"],
    [/\bthe browser's memory\b/gu, "the app's memory"],
  ],
};

// replacements are the strings a rewording cannot put right.
const replacements: Record<Locale, Partial<Catalogue>> = {
  ja: {
    "shell.bootstrapFailed": "ローカルセッションを開始できませんでした。アプリを開き直してください。",
    "shell.sessionEnded": "再読み込みするとローカルセッションを自動で復旧します。",
    "shortcuts.saveFailed": "保存できませんでした。プリセットを再読み込みし、接続状態を確認してください。",
    "sftp.manager.browserSource": "このデバイス",
    "sftp.manager.browserDestination": "このデバイスの保存先",
    "terminal.notificationSoundHint": "アプリが裏にあるとき、ターミナル内のプログラムが通知（OSC 9／99／777）を送ると鳴ります。このデバイスが使うプリセットに保存されます。",
    "terminal.touchModeAutoHint": "タッチ優先の画面では、指でターミナルを操作する形にします。判定が合わないときは、オフにしてタッチかマウスに固定します。このデバイスが使うプリセットの設定で、すべてのターミナルに効きます",
    "terminal.browserScrollbackLabel": "アプリのスクロールバック（行）",
    "connection.encodingHint": "ターミナルでこの接続先に使います。",
    "menu.notificationsDescription": "通知と通知音",
    "menu.machinesDescription": "マシンごとのプリセット",
    "settings.machinesDescription": "Syncでつながっているマシンです。どのプリセットを使うかは、マシンごとに選びます。",
    "settings.languageBrowser": "デバイスに合わせる",
    "copy.refused": "クリップボードへの書き込みを拒否されました。",
    "reveal.warning": "秘密鍵をこのページに表示します。画面を見られる人や、クリップボードの履歴を読むアプリは、鍵を写し取れます。",
    "history.backupRetention": "変更前のファイルはアプリの中に残します。",
    "conn.basicAgentOrInherited": "継承した鍵",
  },
  en: {
    "shell.bootstrapFailed": "Secure local session could not be started. Reopen the app.",
    "shell.sessionEnded": "Reload to recover the local session automatically.",
    "shortcuts.saveFailed": "Could not save. Reload the presets and check the connection.",
    "sftp.manager.browserSource": "This device",
    "sftp.manager.browserDestination": "Save location on this device",
    "terminal.notificationSoundHint": "Played when a terminal program sends a notification (OSC 9, 99 or 777) while the app is in the background. Kept in the preset this device uses.",
    "terminal.touchModeAutoHint": "Use terminals by finger where the screen is touch first. Turn this off to fix touch or mouse when the screen is detected wrongly. A setting of the preset this device uses; applies to every terminal",
    "terminal.browserScrollbackLabel": "App scrollback (lines)",
    "connection.encodingHint": "Used by this connection in the terminal.",
    "menu.notificationsDescription": "Notifications and sounds",
    "menu.machinesDescription": "Each machine's preset",
    "settings.machinesDescription": "The machines that sync. Each one chooses the preset it uses.",
    "settings.languageBrowser": "Follow the device",
    "copy.refused": "Writing to the clipboard was refused.",
    "reveal.warning": "The private key is shown on this page. Anyone who can see the screen, and apps that read the clipboard history, can copy it.",
    "history.backupRetention": "Previous versions of changed files are kept inside the app.",
    "conn.basicAgentOrInherited": "Inherited keys",
  },
};

function reworded(locale: Locale): Catalogue {
  const entries = Object.entries(messages[locale]).map(([key, text]) => [
    key,
    rewordings[locale].reduce((worded, [pattern, replacement]) => worded.replace(pattern, replacement), text),
  ]);
  return Object.fromEntries(entries) as Catalogue;
}

const built = new Map<Locale, Catalogue>();

export function androidCatalogue(locale: Locale): Catalogue {
  const existing = built.get(locale);
  if (existing !== undefined) return existing;
  const catalogue = { ...reworded(locale), ...replacements[locale] };
  built.set(locale, catalogue);
  return catalogue;
}
