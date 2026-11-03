import type { CodeMessages } from "../i18n/codeText";

// hostKeyProblemMessages は、ホスト鍵で接続しなかった理由の code（engine の internal/httpserver/sshconnect.go の
// hostKeyProblem）の言い方である。ターミナルのセッションと SFTP のタブが同じ文で伝える。
export const hostKeyProblemMessages: CodeMessages = {
  host_key_declined: "terminal.hostKeyDeclined",
  host_key_unknown: "terminal.hostKeyUnknown",
  host_key_changed: "terminal.hostKeyChanged",
  host_key_revoked: "terminal.hostKeyRevoked",
  known_hosts_symlink: "terminal.knownHostsSymlink",
};

// hostKeyProblemCodes は、その code の一覧である。SFTP の画面は、これらを共通の失敗の通知に出さず、自分で伝える。
export const hostKeyProblemCodes: readonly string[] = Object.keys(hostKeyProblemMessages);
