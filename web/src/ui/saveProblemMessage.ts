import type { Problem } from "../api/client";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

const refusalKeys: Record<string, MessageKey> = {
  alias_already_declared: "refusal.alias_already_declared",
  directory_not_empty: "refusal.directory_not_empty",
  not_a_directory: "refusal.not_a_directory",
  group_is_declared: "refusal.group_is_declared",
  destination_exists: "refusal.destination_exists",
  region_damaged: "refusal.region_damaged",
  metadata_changed: "refusal.metadata_changed",
};

// saveProblemMessage は、設定の保存が断られた理由を利用者向けの文にする。
export function saveProblemMessage(problem: Problem, t: Translate): string {
  switch (problem.code) {
    case "config_syntax_error":
      return t("preview.syntaxError", {
        path: problem.path ?? t("preview.theFile"),
        line: problem.line ?? 0,
        column: problem.column ?? 0,
      });
    case "config_graph_error":
      return t("preview.graphError");
    case "config_conflict":
      return t("preview.conflictError");
    default:
      return problem.code in refusalKeys
        ? t(refusalKeys[problem.code]!, { detail: problem.detail ?? "" })
        : t("preview.rejected", { code: problem.code });
  }
}
