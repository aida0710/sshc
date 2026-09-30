import type { Translate } from "../i18n/context";
import { codeText, type CodeMessages } from "../i18n/codeText";

// 転送を開けなかった理由の語（internal/terminal の ForwardProblem*）の言い方。
// Go のエラーの文は engine がターミナルか API の detail に残す。
const forwardProblems: CodeMessages = {
  address_in_use: "terminal.forwardProblem.addressInUse",
  permission_denied: "terminal.forwardProblem.permissionDenied",
  agent_unreachable: "terminal.forwardProblem.agentUnreachable",
  failed: "terminal.forwardProblem.failed",
};

export function describeForwardProblem(t: Translate, problem: string): string {
  return codeText(t, problem, { messages: forwardProblems, fallback: "terminal.forwardProblem.failed" });
}

// forwardProblemIsSpecific は、理由の語だけで何が起きたかを言えるかを返す。言えない
// ときは、engine の detail を補足として出す。
export function forwardProblemIsSpecific(problem: string): boolean {
  return problem !== "failed" && Object.hasOwn(forwardProblems, problem);
}
