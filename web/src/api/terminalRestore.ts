import { apiClient } from "./client";
import { postEmpty } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type TerminalRestoreOffer = components["schemas"]["TerminalRestoreOffer"];
export type TerminalRestoreSession = components["schemas"]["TerminalRestoreSession"];
export type TerminalRestoreResult = components["schemas"]["TerminalRestoreResult"];

// The sessions this machine's engine had open when it last stopped (an update,
// a restart of the machine, a manual stop). The engine offers them once;
// restoring or declining ends the offer.
export type TerminalRestoreApi = {
  offer(): Promise<TerminalRestoreOffer>;
  restore(): Promise<TerminalRestoreResult>;
  decline(): Promise<void>;
};

const restorePath = "/api/v1/terminal/restore";

export const terminalRestoreApi: TerminalRestoreApi = {
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
