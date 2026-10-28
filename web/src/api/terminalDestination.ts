import { apiClient } from "./client";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type TerminalDestination = components["schemas"]["TerminalDestination"];

// terminalDestinationBase is this machine's terminal API; a run target's is engineTerminalBase(alias).
export const terminalDestinationBase = "/api/v1/terminal";

// fetchTerminalDestination asks the engine behind base where its SSH configuration takes alias. The engine
// does not connect.
export async function fetchTerminalDestination(base: string, alias: string): Promise<TerminalDestination> {
  return validateOpenAPISchema<TerminalDestination>(
    "TerminalDestination",
    await apiClient.read(`${base}/destinations/${encodeURIComponent(alias)}`),
  );
}
