import { failureCode } from "../../api/client";
import type { Translate } from "../../i18n/context";
import { codeText, type CodeMessages } from "../../i18n/codeText";

// エンジンがターミナルへのコマンド送信で返す code（internal/httpserver/terminal_commands.go の
// terminalCommandProblem と、DispatchCommand が宛先ごとに返す problem）の言い方。
// クイックコマンドとワークスペースの一括送信が同じ言い方を使う。
const terminalCommandMessages: CodeMessages = {
  invalid_terminal_command: "terminal.command.invalid",
  terminal_command_too_large: "terminal.command.tooLarge",
  terminal_command_insert_unsafe: "terminal.quickCommandInsertUnsafe",
  terminal_command_preview_changed: "terminal.command.previewChanged",
  terminal_command_target_unavailable: "terminal.command.targetUnavailable",
  terminal_session_not_found: "terminal.command.sessionNotFound",
  terminal_command_target_changed: "terminal.command.targetChanged",
  terminal_command_delivery_failed: "terminal.command.deliveryFailed",
  action_token_expired: "terminal.command.reviewExpired",
};

export function terminalCommandProblemText(t: Translate, code: string): string {
  return codeText(t, code, { messages: terminalCommandMessages, fallback: "terminal.command.failed" });
}

export function describeTerminalCommandFailure(t: Translate, error: unknown): string {
  return terminalCommandProblemText(t, failureCode(error));
}
