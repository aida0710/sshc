import { postJSON } from "../../api/guards";
import { engineTerminalBase } from "../../api/remoteEngines";
import type { components } from "../../api/schema";
import { validateOpenAPISchema } from "../../api/validators.generated";

export type TerminalCommandTarget = components["schemas"]["TerminalCommandTargetRequest"];
export type TerminalCommandRequest = components["schemas"]["TerminalCommandPreviewRequest"];
export type TerminalCommandPreview = components["schemas"]["TerminalCommandPreview"];
export type TerminalCommandDispatch = components["schemas"]["TerminalCommandDispatchResponse"];

export type TerminalCommandApi = {
  preview(request: TerminalCommandRequest): Promise<TerminalCommandPreview>;
  dispatch(prepared: TerminalCommandPreview, request: TerminalCommandRequest, submit?: boolean): Promise<TerminalCommandDispatch>;
};

// createTerminalCommandApi sends quick commands and broadcasts through the terminal API at base. For a run
// target, this machine's engine expands the snippet, shows its redacted form and keeps the run target's
// confirmation (see internal/httpserver/remote_terminal_commands.go).
function createTerminalCommandApi(base: string): TerminalCommandApi {
  return {
    async preview(request) {
      return validateOpenAPISchema<TerminalCommandPreview>(
        "TerminalCommandPreview",
        await postJSON<unknown>(`${base}/commands/preview`, request),
      );
    },
    async dispatch(prepared, request, submit = true) {
      return validateOpenAPISchema<TerminalCommandDispatch>(
        "TerminalCommandDispatchResponse",
        await postJSON<unknown>(`${base}/commands`, { ...request, submit, evidence: prepared.evidence }, prepared.actionToken),
      );
    },
  };
}

export const terminalCommandApi = createTerminalCommandApi("/api/v1/terminal");

// terminalCommandApiFor is the command API of the sessions on runTarget, or on this machine.
export function terminalCommandApiFor(runTarget: string | null | undefined): TerminalCommandApi {
  return runTarget === null || runTarget === undefined ? terminalCommandApi : createTerminalCommandApi(engineTerminalBase(runTarget));
}
