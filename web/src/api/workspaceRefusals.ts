import type { MessageKey } from "../i18n/messages";

// 設定の保存先（workspace）を使えないという拒否は、設定・接続・鍵・Vaultのどの操作でも
// 起き、各画面は自分では説明しない。4xxでも共通の通知に出し、「待つ」か「Historyで復旧する」かを伝える。
export const workspaceRefusalMessages: Readonly<Record<string, MessageKey>> = {
  workspace_busy: "diagnostic.workspaceBusy",
  workspace_pending_transaction: "diagnostic.workspacePendingTransaction",
};

export function isWorkspaceRefusal(code: string): boolean {
  return Object.hasOwn(workspaceRefusalMessages, code);
}
