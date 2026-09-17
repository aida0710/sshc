import { credentialsApi, type CredentialsApi } from "../api/credentials";
import { vaultApi, type VaultApi } from "../api/vault";

// What a connection form needs from the vault: whether it is open, the
// credentials it can assign, and the two ways to open it from the form.
export type ConnectionSecretsApi =
  Pick<VaultApi, "passwordVault" | "passwordEligibility" | "initialiseVault" | "unlockVault"> &
  Pick<CredentialsApi, "credentials">;

export const connectionSecretsApi: ConnectionSecretsApi = {
  passwordVault: vaultApi.passwordVault,
  passwordEligibility: vaultApi.passwordEligibility,
  initialiseVault: vaultApi.initialiseVault,
  unlockVault: vaultApi.unlockVault,
  credentials: credentialsApi.credentials,
};
