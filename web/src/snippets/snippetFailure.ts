import { failureCode } from "../api/client";
import type { Translate } from "../i18n/context";
import { codeText, type CodeMessages } from "../i18n/codeText";

// エンジンが snippet の API で返す失敗 code（internal/httpserver/snippets.go の
// snippetProblem）を、画面に出す文へ変える。
const snippetFailureMessages: CodeMessages = {
  snippet_not_found: "snippets.notFound",
  snippet_preview_changed: "snippets.previewChanged",
  snippet_jobs_full: "snippets.jobsFull",
  snippet_job_finished: "snippets.jobFinished",
  invalid_snippet: "snippets.invalid",
};

export function describeSnippetFailure(t: Translate, error: unknown): string {
  return codeText(t, failureCode(error), { messages: snippetFailureMessages, fallback: "snippets.failed" });
}
