import type { MessageKey } from "../i18n/messages";

export function snippetVariableTypeLabelKey(type: string): MessageKey {
  switch (type) {
    case "string":
      return "snippets.variableType.string";
    case "integer":
      return "snippets.variableType.integer";
    case "boolean":
      return "snippets.variableType.boolean";
    case "secret":
      return "snippets.variableType.secret";
    default:
      return "snippets.variableType.unknown";
  }
}

export function snippetStatusLabelKey(status: string): MessageKey {
  switch (status) {
    case "running":
      return "snippets.status.running";
    case "completed":
      return "snippets.status.completed";
    case "cancelled":
      return "snippets.status.cancelled";
    case "queued":
      return "snippets.status.queued";
    case "succeeded":
      return "snippets.status.succeeded";
    case "failed":
      return "snippets.status.failed";
    default:
      return "snippets.status.unknown";
  }
}

// snippetProblemLabelKey は、ホストごとの結果が失敗した理由（API の固定コード）を
// 画面の文言にする。知らないコードには文言を出さず、状態と終了コードだけを見せる。
export function snippetProblemLabelKey(problem: string | undefined): MessageKey | undefined {
  switch (problem) {
    case "timed_out":
      return "snippets.problem.timedOut";
    case "cancelled":
      return "snippets.problem.cancelled";
    case "run_failed":
      return "snippets.problem.runFailed";
    default:
      return undefined;
  }
}
