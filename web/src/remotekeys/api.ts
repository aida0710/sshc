import { apiClient } from "../api/client";
import { jsonHeaders } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";

export type RemoteKeyPlan = components["schemas"]["RemoteKeyPlan"];
export type RemoteKeyRegisterResponse = components["schemas"]["RemoteKeyRegisterResponse"];
export type ExecutableDirective = components["schemas"]["ExecutableDirective"];

export type RemoteKeyInput = {
  alias: string;
  keyPath: string;
  publicKey: string;
};

export type RemoteKeyRegisterInput = RemoteKeyInput & {
  acknowledgeExecutable: boolean;
  actionToken: string;
};

export type RemoteKeysApi = {
  plan(input: RemoteKeyInput): Promise<RemoteKeyPlan>;
  register(input: RemoteKeyRegisterInput): Promise<RemoteKeyRegisterResponse>;
};






function validatePlan(value: unknown): RemoteKeyPlan {
  return validateOpenAPISchema<RemoteKeyPlan>("RemoteKeyPlan", value);
}

function validateRegistration(value: unknown): RemoteKeyRegisterResponse {
  return validateOpenAPISchema<RemoteKeyRegisterResponse>("RemoteKeyRegisterResponse", value);
}


export const remoteKeysApi: RemoteKeysApi = {
  async plan(input) {
    return validatePlan(
      await apiClient.mutate<unknown>("/api/v1/remote-keys/plan", {
        method: "POST",
        headers: jsonHeaders,
        body: JSON.stringify({ alias: input.alias, keyPath: input.keyPath, publicKey: input.publicKey }),
      }),
    );
  },
  async register(input) {
    return validateRegistration(
      await apiClient.mutate<unknown>("/api/v1/remote-keys/register", {
        method: "POST",
        headers: { ...jsonHeaders, "X-SSHC-Action": input.actionToken },
        body: JSON.stringify({
          alias: input.alias,
          keyPath: input.keyPath,
          publicKey: input.publicKey,
          acknowledgeExecutable: input.acknowledgeExecutable,
        }),
      }),
    );
  },
};
