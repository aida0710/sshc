import { failureCode } from "../../api/client";
import type { Translate } from "../../i18n/context";
import { codeText, type CodeMessages } from "../../i18n/codeText";

// エンジンが保存レイアウトの API で返す code（internal/httpserver/workspaces.go）の言い方。
const workspaceMessages: CodeMessages = {
  workspace_not_found: "workspace.notFound",
  workspace_limit: "workspace.limitReached",
  invalid_workspace: "workspace.invalid",
  workspace_newer_schema: "workspace.newerSchema",
};

export function describeWorkspaceFailure(t: Translate, error: unknown): string {
  return codeText(t, failureCode(error), { messages: workspaceMessages, fallback: "workspace.failed" });
}
