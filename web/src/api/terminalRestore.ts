import { apiClient } from "./client";
import { postEmpty } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type TerminalRestoreOffer = components["schemas"]["TerminalRestoreOffer"];
export type TerminalRestoreSession = components["schemas"]["TerminalRestoreSession"];
export type TerminalRestoreResult = components["schemas"]["TerminalRestoreResult"];

// The sessions an engine had open when it last stopped (an update, a restart
// of the machine, a manual stop). The engine offers them once; restoring or
// declining ends the offer.
export type TerminalRestoreApi = {
  offer(): Promise<TerminalRestoreOffer>;
  restore(): Promise<TerminalRestoreResult>;
  decline(): Promise<void>;
};

// `base` is the engine's terminal API: this machine's /api/v1/terminal, or a
// run-target engine's /api/v1/engines/{alias}/terminal that this machine's
// engine forwards.
export function createTerminalRestoreApi(base: string): TerminalRestoreApi {
  const restorePath = `${base}/restore`;
  return {
    async offer() {
      return validateOpenAPISchema<TerminalRestoreOffer>("TerminalRestoreOffer", await apiClient.read(restorePath));
    },
    async restore() {
      return validateOpenAPISchema<TerminalRestoreResult>("TerminalRestoreResult", await postEmpty<unknown>(restorePath));
    },
    async decline() {
      await apiClient.mutate<unknown>(restorePath, { method: "DELETE" });
    },
  };
}

export const terminalRestoreApi = createTerminalRestoreApi("/api/v1/terminal");
