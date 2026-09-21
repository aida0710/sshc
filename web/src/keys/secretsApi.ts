import { credentialsApi, type CredentialsApi } from "../api/credentials";
import { vaultApi, type VaultApi } from "../api/vault";

// The key list stores passphrases as credentials and assigns them to keys,
// which needs the vault open.
export type KeySecretsApi = Pick<VaultApi, "passwordVault"> &
  Pick<CredentialsApi, "credentials" | "storeCredential" | "assignCredential" | "unassignCredential">;
export const keySecretsApi: KeySecretsApi = { ...vaultApi, ...credentialsApi };
