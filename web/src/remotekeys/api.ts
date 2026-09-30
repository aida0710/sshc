import { postJSON } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";

export type RemoteKeyPlan = components["schemas"]["RemoteKeyPlan"];
export type RemoteKeyRegisterResponse = components["schemas"]["RemoteKeyRegisterResponse"];
export type ExecutableDirective = components["schemas"]["ExecutableDirective"];

export type RemoteKeyInput = components["schemas"]["RemoteKeyPlanRequest"];

// actionToken は本文ではなく X-SSHC-Action ヘッダーで送る。
export type RemoteKeyRegisterInput = components["schemas"]["RemoteKeyRegisterRequest"] & {
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
      await postJSON<unknown>("/api/v1/remote-keys/plan", { alias: input.alias, keyPath: input.keyPath, publicKey: input.publicKey }),
    );
  },
  async register(input) {
    return validateRegistration(
      await postJSON<unknown>("/api/v1/remote-keys/register", {
        alias: input.alias,
        keyPath: input.keyPath,
        publicKey: input.publicKey,
        acknowledgeExecutable: input.acknowledgeExecutable,
      }, input.actionToken),
    );
  },
};
