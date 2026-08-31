import { apiClient } from "../api/client";
import { jsonHeaders, issueAction } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";

export type KeyItem = components["schemas"]["KeyItem"];
export type KeyCertificate = components["schemas"]["KeyCertificate"];
export type UnreadableFile = components["schemas"]["UnreadableFile"];
export type UnresolvedReference = components["schemas"]["UnresolvedReference"];
export type KeyReference = components["schemas"]["KeyReference"];
export type KeyVariant = components["schemas"]["KeyVariant"];
export type KeyInventoryResponse = components["schemas"]["KeyInventoryResponse"];
export type KeyAlgorithmsResponse = components["schemas"]["KeyAlgorithmsResponse"];
export type GenerateKeyResponse = components["schemas"]["GenerateKeyResponse"];
export type HardwareCommandResponse = components["schemas"]["HardwareCommandResponse"];
export type ChangePassphraseResponse = components["schemas"]["ChangePassphraseResponse"];
export type RevealPrivateKeyResponse = components["schemas"]["RevealPrivateKeyResponse"];
export type RegisterKeyResponse = components["schemas"]["RegisterKeyResponse"];
export type AgentIdentitiesResponse = components["schemas"]["AgentIdentitiesResponse"];
export type PublicKeyResponse = components["schemas"]["PublicKeyResponse"];
export type RelocateKeyResponse = components["schemas"]["RelocateKeyResponse"];
export type RelocatedKeyFile = components["schemas"]["RelocatedKeyFile"];
export type RewrittenKeyReference = components["schemas"]["RewrittenKeyReference"];
export type AgentIdentity = components["schemas"]["AgentIdentity"];
export type IssueActionResponse = components["schemas"]["IssueActionResponse"];
export type TrashListResponse = components["schemas"]["TrashListResponse"];
export type TrashKeyResponse = components["schemas"]["TrashKeyResponse"];
export type RestoreTrashResponse = components["schemas"]["RestoreTrashResponse"];
export type PurgeTrashResponse = components["schemas"]["PurgeTrashResponse"];

export const REVEAL_ACTION_KIND = "private_key.reveal";
export const PURGE_ACTION_KIND = "trash.purge";

export function selectablePrivateKeys(inventory: Pick<KeyInventoryResponse, "items">): KeyItem[] {
  return inventory.items.filter((item) => item.kind === "private_key");
}

export type KeyLocationInput = components["schemas"]["RelocateKeyRequest"];
export type GenerateKeyInput = components["schemas"]["GenerateKeyRequest"];
export type HardwareCommandInput = components["schemas"]["HardwareCommandRequest"];
export type PassphraseInput = components["schemas"]["ChangePassphraseRequest"];
export type RegisterAgentInput = components["schemas"]["RegisterKeyRequest"];

export type KeysApi = {
  inventory(): Promise<KeyInventoryResponse>;
  algorithms(): Promise<KeyAlgorithmsResponse>;
  generate(input: GenerateKeyInput): Promise<GenerateKeyResponse>;
  hardwareCommand(input: HardwareCommandInput): Promise<HardwareCommandResponse>;
  changePassphrase(keyId: string, input: PassphraseInput): Promise<ChangePassphraseResponse>;
  reveal(keyId: string): Promise<RevealPrivateKeyResponse>;
  publicKey(keyId: string): Promise<PublicKeyResponse>;
  relocate(keyId: string, change: KeyLocationInput): Promise<RelocateKeyResponse>;
  registerWithAgent(keyId: string, input: RegisterAgentInput): Promise<RegisterKeyResponse>;
  deregisterFromAgent(keyId: string): Promise<AgentIdentitiesResponse>;
  trash(keyId: string): Promise<TrashKeyResponse>;
  listTrash(): Promise<TrashListResponse>;
  restore(entryId: string): Promise<RestoreTrashResponse>;
  purge(entryId: string): Promise<PurgeTrashResponse>;
};






function validateInventory(value: unknown): KeyInventoryResponse {
  return validateOpenAPISchema<KeyInventoryResponse>("KeyInventoryResponse", value);
}

function validateAgentIdentitiesResponse(value: unknown): AgentIdentitiesResponse {
  return validateOpenAPISchema<AgentIdentitiesResponse>("AgentIdentitiesResponse", value);
}

