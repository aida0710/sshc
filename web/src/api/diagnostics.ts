import { localApiBase } from "./remoteEngines";
import { issueAction, postEmpty, postJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type ConfigCheckResponse = components["schemas"]["ConfigCheckResponse"];
export type EffectiveResponse = components["schemas"]["EffectiveResponse"];
export type ReachabilityResponse = components["schemas"]["ReachabilityResponse"];
export type AuthenticationResponse = components["schemas"]["AuthenticationResponse"];

const REACHABILITY_ACTION_KIND = "diagnostics.reachability";
const AUTHENTICATION_ACTION_KIND = "diagnostics.authentication";

export type DiagnosticsApi = {
  configCheck(): Promise<ConfigCheckResponse>;
  effective(alias: string): Promise<EffectiveResponse>;
  reachability(alias: string): Promise<ReachabilityResponse>;
  authentication(
    alias: string,
    acknowledgeExecutable: boolean,
  ): Promise<AuthenticationResponse>;
};

function validateConfigCheck(value: unknown): ConfigCheckResponse {
  return validateOpenAPISchema<ConfigCheckResponse>("ConfigCheckResponse", value);
}

function validateEffective(value: unknown): EffectiveResponse {
  return validateOpenAPISchema<EffectiveResponse>("EffectiveResponse", value);
}

function validateReachability(value: unknown): ReachabilityResponse {
  return validateOpenAPISchema<ReachabilityResponse>("ReachabilityResponse", value);
}

function validateAuthentication(value: unknown): AuthenticationResponse {
  return validateOpenAPISchema<AuthenticationResponse>("AuthenticationResponse", value);
}

// Checks that read the SSH configuration or touch a host without opening a
// terminal: the effective config, reachability and an authentication probe.
// createDiagnosticsApi is this API of the engine at base (engineApiBase): this machine's, or a run target's as this
// machine's engine forwards it.
export function createDiagnosticsApi(base: string): DiagnosticsApi {
  return {
    async configCheck() {
      return validateConfigCheck(
        await postEmpty<unknown>(`${base}/diagnostics/config`),
      );
    },
    async effective(alias) {
      return validateEffective(
        await postJSON<unknown>(`${base}/diagnostics/effective`, { alias }),
      );
    },
    async reachability(alias) {
      const token = await issueAction(REACHABILITY_ACTION_KIND, alias, base);
      return validateReachability(
        await postJSON<unknown>(
          `${base}/diagnostics/reachability`,
          { alias },
          token,
        ),
      );
    },
    async authentication(alias, acknowledgeExecutable) {
      const token = await issueAction(AUTHENTICATION_ACTION_KIND, alias, base);
      return validateAuthentication(
        await postJSON<unknown>(
          `${base}/diagnostics/authentication`,
          { alias, acknowledgeExecutable },
          token,
        ),
      );
    },
  };
}

export const diagnosticsApi = createDiagnosticsApi(localApiBase);
