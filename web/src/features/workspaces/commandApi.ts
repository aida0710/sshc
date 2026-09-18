import { asArray, asRecord, asString, postJSON } from "../../api/guards";
import type { components } from "../../api/schema";

export type TerminalCommandTarget = components["schemas"]["TerminalCommandTargetRequest"];
export type TerminalCommandRequest = components["schemas"]["TerminalCommandPreviewRequest"];
export type TerminalCommandPreview = components["schemas"]["TerminalCommandPreview"];
export type TerminalCommandDispatch = components["schemas"]["TerminalCommandDispatchResponse"];

function preview(value: unknown): TerminalCommandPreview {
  const item = asRecord(value);
  return {
    snippetId: asString(item.snippetId),
    evidence: asString(item.evidence),
    reviewEvidence: asString(item.reviewEvidence),
    actionToken: asString(item.actionToken),
    actionExpiresAt: asString(item.actionExpiresAt),
    targets: asArray(item.targets).map((raw) => {
      const target = asRecord(raw);
      return {
        targetId: asString(target.targetId),
        sessionId: asString(target.sessionId),
        alias: asString(target.alias),
        title: asString(target.title),
        command: asString(target.command),
      };
    }),
  };
}

function dispatch(value: unknown): TerminalCommandDispatch {
  const item = asRecord(value);
  return {
    results: asArray(item.results).map((raw) => {
      const result = asRecord(raw);
      const status = asString(result.status);
      if (status !== "delivered" && status !== "failed") throw new Error("invalid_response");
      return {
        targetId: asString(result.targetId),
        sessionId: asString(result.sessionId),
        alias: asString(result.alias),
        title: asString(result.title),
        status,
        ...(result.problem === undefined ? {} : { problem: asString(result.problem) }),
      };
    }),
  };
}

export const terminalCommandApi = {
  async preview(request: TerminalCommandRequest): Promise<TerminalCommandPreview> {
    return preview(await postJSON<unknown>("/api/v1/terminal/commands/preview", request));
  },

  async dispatch(prepared: TerminalCommandPreview, request: TerminalCommandRequest, submit = true): Promise<TerminalCommandDispatch> {
    return dispatch(await postJSON<unknown>("/api/v1/terminal/commands", { ...request, submit, evidence: prepared.evidence }, prepared.actionToken));
  },
};
