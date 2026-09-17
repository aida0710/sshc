import { issueAction, postEmpty, postJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type ConfigCheckResponse = components["schemas"]["ConfigCheckResponse"];
export type EffectiveResponse = components["schemas"]["EffectiveResponse"];
export type ReachabilityResponse = components["schemas"]["ReachabilityResponse"];
export type AuthenticationResponse = components["schemas"]["AuthenticationResponse"];

export const REACHABILITY_ACTION_KIND = "diagnostics.reachability";
export const AUTHENTICATION_ACTION_KIND = "diagnostics.authentication";

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
export const diagnosticsApi: DiagnosticsApi = {
  async configCheck() {
    return validateConfigCheck(
      await postEmpty<unknown>("/api/v1/diagnostics/config"),
    );
  },
  async effective(alias) {
    return validateEffective(
      await postJSON<unknown>("/api/v1/diagnostics/effective", { alias }),
    );
  },
  async reachability(alias) {
    const token = await issueAction(REACHABILITY_ACTION_KIND, alias);
    return validateReachability(
      await postJSON<unknown>(
        "/api/v1/diagnostics/reachability",
        { alias },
        token,
      ),
    );
  },
  async authentication(alias, acknowledgeExecutable) {
    const token = await issueAction(AUTHENTICATION_ACTION_KIND, alias);
    return validateAuthentication(
      await postJSON<unknown>(
        "/api/v1/diagnostics/authentication",
        { alias, acknowledgeExecutable },
        token,
      ),
    );
  },
};
