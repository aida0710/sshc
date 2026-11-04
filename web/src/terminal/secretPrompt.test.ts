import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { textBeforeCursor } from "./cursorLineText";
import { asksForSecret, secretPromptRowsRead } from "./secretPrompt";

function write(view: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => view.write(data, resolve));
}

describe("asksForSecret", () => {
  it.each([
    // sshc's own questions (internal/sshclient/auth.go)
    "Password for deploy@10.0.2.15:22: ",
    "Password for deploy@10.0.2.15:22 (web-01): ",
    "Saved password was rejected. Password for deploy@10.0.2.15:22 (web-01): ",
    "Enter passphrase for key '/home/deploy/.ssh/id_ed25519': ",
    "Enter passphrase for key '/home/deploy/my keys/id_ed25519': ",
    // sudo, su and ssh on the far side
    "Password: ",
    "password:",
    "[sudo] password for deploy: ",
    "deploy@web-01's password: ",
    "(deploy@web-01) Password: ",
    // Other programs that ask
    "Enter password: ",
    "Password for user postgres: ",
    "Password for 'https://deploy@git.example.com': ",
    "Vault password: ",
    "BECOME password[defaults to SSH password]: ",
    "Enter pass phrase for server.key:",
    "Enter PEM pass phrase:",
    "Enter passphrase (empty for no passphrase): ",
    "Enter same passphrase again: ",
    "Bad passphrase, try again for /home/deploy/.ssh/id_ed25519: ",
    "Current password: ",
    "New password: ",
    "Retype new password: ",
    "One-time password (OATH) for 'deploy': ",
    // After some of the answer is typed, in programs that echo asterisks
    "Password: ***",
    "Password for deploy@10.0.2.15:22: ****",
  ])("asks in English: %s", (text) => {
    expect(asksForSecret(text)).toBe(true);
  });

  it.each([
    "パスワード: ",
    "パスワード:",
    "パスワード：",
    "[sudo] deploy のパスワード: ",
    "現在のパスワード: ",
    "新しいパスワード: ",
    "新しいパスワードを再入力してください: ",
    "パスワードを入力してください：",
    "パスワード（確認）：",
    "パスフレーズを入力してください: ",
    "鍵 '/home/deploy/.ssh/id_ed25519' のパスフレーズ: ",
    "パスワード: ***",
  ])("asks in Japanese: %s", (text) => {
    expect(asksForSecret(text)).toBe(true);
  });

  it.each([
    "",
    // Shell prompts
    "deploy@web-01:~$ ",
    "deploy@web-01:~/password$ ",
    "~/password ❯ ",
    "PS C:\\Users\\deploy> ",
    "bash-5.2# ",
    // The word in the middle of a sentence
    "password: hunter2",
    "the password: is wrong",
    "Enter your password. $ ",
    "Password reset email sent: ",
    "Password authentication failed for deploy: ",
    "No password here",
    "Forgot your password?",
    "passwd: password updated successfully",
    "Changing password for deploy.",
    // Not the start of a word: the key or path of a file being edited
    "db_password:",
    "user-password:",
    "mypassword:",
    "~/password:",
    "src/password:",
    // Questions that want to be seen
    "Are you sure you want to continue connecting (yes/no)? ",
    "Please type 'yes' or 'no': ",
    "Username: ",
    "Username for 'https://git.example.com': ",
    "Verification code: ",
    // Claude Code's input line
    "> ",
    "│ > fix the password check",
    // In Japanese
    "パスワードが違います: ",
    "パスワードを変更しました: ",
    "パスワードは次のとおりです",
    "パスワード: abc",
    "ユーザー名: ",
  ])("does not ask: %s", (text) => {
    expect(asksForSecret(text)).toBe(false);
  });
});

describe("the text before the cursor, read from a terminal", () => {
  it("joins a question that wraps on a phone, as sshc's own do", async () => {
    const view = new Terminal({ cols: 40, rows: 24, allowProposedApi: true });
    await write(view, "Enter passphrase for key '/home/deploy/.ssh/id_ed25519': ");
    expect(asksForSecret(textBeforeCursor(view, secretPromptRowsRead))).toBe(true);
  });

  it("does not count what is to the right of the cursor", async () => {
    const view = new Terminal({ cols: 40, rows: 24, allowProposedApi: true });
    await write(view, "Password: \x1b[s      right pane\x1b[u");
    expect(asksForSecret(textBeforeCursor(view, secretPromptRowsRead))).toBe(true);
  });

  it("stops counting once the cursor leaves the line", async () => {
    const view = new Terminal({ cols: 40, rows: 24, allowProposedApi: true });
    await write(view, "Password: ");
    expect(asksForSecret(textBeforeCursor(view, secretPromptRowsRead))).toBe(true);
    await write(view, "\r\n");
    expect(asksForSecret(textBeforeCursor(view, secretPromptRowsRead))).toBe(false);
  });

  it("sees a question asked on the alternate screen, where tmux draws everything", async () => {
    const view = new Terminal({ cols: 40, rows: 24, allowProposedApi: true });
    await write(view, "\x1b[?1049h\x1b[5;1H[sudo] password for deploy: ");
    expect(view.buffer.active.type).toBe("alternate");
    expect(asksForSecret(textBeforeCursor(view, secretPromptRowsRead))).toBe(true);
  });

  it("reads only the last rows of a long unbroken line", async () => {
    const view = new Terminal({ cols: 10, rows: 24, allowProposedApi: true });
    await write(view, `${"x".repeat(100)}Password: `);
    expect(textBeforeCursor(view, 2)).toBe(`${"x".repeat(10)}Password: `);
    expect(textBeforeCursor(view)).toBe(`${"x".repeat(100)}Password: `);
  });
});
