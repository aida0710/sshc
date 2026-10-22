import { postJSON } from "../../api/guards";
import type { components } from "../../api/schema";
import { validateOpenAPISchema } from "../../api/validators.generated";

export type TerminalCommandTarget = components["schemas"]["TerminalCommandTargetRequest"];
export type TerminalCommandRequest = components["schemas"]["TerminalCommandPreviewRequest"];
export type TerminalCommandPreview = components["schemas"]["TerminalCommandPreview"];
export type TerminalCommandDispatch = components["schemas"]["TerminalCommandDispatchResponse"];

export const terminalCommandApi = {
  async preview(request: TerminalCommandRequest): Promise<TerminalCommandPreview> {
    return validateOpenAPISchema<TerminalCommandPreview>(
      "TerminalCommandPreview",
      await postJSON<unknown>("/api/v1/terminal/commands/preview", request),
    );
  },

  async dispatch(prepared: TerminalCommandPreview, request: TerminalCommandRequest, submit = true): Promise<TerminalCommandDispatch> {
    return validateOpenAPISchema<TerminalCommandDispatch>(
      "TerminalCommandDispatchResponse",
      await postJSON<unknown>("/api/v1/terminal/commands", { ...request, submit, evidence: prepared.evidence }, prepared.actionToken),
    );
  },
};