function validateAlgorithms(value: unknown): KeyAlgorithmsResponse {
  return validateOpenAPISchema<KeyAlgorithmsResponse>("KeyAlgorithmsResponse", value);
}

function validateReveal(value: unknown): RevealPrivateKeyResponse {
  return validateOpenAPISchema<RevealPrivateKeyResponse>("RevealPrivateKeyResponse", value);
}

function validateRegister(value: unknown): RegisterKeyResponse {
  return validateOpenAPISchema<RegisterKeyResponse>("RegisterKeyResponse", value);
}

function validatePublicKey(value: unknown): PublicKeyResponse {
  return validateOpenAPISchema<PublicKeyResponse>("PublicKeyResponse", value);
}

function validateRelocate(value: unknown): RelocateKeyResponse {
  return validateOpenAPISchema<RelocateKeyResponse>("RelocateKeyResponse", value);
}

function validateTrashList(value: unknown): TrashListResponse {
  return validateOpenAPISchema<TrashListResponse>("TrashListResponse", value);
}

function validateRestore(value: unknown): RestoreTrashResponse {
  return validateOpenAPISchema<RestoreTrashResponse>("RestoreTrashResponse", value);
}



export const keysApi: KeysApi = {
  async inventory() {
    return validateInventory(await apiClient.read("/api/v1/keys"));
  },
  async algorithms() {
    return validateAlgorithms(await apiClient.read("/api/v1/keys/algorithms"));
  },
  generate: (input) =>
    apiClient.mutate<GenerateKeyResponse>("/api/v1/keys", {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({
        algorithm: input.algorithm,
        bits: input.bits ?? 0,
        fileName: input.fileName,
        group: input.group,
        comment: input.comment,
        passphrase: input.passphrase,
        unencrypted: input.unencrypted,
      }),
    }),
  hardwareCommand: (input) =>
    apiClient.mutate<HardwareCommandResponse>("/api/v1/keys/hardware-command", {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify(input),
    }),
  changePassphrase: (keyId, input) =>
    apiClient.mutate<ChangePassphraseResponse>(`/api/v1/keys/${encodeURIComponent(keyId)}/passphrase`, {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify(input),
    }),
  async reveal(keyId) {
    const token = await issueAction(REVEAL_ACTION_KIND, keyId);
    return validateReveal(
      await apiClient.mutate<unknown>(`/api/v1/keys/${encodeURIComponent(keyId)}/reveal`, {
        method: "POST",
        headers: { "X-SSHC-Action": token },
      }),
    );
  },
  async publicKey(keyId) {
    return validatePublicKey(await apiClient.read(`/api/v1/keys/${encodeURIComponent(keyId)}/public`));
  },
  async relocate(keyId, change) {
    const response = await apiClient.send(`/api/v1/keys/${encodeURIComponent(keyId)}/location`, {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify(change),
    });
    if (!response.ok && response.status !== 409) {
      throw new Error("api_mutation_failed");
    }
    return validateRelocate(await response.json());
  },
  async registerWithAgent(keyId, input) {
    return validateRegister(
      await apiClient.mutate<unknown>(`/api/v1/keys/${encodeURIComponent(keyId)}/agent`, {
        method: "POST",
        headers: jsonHeaders,
        body: JSON.stringify(input),
      }),
    );
  },
  async deregisterFromAgent(keyId) {
    return validateAgentIdentitiesResponse(
      await apiClient.mutate<unknown>(`/api/v1/keys/${encodeURIComponent(keyId)}/agent`, {
        method: "DELETE",
      }),
    );
  },
  trash: (keyId) =>
    apiClient.mutate<TrashKeyResponse>(`/api/v1/keys/${encodeURIComponent(keyId)}/trash`, { method: "POST" }),
  async listTrash() {
    return validateTrashList(await apiClient.read("/api/v1/trash"));
  },
  async restore(entryId) {
    const response = await apiClient.send(`/api/v1/trash/${encodeURIComponent(entryId)}/restore`, {
      method: "POST",
    });
    if (!response.ok && response.status !== 409) {
      throw new Error("api_mutation_failed");
    }
    return validateRestore(await response.json());
  },
  async purge(entryId) {
    const token = await issueAction(PURGE_ACTION_KIND, entryId);
    return apiClient.mutate<PurgeTrashResponse>(`/api/v1/trash/${encodeURIComponent(entryId)}`, {
      method: "DELETE",
      headers: { "X-SSHC-Action": token },
    });
  },
};
