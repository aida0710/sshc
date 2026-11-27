import { postJSON } from "../../api/guards";
import { engineTerminalBase } from "../../api/remoteEngines";
import type { components } from "../../api/schema";
import { validateOpenAPISchema } from "../../api/validators.generated";

export type TerminalCommandRequest = components["schemas"]["TerminalCommandPreviewRequest"];
export type TerminalCommandPreview = components["schemas"]["TerminalCommandPreview"];
export type TerminalCommandDispatch = components["schemas"]["TerminalCommandDispatchResponse"];

export type TerminalCommandApi = {
  preview(request: TerminalCommandRequest): Promise<TerminalCommandPreview>;
  dispatch(prepared: TerminalCommandPreview, request: TerminalCommandRequest, submit?: boolean): Promise<TerminalCommandDispatch>;
};

// createTerminalCommandApi sends quick commands and broadcasts through the terminal API at base. リモートセッションの
// セッションへは、このマシンのエンジンがそのまま取り次ぎ、スニペットの展開と確認のトークンは実行先のエンジンがする。
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
