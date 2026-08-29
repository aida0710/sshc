import { apiClient } from "../../api/client";
import { asArray, asRecord, asString, jsonHeaders } from "../../api/guards";
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
    return preview(await apiClient.mutate<unknown>("/api/v1/terminal/commands/preview", {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify(request),
    }));
  },

  async dispatch(prepared: TerminalCommandPreview, request: TerminalCommandRequest, submit = true): Promise<TerminalCommandDispatch> {
    return dispatch(await apiClient.mutate<unknown>("/api/v1/terminal/commands", {
      method: "POST",
      headers: { ...jsonHeaders, "X-SSHC-Action": prepared.actionToken },
      body: JSON.stringify({ ...request, submit, evidence: prepared.evidence }),
    }));
  },
};
