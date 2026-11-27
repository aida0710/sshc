import { apiClient } from "./client";
import { engineTerminalBase } from "./remoteEngines";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type MachineStatus = components["schemas"]["MachineStatus"];

export type TerminalOverviewApi = {
  machineStatus(): Promise<MachineStatus>;
};

// The overview is decoration on Home: when it cannot be read, Home leaves it out and says
// nothing. An engine older than these routes answers 404 without a problem (request_failed).
const quietFailures = ["request_failed", "not_found", "network_request_failed"] as const;

// terminalOverviewApi reads the machine of the engine new shells open on: this machine's (runTarget null), or a run
// target's through this machine's engine.
export function terminalOverviewApi(runTarget: string | null): TerminalOverviewApi {
  const base = runTarget === null ? "/api/v1/terminal" : engineTerminalBase(runTarget);
  return {
    async machineStatus() {
      return validateOpenAPISchema<MachineStatus>("MachineStatus", await apiClient.read(`${base}/machine`, { locallyHandledCodes: quietFailures }));
    },
  };
}
