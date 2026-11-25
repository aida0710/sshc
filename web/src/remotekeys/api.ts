import { localApiBase } from "../api/remoteEngines";
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


// createRemoteKeysApi is this API of the engine at base (engineApiBase): this machine's, or a run target's as this
// machine's engine forwards it.
export function createRemoteKeysApi(base: string): RemoteKeysApi {
  return {
    async plan(input) {
      return validatePlan(
        await postJSON<unknown>(`${base}/remote-keys/plan`, { alias: input.alias, keyPath: input.keyPath, publicKey: input.publicKey }),
      );
    },
    async register(input) {
      return validateRegistration(
        await postJSON<unknown>(`${base}/remote-keys/register`, {
          alias: input.alias,
          keyPath: input.keyPath,
          publicKey: input.publicKey,
          acknowledgeExecutable: input.acknowledgeExecutable,
        }, input.actionToken),
      );
    },
  };
}

export const remoteKeysApi = createRemoteKeysApi(localApiBase);
