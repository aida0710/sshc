import type { Credential } from "../api/credentials";
import type { PasswordVaultStatus } from "../api/vault";
import type { ConnectionSecretsApi } from "./secretsApi";

// What the vault says to a connection form: whether it is open, and the credentials it holds once it is.
export type VaultCredentials = { status: PasswordVaultStatus; listed: Credential[] };

type VaultReader = Pick<ConnectionSecretsApi, "passwordVault" | "credentials">;
type VaultOpener = Pick<ConnectionSecretsApi, "credentials" | "initialiseVault" | "unlockVault">;

// readVaultCredentials reads the vault's state, and its credentials when it is open.
export async function readVaultCredentials(secrets: VaultReader): Promise<VaultCredentials> {
  const status = await secrets.passwordVault();
  return { status, listed: status.unlocked ? (await secrets.credentials()).credentials : [] };
}

// openVaultCredentials unlocks the vault with the master password, or creates it when there is none, and
// reads its credentials. It throws when the password is refused.
export async function openVaultCredentials(secrets: VaultOpener, vault: PasswordVaultStatus, masterPassword: string): Promise<VaultCredentials> {
  const status = vault.exists ? await secrets.unlockVault(masterPassword) : await secrets.initialiseVault(masterPassword);
  return { status, listed: status.unlocked ? (await secrets.credentials()).credentials : [] };
}
